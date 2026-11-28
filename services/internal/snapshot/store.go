package snapshot

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// ErrNotFound means the event has no published snapshot (or not that version).
var ErrNotFound = errors.New("snapshot: not found")

// Stored is a snapshot as stored: the exact document bytes and their SHA-256.
type Stored struct {
	OrganiserID string
	Version     int
	Doc         []byte
	SHA256      string
}

// Source reads published snapshots from the database. version 0 means the live one.
type Source interface {
	Fetch(ctx context.Context, eventID string, version int) (Stored, error)
}

// DBSource reads through control.live_snapshot(), the only published-only read the core has
// (design D3.3); it never reads the catalogue's draft tables.
type DBSource struct{ Pool *pgxpool.Pool }

func (d DBSource) Fetch(ctx context.Context, eventID string, version int) (Stored, error) {
	if d.Pool == nil {
		return Stored{}, errors.New("snapshot: no database")
	}
	var out Stored
	err := db.WithTx(ctx, d.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var err error
		out, err = fetchTx(ctx, tx, eventID, version)
		return err
	})
	return out, err
}

// fetchTx reads one published version (0: the live one) inside a transaction.
func fetchTx(ctx context.Context, tx pgx.Tx, eventID string, version int) (Stored, error) {
	var out Stored
	var v *int
	if version > 0 {
		v = &version
	}
	var doc string
	err := tx.QueryRow(ctx,
		"select organiser_id::text, version, document_text, sha256 from control.live_snapshot($1::uuid, $2::int)",
		eventID, v).Scan(&out.OrganiserID, &out.Version, &doc, &out.SHA256)
	out.Doc = []byte(doc)
	if errors.Is(err, pgx.ErrNoRows) {
		return Stored{}, ErrNotFound
	}
	return out, err
}

// Options tune the caches. The defaults keep a new version reaching every core within 10 seconds
// even when the activation call is lost: at most RedisTTL old in Redis plus MemTTL in process.
type Options struct {
	MemTTL   time.Duration // in-process copy of the live version (default 5 s)
	RedisTTL time.Duration // shared copy of the live version (default 4 s)
	// Earlier versions never change: kept longer (default 1 h in Redis, 1 min in process).
	VersionTTL time.Duration
	// MissingTTL: how long "this event has no live snapshot" is remembered (in process and in
	// Redis), so requests for unknown events cannot reach the database each time (default 5 s).
	MissingTTL time.Duration
	// MACKey signs every copy kept in Redis (snapshots, "missing" marks, availability answers): a
	// copy that Redis returns without a valid signature is ignored. From the SNAPSHOT_CACHE_KEY
	// secret; without it each process uses a random key (its own copies only).
	MACKey []byte
	Now    func() time.Time
}

type memEntry struct {
	snap    *Snapshot
	sha     string
	expires time.Time
}

// Store loads snapshots: in-process copy, then Redis, then the database, verifying the hash of
// every copy it uses.
type Store struct {
	src  Source
	rdb  *redis.Client // may be nil: database and in-process copy only
	log  *slog.Logger
	opts Options

	mu       sync.Mutex
	live     map[string]memEntry  // event -> live version
	versions map[string]memEntry  // event@version
	missing  map[string]time.Time // event -> until when it is known to have no live snapshot
	flight   singleflight.Group   // one database load per event at a time
}

var (
	loadsDB        = metrics.NewCounter("eps_snapshot_db_loads_total", "Snapshot loads from the database.")
	hashMismatches = metrics.NewCounter("eps_snapshot_hash_mismatch_total", "Cached snapshots discarded because their content did not match the hash.")
)

// Default cache lifetimes: a lost activation is healed within DefaultMemTTL + DefaultRedisTTL
// (or DefaultMissingTTL + DefaultMemTTL for an event first published while it was asked for).
const (
	DefaultMemTTL     = 5 * time.Second
	DefaultRedisTTL   = 4 * time.Second
	DefaultMissingTTL = 4 * time.Second
)

// NewStore builds a store over a source and an optional Redis client.
func NewStore(src Source, rdb *redis.Client, log *slog.Logger, o Options) *Store {
	if o.MemTTL <= 0 {
		o.MemTTL = DefaultMemTTL
	}
	if o.RedisTTL <= 0 {
		o.RedisTTL = DefaultRedisTTL
	}
	if o.VersionTTL <= 0 {
		o.VersionTTL = time.Hour
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.MissingTTL <= 0 {
		o.MissingTTL = DefaultMissingTTL
	}
	if log == nil {
		log = slog.Default()
	}
	if len(o.MACKey) == 0 {
		o.MACKey = make([]byte, 32)
		_, _ = rand.Read(o.MACKey)
		if rdb != nil {
			log.Warn("SNAPSHOT_CACHE_KEY not set: Redis copies are signed with a per-process key")
		}
	}
	return &Store{src: src, rdb: rdb, log: log, opts: o, live: map[string]memEntry{}, versions: map[string]memEntry{}, missing: map[string]time.Time{}}
}

// Sign returns the signature of a cached value (hex HMAC-SHA256 under the cache key).
func (s *Store) Sign(parts ...string) string {
	m := hmac.New(sha256.New, s.opts.MACKey)
	for _, p := range parts {
		m.Write([]byte(p))
		m.Write([]byte{0})
	}
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a signature made by Sign.
func (s *Store) Verify(sig string, parts ...string) bool {
	return hmac.Equal([]byte(sig), []byte(s.Sign(parts...)))
}

func missingKey(eventID string) string {
	k, _ := redisx.PlatformKey("snapshot-missing", eventID)
	return k
}

// isMissing reports a recent "no live snapshot" answer for the event (in process, then Redis).
func (s *Store) isMissing(ctx context.Context, eventID string) bool {
	now := s.opts.Now()
	s.mu.Lock()
	until, ok := s.missing[eventID]
	s.mu.Unlock()
	if ok && now.Before(until) {
		return true
	}
	if s.rdb == nil {
		return false
	}
	v, err := s.rdb.Get(ctx, missingKey(eventID)).Result()
	if err != nil || !s.Verify(v, "missing", eventID) {
		return false
	}
	s.mu.Lock()
	s.missing[eventID] = now.Add(s.opts.MissingTTL)
	s.mu.Unlock()
	return true
}

func (s *Store) markMissing(ctx context.Context, eventID string) {
	s.mu.Lock()
	if len(s.missing) > 10_000 { // bounded: a flood of random ids cannot grow it without limit
		s.missing = map[string]time.Time{}
	}
	s.missing[eventID] = s.opts.Now().Add(s.opts.MissingTTL)
	s.mu.Unlock()
	if s.rdb != nil {
		_ = s.rdb.Set(ctx, missingKey(eventID), s.Sign("missing", eventID), s.opts.MissingTTL).Err()
	}
}

func (s *Store) clearMissing(ctx context.Context, eventID string) {
	s.mu.Lock()
	delete(s.missing, eventID)
	delete(s.live, eventID)
	s.mu.Unlock()
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, missingKey(eventID)).Err()
	}
}

func hashOf(doc []byte) string {
	h := sha256.Sum256(doc)
	return hex.EncodeToString(h[:])
}

// decode verifies the hash and the document's own claims about itself.
func decode(st Stored, eventID string) (*Snapshot, error) {
	if hashOf(st.Doc) != st.SHA256 {
		return nil, errHash
	}
	s, err := Parse(st.Doc)
	if err != nil {
		return nil, err
	}
	if s.EventID != eventID || s.Version != st.Version || s.OrganiserID != st.OrganiserID {
		return nil, fmt.Errorf("%w: document does not match its row", ErrInvalid)
	}
	return s, nil
}

var errHash = errors.New("snapshot: content does not match its hash")

func orgKey(eventID string) string {
	k, _ := redisx.PlatformKey("snapshot-org", eventID)
	return k
}

func liveKey(org, eventID string) (string, error) {
	return redisx.Key(org, eventID, "snapshot", "live")
}
func versionKey(org, eventID string, v int) (string, error) {
	return redisx.Key(org, eventID, "snapshot", "v"+strconv.Itoa(v))
}

// Load returns the live snapshot of an event.
func (s *Store) Load(ctx context.Context, eventID string) (*Snapshot, error) {
	now := s.opts.Now()
	s.mu.Lock()
	if e, ok := s.live[eventID]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.snap, nil
	}
	s.mu.Unlock()
	if s.isMissing(ctx, eventID) {
		return nil, ErrNotFound
	}
	if snap, sha, ok := s.fromRedisLive(ctx, eventID); ok {
		s.remember(eventID, snap, sha, s.opts.MemTTL)
		return snap, nil
	}
	// One database load per event at a time, however many requests miss together.
	v, err, _ := s.flight.Do(eventID, func() (any, error) { return s.loadLiveFromDB(context.WithoutCancel(ctx), eventID) })
	if err != nil {
		return nil, err
	}
	return v.(*Snapshot), nil
}

func (s *Store) loadLiveFromDB(ctx context.Context, eventID string) (*Snapshot, error) {
	loadsDB.Inc()
	st, err := s.src.Fetch(ctx, eventID, 0)
	if errors.Is(err, ErrNotFound) {
		s.markMissing(ctx, eventID)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	snap, err := decode(st, eventID)
	if err != nil {
		// The database row itself is wrong: refuse to sell from it.
		s.log.Error("stored snapshot rejected", "event", eventID, "version", st.Version, "error", err.Error())
		return nil, err
	}
	s.toRedisLive(ctx, st)
	s.remember(eventID, snap, st.SHA256, s.opts.MemTTL)
	return snap, nil
}

func (s *Store) remember(eventID string, snap *Snapshot, sha string, ttl time.Duration) {
	s.mu.Lock()
	s.live[eventID] = memEntry{snap: snap, sha: sha, expires: s.opts.Now().Add(ttl)}
	s.mu.Unlock()
}

func (s *Store) fromRedisLive(ctx context.Context, eventID string) (*Snapshot, string, bool) {
	if s.rdb == nil {
		return nil, "", false
	}
	org, err := s.rdb.Get(ctx, orgKey(eventID)).Result()
	if err != nil {
		return nil, "", false
	}
	key, err := liveKey(org, eventID)
	if err != nil {
		return nil, "", false
	}
	return s.fromRedis(ctx, key, eventID, org)
}

func (s *Store) fromRedis(ctx context.Context, key, eventID, org string) (*Snapshot, string, bool) {
	vals, err := s.rdb.HMGet(ctx, key, "version", "sha256", "doc", "mac").Result()
	if err != nil || len(vals) != 4 || vals[0] == nil || vals[1] == nil || vals[2] == nil {
		return nil, "", false
	}
	v, _ := strconv.Atoi(fmt.Sprint(vals[0]))
	st := Stored{OrganiserID: org, Version: v, SHA256: fmt.Sprint(vals[1]), Doc: []byte(fmt.Sprint(vals[2]))}
	snap, err := decode(st, eventID)
	if err == nil && (vals[3] == nil || !s.Verify(fmt.Sprint(vals[3]), "snapshot", org, eventID, strconv.Itoa(st.Version), st.SHA256)) {
		// Not written by a core holding the cache key: never trusted, whatever its hash says.
		err = errors.New("snapshot: cached copy not signed with the cache key")
	}
	if err != nil {
		// A cached copy that does not match its hash (or its key) is dropped and reloaded.
		hashMismatches.Inc()
		s.log.Warn("cached snapshot discarded", "event", eventID, "key", key, "error", err.Error())
		_ = s.rdb.Del(ctx, key).Err()
		return nil, "", false
	}
	return snap, st.SHA256, true
}

func (s *Store) toRedisLive(ctx context.Context, st Stored) {
	if s.rdb == nil {
		return
	}
	snap, _ := Parse(st.Doc)
	if snap == nil {
		return
	}
	key, err := liveKey(st.OrganiserID, snap.EventID)
	if err != nil {
		return
	}
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, orgKey(snap.EventID), st.OrganiserID, 24*time.Hour)
	pipe.HSet(ctx, key, "version", st.Version, "sha256", st.SHA256, "doc", string(st.Doc), "mac", s.Sign("snapshot", st.OrganiserID, snap.EventID, strconv.Itoa(st.Version), st.SHA256))
	pipe.PExpire(ctx, key, s.opts.RedisTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		s.log.Warn("snapshot cache write failed", "event", snap.EventID, "error", err.Error())
	}
}

// LoadVersion returns one published version of an event unchanged (orders in flight name it).
func (s *Store) LoadVersion(ctx context.Context, eventID string, version int) (*Snapshot, error) {
	if version < 1 {
		return nil, ErrNotFound
	}
	mk := eventID + "@" + strconv.Itoa(version)
	now := s.opts.Now()
	s.mu.Lock()
	if e, ok := s.versions[mk]; ok && now.Before(e.expires) {
		s.mu.Unlock()
		return e.snap, nil
	}
	s.mu.Unlock()
	if s.rdb != nil {
		if org, err := s.rdb.Get(ctx, orgKey(eventID)).Result(); err == nil {
			if key, err := versionKey(org, eventID, version); err == nil {
				if snap, sha, ok := s.fromRedis(ctx, key, eventID, org); ok && snap.Version == version {
					s.rememberVersion(mk, snap, sha)
					return snap, nil
				}
			}
		}
	}
	loadsDB.Inc()
	st, err := s.src.Fetch(ctx, eventID, version)
	if err != nil {
		return nil, err
	}
	snap, err := decode(st, eventID)
	if err != nil {
		s.log.Error("stored snapshot rejected", "event", eventID, "version", version, "error", err.Error())
		return nil, err
	}
	if s.rdb != nil {
		if key, err := versionKey(st.OrganiserID, eventID, version); err == nil {
			pipe := s.rdb.TxPipeline()
			pipe.Set(ctx, orgKey(eventID), st.OrganiserID, 24*time.Hour)
			pipe.HSet(ctx, key, "version", st.Version, "sha256", st.SHA256, "doc", string(st.Doc), "mac", s.Sign("snapshot", st.OrganiserID, eventID, strconv.Itoa(st.Version), st.SHA256))
			pipe.Expire(ctx, key, s.opts.VersionTTL)
			_, _ = pipe.Exec(ctx)
		}
	}
	s.rememberVersion(mk, snap, st.SHA256)
	return snap, nil
}

func (s *Store) rememberVersion(key string, snap *Snapshot, sha string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.versions) > 1000 { // bounded: old versions are rare
		s.versions = map[string]memEntry{}
	}
	s.versions[key] = memEntry{snap: snap, sha: sha, expires: s.opts.Now().Add(time.Minute)}
}

// Activate is told by the control plane that a publish made `version` live: it loads that version
// (404 for another organiser's event or an unknown version), then refreshes the live copy from the
// database, so this core serves it on its next request and the others within MemTTL.
func (s *Store) Activate(ctx context.Context, organiserID, eventID string, version int) (string, error) {
	st, err := s.src.Fetch(ctx, eventID, version)
	if err != nil {
		return "", err
	}
	if st.OrganiserID != organiserID {
		return "", ErrNotFound
	}
	if _, err := decode(st, eventID); err != nil {
		return "", err
	}
	s.clearMissing(ctx, eventID)
	if _, err := s.loadLiveFromDB(ctx, eventID); err != nil {
		return "", err
	}
	return st.SHA256, nil
}

// OrganiserOf returns the organiser of an event with a published snapshot ("" when none).
func (s *Store) OrganiserOf(ctx context.Context, eventID string) (string, error) {
	snap, err := s.Load(ctx, eventID)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return snap.OrganiserID, nil
}
