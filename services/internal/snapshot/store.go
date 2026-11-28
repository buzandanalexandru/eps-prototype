package snapshot

import (
	"context"
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
	var v *int
	if version > 0 {
		v = &version
	}
	err := db.WithTx(ctx, d.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var doc string
		err := tx.QueryRow(ctx,
			"select organiser_id::text, version, document_text, sha256 from control.live_snapshot($1::uuid, $2::int)",
			eventID, v).Scan(&out.OrganiserID, &out.Version, &doc, &out.SHA256)
		out.Doc = []byte(doc)
		return err
	})
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
	Now        func() time.Time
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
	live     map[string]memEntry // event -> live version
	versions map[string]memEntry // event@version
}

var (
	loadsDB        = metrics.NewCounter("eps_snapshot_db_loads_total", "Snapshot loads from the database.")
	hashMismatches = metrics.NewCounter("eps_snapshot_hash_mismatch_total", "Cached snapshots discarded because their content did not match the hash.")
)

// NewStore builds a store over a source and an optional Redis client.
func NewStore(src Source, rdb *redis.Client, log *slog.Logger, o Options) *Store {
	if o.MemTTL <= 0 {
		o.MemTTL = 5 * time.Second
	}
	if o.RedisTTL <= 0 {
		o.RedisTTL = 4 * time.Second
	}
	if o.VersionTTL <= 0 {
		o.VersionTTL = time.Hour
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	return &Store{src: src, rdb: rdb, log: log, opts: o, live: map[string]memEntry{}, versions: map[string]memEntry{}}
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
	if snap, sha, ok := s.fromRedisLive(ctx, eventID); ok {
		s.remember(eventID, snap, sha, s.opts.MemTTL)
		return snap, nil
	}
	return s.loadLiveFromDB(ctx, eventID)
}

func (s *Store) loadLiveFromDB(ctx context.Context, eventID string) (*Snapshot, error) {
	loadsDB.Inc()
	st, err := s.src.Fetch(ctx, eventID, 0)
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
	vals, err := s.rdb.HMGet(ctx, key, "version", "sha256", "doc").Result()
	if err != nil || len(vals) != 3 || vals[0] == nil || vals[1] == nil || vals[2] == nil {
		return nil, "", false
	}
	v, _ := strconv.Atoi(fmt.Sprint(vals[0]))
	st := Stored{OrganiserID: org, Version: v, SHA256: fmt.Sprint(vals[1]), Doc: []byte(fmt.Sprint(vals[2]))}
	snap, err := decode(st, eventID)
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
	pipe.HSet(ctx, key, "version", st.Version, "sha256", st.SHA256, "doc", string(st.Doc))
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
			pipe.HSet(ctx, key, "version", st.Version, "sha256", st.SHA256, "doc", string(st.Doc))
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
