package queue

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// Proof of work (D-2): SHA-256(sale ":" sid ":" nonce) must start with this many zero bits.
const PowBits = 16

// Queue states kept in Redis (absent means the line is not drawn yet).
const (
	StateShuffling = "shuffling"
	StateOpen      = "open"
)

// Place states the fan's page shows.
const (
	PlaceBefore    = "before"    // the pre-queue has not opened: no place taken
	PlaceNotOpen   = "notopen"   // queue off, sale not open yet: no place taken
	PlacePrequeue  = "prequeue"  // in the pre-queue: the order is drawn at opening
	PlaceShuffling = "shuffling" // the draw is running
	PlaceWaiting   = "waiting"   // in line, with a position
	PlaceAdmitted  = "admitted"  // a pass is ready
	PlaceEnded     = "ended"     // the pass was used or ended
)

// FetchWindow is how long an admitted fan has to collect its pass before its shopping place goes to
// the next fan (review spec M3).
const FetchWindow = 60 * time.Second

// MaxSessionsPerPlace bounds the browsers (sessions) one place hands passes to (review D-d).
const MaxSessionsPerPlace = 4

var (
	joins      = metrics.NewCounter("eps_queue_joins_total", "Places taken in waiting rooms.")
	rejoins    = metrics.NewCounter("eps_queue_rejoins_total", "Joins that returned an existing place.")
	admittedC  = metrics.NewCounter("eps_queue_admitted_total", "Fans admitted from waiting rooms.")
	passesC    = metrics.NewCounter("eps_queue_passes_issued_total", "Passes signed for admitted fans.")
	joinLimit  = map[string]*metrics.Counter{}
	shuffles   = metrics.NewCounter("eps_queue_shuffles_total", "Pre-queues drawn at opening.")
	penalised  = metrics.NewCounter("eps_queue_suspects_moved_total", "Suspect places moved to the back after the draw.")
	slowLaned  = metrics.NewCounter("eps_queue_slow_lane_total", "Places moved to the slow lane.")
	statusHits = metrics.NewCounter("eps_queue_status_total", "Status answers.")
	unfetched  = metrics.NewCounter("eps_queue_passes_unfetched_total", "Admissions whose pass was not collected in time (the place went to the next fan).")
)

func init() {
	// Joins refused by the per-address limits, by range (review spec M10).
	for _, k := range []string{"address", "subnet", "new_guests"} {
		joinLimit[k] = metrics.NewCounter("eps_queue_join_rate_limited_total", "Joins refused by the per-address limits.", "range", k)
	}
}

// Engine runs the waiting rooms on Redis. The gateway holds the Signer; the core only verifies.
type Engine struct {
	Redis  *redis.Client
	Sales  *sales.Service
	Signer *Signer
	Scorer *botdefence.Scorer
	// Now is a test clock; nil means Redis's own clock decides opening and arrival (review D-b).
	Now func() time.Time
	// JoinsPerMinute limits joins per client address (D-5; 0 means 10) and per /24 (×20).
	JoinsPerMinute int
	// NewGuestsPerMinute limits new guest places per address (review D-f; 0 means 5).
	NewGuestsPerMinute int
	// Secret keys the stored identity and address hashes (review sec L4); empty in tests is fine.
	Secret []byte
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// testNow is the clock the scripts use: the test clock, else 0 (the script asks Redis TIME).
func (e *Engine) testNow() int64 {
	if e.Now != nil {
		return e.Now().UnixMilli()
	}
	return 0
}

// Keys of one sale (one hash tag, with a short form of the sale id: review D-h).
type Keys struct{ org, event, sale, tag string }

// ShortSale is the sale's part of the tag: its first 12 hex digits.
func ShortSale(sale string) string {
	s := strings.ReplaceAll(sale, "-", "")
	if len(s) > 12 {
		s = s[:12]
	}
	return s
}

// KeysOf names a sale's keys.
func KeysOf(org, event, sale string) (Keys, error) {
	tag := ShortSale(sale)
	if _, err := redisx.HotKey(org, event, tag, "q"); err != nil || tag == "" {
		return Keys{}, errors.New("queue: bad sale key")
	}
	return Keys{org, event, sale, tag}, nil
}

func (k Keys) k(kind string, rest ...string) string {
	key, err := redisx.HotKey(k.org, k.event, k.tag, kind, rest...)
	if err != nil {
		panic(fmt.Sprintf("queue: bad key part: %v", err))
	}
	return key
}

func (k Keys) prefix(kind string) string {
	s := k.k(kind, "x")
	return s[:len(s)-1]
}

// Key names (short: one sale holds a key per fan).
func (k Keys) State() string            { return k.k("qs") } // hash: state, tail, tokens, last, admc, draw...
func (k Keys) Pre() string              { return k.k("qp") }
func (k Keys) Line() string             { return k.k("ql") }
func (k Keys) Slow() string             { return k.k("qw") }
func (k Keys) Active() string           { return k.k("qa") }
func (k Keys) Members() string          { return k.k("qm") } // the draw's member snapshot
func (k Keys) Rate() string             { return k.k("qr") } // admissions per tick, last minute
func (k Keys) Audit() string            { return k.k("qu") }
func (k Keys) Fan(qid string) string    { return k.k("qf", qid) } // one hash per place: entry, signals, passes
func (k Keys) Bot(qid string) string    { return k.Fan(qid) }
func (k Keys) Pass(qid string) string   { return k.Fan(qid) }
func (k Keys) Place(who string) string  { return k.k("qo", who) }
func (k Keys) lock(kind string) string  { return k.k("qx", kind) }
func (k Keys) rl(rest ...string) string { return k.k("qz", rest...) }

// salesIndex lists every sale with a waiting room (the gateway's admitter walks it).
func salesIndex() string {
	k, _ := redisx.PlatformKey("queue-sales")
	return k
}

// PowOK checks the page's proof of work for a session.
func PowOK(saleID, sid, nonce string) bool {
	if nonce == "" || len(nonce) > 64 {
		return false
	}
	h := sha256.Sum256([]byte(saleID + ":" + sid + ":" + nonce))
	z := 0
	for _, b := range h {
		if b == 0 {
			z += 8
			continue
		}
		z += bits.LeadingZeros8(b)
		break
	}
	return z >= PowBits
}

// SolvePow finds a nonce (tests and the load scripts; the page does the same in JavaScript).
func SolvePow(saleID, sid string) string {
	prefix := []byte(saleID + ":" + sid + ":")
	buf := make([]byte, 0, len(prefix)+20)
	for i := 0; ; i++ {
		buf = strconv.AppendInt(append(buf[:0], prefix...), int64(i), 10)
		h := sha256.Sum256(buf)
		if h[0] == 0 && h[1] == 0 {
			return strconv.Itoa(i)
		}
	}
}

// joinScript takes (or finds) a place, deciding by Redis state and Redis's own clock (review D-b):
// an open line takes the fan at its back whatever the caller's clock said; before the draw the fan
// waits in the pre-queue with its arrival time (an arrival at or after opening is a latecomer the
// draw places behind, in arrival order).
// KEYS: place, state, pre, line, fan, active.
// ARGV: new qid, test now (0: TIME), queue on, opens at ms, pass life ms, who, sid, address hash,
// fetch window ms.
// Returns {qid, fresh (1|0), where, now}.
var joinScript = redis.NewScript(`
local now = tonumber(ARGV[2])
if now == 0 then local t = redis.call('TIME'); now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000) end
local cur = redis.call('GET', KEYS[1])
if cur then
  local f = redis.call('HMGET', KEYS[5] .. cur, 'adm', 'pst', 'pf')
  local ended = false
  if f[1] then
    local adm = tonumber(f[1])
    if f[2] == 'used' or f[2] == 'revoked' or now >= adm + tonumber(ARGV[5]) then ended = true end
    if not f[3] and now >= adm + tonumber(ARGV[9]) then ended = true end
  elseif redis.call('EXISTS', KEYS[5] .. cur) == 0 then
    ended = true -- the place is gone (a store reset)
  end
  if not ended then return {cur, 0, f[1] and 'admitted' or 'kept', now} end
end
local qid = ARGV[1]
local fan = KEYS[5] .. qid
redis.call('SET', KEYS[1], qid, 'EX', 172800)
redis.call('HSET', fan, 'who', ARGV[6], 'sid', ARGV[7], 'ip', ARGV[8], 'joined', now)
redis.call('EXPIRE', fan, 172800)
local state = redis.call('HGET', KEYS[2], 'state')
if ARGV[3] == '0' then
  if now < tonumber(ARGV[4]) then return {qid, 1, 'notopen', now} end
  redis.call('HSET', fan, 'adm', now)
  redis.call('ZADD', KEYS[6], now + tonumber(ARGV[9]), qid)
  redis.call('HINCRBY', KEYS[2], 'admc', 1)
  return {qid, 1, 'admitted', now}
end
if state == 'open' then
  local pos = redis.call('HINCRBY', KEYS[2], 'tail', 1)
  redis.call('ZADD', KEYS[4], pos, qid)
  redis.call('HSET', fan, 'pos', pos)
  return {qid, 1, 'line', now}
end
redis.call('ZADD', KEYS[3], now, qid)
return {qid, 1, 'pre', now}
`)

// JoinInput is what the gateway knows of a join.
type JoinInput struct {
	Sub     string // account id or guest:<id>
	Kind    string // account | guest
	Session string // the session secret (X-EPS-Session)
	Pow     string
	Print   string // the page's browser fingerprint hash (optional)
	UA      string
	IP      string
	Tab     string // the page's tab id (polling is scored per tab)
	// SimRehearsal is the rehearsal event a simulator token names ("" for every other token).
	SimRehearsal string
}

// Status is what the waiting room shows.
type Status struct {
	State           string    `json:"state"`
	SaleID          string    `json:"saleId"`
	EventID         string    `json:"eventId"`
	OpensAt         time.Time `json:"opensAt"`
	PrequeueOpensAt time.Time `json:"prequeueOpensAt"`
	QueueToken      string    `json:"queueToken,omitempty"`
	Ahead           *int64    `json:"ahead,omitempty"`
	EstimateSeconds *int64    `json:"estimateSeconds,omitempty"`
	Paused          bool      `json:"paused"`
	// Step 6: degraded mode (no admissions, places kept) and a rehearsal with simulated fans.
	Degraded      bool      `json:"degraded,omitempty"`
	Rehearsal     bool      `json:"rehearsal,omitempty"`
	Pass          string    `json:"pass,omitempty"`
	PassExpiresAt *int64    `json:"passExpiresAt,omitempty"`
	NextPollMs    int64     `json:"nextPollMs"`
	ServerTime    time.Time `json:"serverTime"`
}

// ErrRateLimited means too many joins from one address (429 with Retry-After).
var ErrRateLimited = errors.New("queue: too many joins from this address")

// ErrBadToken means the queue token is not valid for this sale and session.
var ErrBadToken = errors.New("queue: queue token not valid")

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IdentityHash is a place's owner as stored: a keyed hash of the sale and the account or guest id.
func IdentityHash(secret []byte, sale, sub string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("place|" + sale + "|" + sub))
	return hex.EncodeToString(m.Sum(nil)[:12])
}

// addrHash is a keyed hash of an address unit (never the address itself; review D-h).
func (e *Engine) addrHash(kind, v string) string {
	m := hmac.New(sha256.New, e.Secret)
	m.Write([]byte(kind + "|" + v))
	return hex.EncodeToString(m.Sum(nil)[:8])
}

// shortSid is the stored form of a session hash.
func shortSid(sid string) string {
	if len(sid) > 32 {
		return sid[:32]
	}
	return sid
}

func (e *Engine) limit(ctx context.Context, key string, max int64, window time.Duration) bool {
	n, err := e.Redis.Incr(ctx, key).Result()
	if err != nil {
		return true
	}
	if n == 1 {
		_ = e.Redis.Expire(ctx, key, window+5*time.Second).Err()
	}
	return n <= max
}

// jitter is a random duration in [lo, hi).
func jitter(lo, hi time.Duration) time.Duration {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := int64(b[0])<<24 | int64(b[1])<<16 | int64(b[2])<<8 | int64(b[3])
	return lo + time.Duration(n%int64(hi-lo))
}

// Join takes the fan's place (or returns the one it has): before the pre-queue nothing is taken;
// in the pre-queue window the fan waits for the draw; after the draw the fan joins the back of the
// line; with the queue off a pass is ready at once. Bot signals are scored silently.
func (e *Engine) Join(ctx context.Context, saleID string, in JoinInput) (*Status, error) {
	sale, err := e.Sales.Lookup(ctx, saleID)
	if err != nil {
		return nil, err
	}
	st := sale.Settings
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return nil, err
	}
	now := e.now()
	// A simulated fan joins only the rehearsal its token names (step 6, design D-3).
	if in.SimRehearsal != "" && (sale.Rehearsal == nil || sale.EventID != in.SimRehearsal) {
		return nil, apperr.New(http.StatusForbidden, apperr.SimRefused, "Simulated fans may only join their own rehearsal.", false)
	}
	base := &Status{SaleID: sale.ID, EventID: sale.EventID, OpensAt: st.OpensAt, PrequeueOpensAt: st.PrequeueOpensAt(), ServerTime: now,
		Degraded: sale.Degraded, Rehearsal: sale.Rehearsal != nil}
	if st.PhaseAt(now) == sales.Before {
		base.State = PlaceBefore
		// Come back at the opening of the pre-queue, spread over a few seconds (review D-i).
		base.NextPollMs = min(60_000, max(1_000, st.PrequeueOpensAt().Sub(now).Milliseconds())) + jitter(0, 5*time.Second).Milliseconds()
		return base, nil
	}
	if in.Sub == "" || len(in.Session) < 16 || len(in.Session) > 200 {
		return nil, ErrBadToken
	}
	if st.AccountsOnly && in.Kind != "account" {
		return nil, apperr.New(403, apperr.SignInRequired, "Sign in to join this waiting room.", false)
	}
	sid := SessionHash(in.Session)
	who := IdentityHash(e.Secret, sale.ID, in.Sub)
	// A rejoin of a live place costs nothing against the join limits (review sec L2).
	existing, _ := e.Redis.Get(ctx, k.Place(who)).Result()
	addr := botdefence.Address(in.IP)
	if existing == "" && addr != "" {
		per := int64(e.JoinsPerMinute)
		if per <= 0 {
			per = 10
		}
		minute := strconv.FormatInt(now.Unix()/60, 10)
		if !e.limit(ctx, k.rl("ip", e.addrHash("a", addr), minute), per, time.Minute) {
			joinLimit["address"].Inc()
			return nil, ErrRateLimited
		}
		if sn := botdefence.Subnet(in.IP); sn != "" && !e.limit(ctx, k.rl("sn", e.addrHash("s", sn), minute), per*20, time.Minute) {
			joinLimit["subnet"].Inc()
			return nil, ErrRateLimited
		}
		// New guest identities from one address (review D-f: guest identities are free to make).
		if in.Kind == "guest" {
			g := int64(e.NewGuestsPerMinute)
			if g <= 0 {
				g = 5
			}
			if !e.limit(ctx, k.rl("ng", e.addrHash("a", addr), minute), g, time.Minute) {
				joinLimit["new_guests"].Inc()
				return nil, ErrRateLimited
			}
		}
	}
	qid := newID()
	queueOn := "1"
	if !st.QueueOn {
		queueOn = "0"
	}
	res, err := joinScript.Run(ctx, e.Redis, []string{k.Place(who), k.State(), k.Pre(), k.Line(), k.prefix("qf"), k.Active()},
		qid, e.testNow(), queueOn, st.OpensAt.UnixMilli(), PassLife.Milliseconds(), who, shortSid(sid), e.addrHash("a", addr), FetchWindow.Milliseconds()).Slice()
	if err != nil {
		return nil, err
	}
	qid = fmt.Sprint(res[0])
	fresh := res[1].(int64) == 1
	where := fmt.Sprint(res[2])
	if e.Now == nil {
		now = time.UnixMilli(res[3].(int64))
	}
	if fresh {
		joins.Inc()
		_ = e.Redis.SAdd(ctx, salesIndex(), sale.OrganiserID+"/"+sale.EventID+"/"+sale.ID).Err()
		b, _ := json.Marshal(map[string]any{"t": "join", "qid": qid, "who": who, "at": now.UnixMilli()})
		_ = e.Redis.RPush(ctx, k.Audit(), b).Err()
		if where == "admitted" {
			admittedC.Inc()
			b, _ := json.Marshal(map[string]any{"t": "admit", "qid": qid, "at": now.UnixMilli()})
			_ = e.Redis.RPush(ctx, k.Audit(), b).Err()
		}
		if st.BotDefence && e.Scorer != nil {
			e.scoreJoin(ctx, k, sale.ID, qid, sid, in)
		}
	} else {
		rejoins.Inc()
	}
	if where == "notopen" {
		base.State = PlaceNotOpen
		base.NextPollMs = min(30_000, max(500, st.OpensAt.Sub(now).Milliseconds())) + jitter(2*time.Second, 15*time.Second).Milliseconds()
		return base, nil
	}
	tok := e.Signer.Sign(Claims{Kind: KindQueue, Org: sale.OrganiserID, Evt: sale.EventID, Sale: sale.ID, Sub: in.Sub, Sid: sid, QID: qid,
		Iat: now.Unix(), Exp: now.Add(QueueTokenLife).Unix()})
	out, err := e.status(ctx, sale, k, qid, in.Sub, sid, tabOf(in.Tab), now)
	if err != nil {
		return nil, err
	}
	out.QueueToken = tok
	return out, nil
}

func tabOf(t string) string {
	if t == "" || len(t) > 16 || strings.ContainsAny(t, ":{} ") {
		return "-"
	}
	return t
}

// scoreJoin records the join's signals (D-6 with review D-e): proof of work, automation markers, a
// hosting address, many places from one address (or /64) or one /24 (or /48), one browser print
// shared by many people.
func (e *Engine) scoreJoin(ctx context.Context, k Keys, saleID, qid, sid string, in JoinInput) {
	bk := k.Bot(qid)
	add := func(s botdefence.Signal) { _, _ = e.Scorer.Add(ctx, bk, s) }
	if !PowOK(saleID, sid, in.Pow) {
		add(botdefence.NoProofOfWork)
	}
	if botdefence.IsHeadless(in.UA) {
		add(botdefence.Headless)
	}
	if botdefence.InDataCentre(in.IP) {
		add(botdefence.DataCentre)
	}
	count := func(kind, v string) (string, int64) {
		key := k.k(kind, e.addrHash(kind, v))
		pipe := e.Redis.Pipeline()
		pipe.SAdd(ctx, key, qid)
		pipe.Expire(ctx, key, time.Hour)
		n := pipe.SCard(ctx, key)
		if _, err := pipe.Exec(ctx); err != nil {
			return key, 0
		}
		return key, n.Val()
	}
	// Address signals rise by stages; when a stage is crossed the earlier places get it too.
	raise := func(key string, n int64, at int64, sig botdefence.Signal) {
		if n <= at {
			return
		}
		_, _ = e.Scorer.Raise(ctx, bk, sig)
		if n == at+1 {
			for _, other := range e.Redis.SMembers(ctx, key).Val() {
				if other != qid {
					_, _ = e.Scorer.Raise(ctx, k.Bot(other), sig)
				}
			}
		}
	}
	if a := botdefence.Address(in.IP); a != "" {
		key, n := count("qi", a)
		raise(key, n, botdefence.ManyPerIPAt, botdefence.ManyPerIP)
		raise(key, n, botdefence.BusyIPAt, botdefence.BusyIP)
	}
	if sn := botdefence.Subnet(in.IP); sn != "" {
		key, n := count("qn", sn)
		raise(key, n, botdefence.ManyPerSubnetAt, botdefence.ManyPerSubnet)
	}
	if in.Print != "" && len(in.Print) <= 128 {
		key := k.k("qk", e.addrHash("p", in.Print))
		pipe := e.Redis.Pipeline()
		pipe.SAdd(ctx, key, IdentityHash(e.Secret, saleID, in.Sub))
		pipe.Expire(ctx, key, time.Hour)
		n := pipe.SCard(ctx, key)
		if _, err := pipe.Exec(ctx); err == nil && n.Val() > botdefence.SharedPrintAt {
			add(botdefence.SharedPrint)
		}
	}
}

// Verifier checks the engine's own tokens.
func (e *Engine) Verifier() *Verifier { return NewVerifier(e.Signer.Key.Public().(ed25519.PublicKey)) }

// StatusOf answers a poll with the queue token alone (review D-i: no core token, no session read):
// the token is bound to the browser's session secret. A poll faster than every 2 s gets the last
// answer again (and may count as a bot signal).
func (e *Engine) StatusOf(ctx context.Context, saleID, queueToken, session, tab string) (*Status, error) {
	if e.Signer == nil {
		return nil, ErrBadToken
	}
	c, err := e.Verifier().Verify(queueToken, KindQueue, e.now(), false)
	if err != nil || c.Sale != saleID || c.Sid != SessionHash(session) {
		return nil, ErrBadToken
	}
	sale, err := e.Sales.Lookup(ctx, saleID)
	if err != nil {
		return nil, err
	}
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return nil, err
	}
	now := e.now()
	tab = tabOf(tab)
	if sale.Settings.BotDefence && e.Scorer != nil {
		_, _ = e.Scorer.Poll(ctx, k.Bot(c.QID), tab, now.UnixMilli())
	}
	cacheKey := k.rl("st", c.QID, shortSid(c.Sid))
	if last, err := e.Redis.Get(ctx, cacheKey).Result(); err == nil {
		var s Status
		if json.Unmarshal([]byte(last), &s) == nil && now.Sub(s.ServerTime) < 2*time.Second && now.Sub(s.ServerTime) >= 0 {
			s.ServerTime = now
			return &s, nil
		}
	}
	s, err := e.status(ctx, sale, k, c.QID, c.Sub, c.Sid, tab, now)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(s)
	_ = e.Redis.Set(ctx, cacheKey, b, 2*time.Second).Err()
	return s, nil
}

// passScript hands the place's pass for one session (review D-d): one place, a pass per session
// (at most MaxSessionsPerPlace), issued once per session; collecting the first pass moves the
// shopping place's end from the fetch window to the pass's own end.
// KEYS: fan, active. ARGV: sid, token (or "" to read only), now ms, pass end ms, qid, max sessions.
// Returns {state, token}: state ok | sign (no token yet: sign one) | ended | full.
var passScript = redis.NewScript(`
local f = redis.call('HMGET', KEYS[1], 'adm', 'pst', 'p:' .. ARGV[1], 'np')
if not f[1] then return {'none', ''} end
if f[2] == 'used' or f[2] == 'revoked' or tonumber(ARGV[3]) >= tonumber(ARGV[4]) then return {'ended', ''} end
if f[3] then return {'ok', f[3]} end
if ARGV[2] == '' then return {'sign', ''} end
local n = tonumber(f[4] or '0')
if n >= tonumber(ARGV[6]) then return {'full', ''} end
redis.call('HSET', KEYS[1], 'p:' .. ARGV[1], ARGV[2], 'np', n + 1, 'pst', 'active', 'pf', ARGV[3])
redis.call('ZADD', KEYS[2], 'XX', 'GT', ARGV[4], ARGV[5])
return {'ok', ARGV[2]}
`)

// status works out a place's state at now.
func (e *Engine) status(ctx context.Context, sale *sales.Sale, k Keys, qid, sub, sid, tab string, now time.Time) (*Status, error) {
	st := sale.Settings
	out := &Status{SaleID: sale.ID, EventID: sale.EventID, OpensAt: st.OpensAt, PrequeueOpensAt: st.PrequeueOpensAt(), ServerTime: now, Paused: st.Paused,
		Degraded: sale.Degraded, Rehearsal: sale.Rehearsal != nil}
	statusHits.Inc()
	pipe := e.Redis.Pipeline()
	fan := pipe.HMGet(ctx, k.Fan(qid), "adm", "pf", "pos", "score")
	pre := pipe.ZScore(ctx, k.Pre(), qid)
	rank := pipe.ZRank(ctx, k.Line(), qid)
	score := pipe.ZScore(ctx, k.Line(), qid)
	slow := pipe.ZRank(ctx, k.Slow(), qid)
	lineLen := pipe.ZCard(ctx, k.Line())
	state := pipe.HMGet(ctx, k.State(), "state", "admc")
	rate := pipe.LRange(ctx, k.Rate(), 0, 119)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	fv := fan.Val()
	if fv[0] == nil && pre.Err() != nil && rank.Err() != nil && slow.Err() != nil {
		if s := fmt.Sprint(state.Val()[0]); s == StateShuffling {
			out.State = PlaceShuffling
			out.NextPollMs = jitter(2*time.Second, 4*time.Second).Milliseconds()
			return out, nil
		}
		return nil, ErrBadToken // the place is gone (the store was reset): join again
	}
	defer func() {
		if e.Scorer != nil && out.NextPollMs > 0 {
			e.Scorer.Told(ctx, k.Bot(qid), tab, now.Add(time.Duration(out.NextPollMs)*time.Millisecond).UnixMilli())
		}
	}()
	switch {
	case fv[0] != nil:
		at, _ := strconv.ParseInt(fmt.Sprint(fv[0]), 10, 64)
		exp := time.UnixMilli(at).Add(PassLife)
		if fv[1] == nil && now.After(time.UnixMilli(at).Add(FetchWindow)) {
			// Admitted but nobody collected the pass in time: the shopping place went on (review spec M3).
			out.State = PlaceEnded
			return out, nil
		}
		r, err := passScript.Run(ctx, e.Redis, []string{k.Fan(qid), k.Active()}, shortSid(sid), "", now.UnixMilli(), exp.UnixMilli(), qid, MaxSessionsPerPlace).StringSlice()
		if err != nil {
			return nil, err
		}
		tok := r[1]
		switch r[0] {
		case "ended", "full", "none":
			out.State = PlaceEnded
			return out, nil
		case "sign":
			signed := e.Signer.Sign(Claims{Kind: KindPass, Org: sale.OrganiserID, Evt: sale.EventID, Sale: sale.ID, Sub: sub, Sid: sid, QID: qid,
				Iat: now.Unix(), Exp: exp.Unix()})
			r, err = passScript.Run(ctx, e.Redis, []string{k.Fan(qid), k.Active()}, shortSid(sid), signed, now.UnixMilli(), exp.UnixMilli(), qid, MaxSessionsPerPlace).StringSlice()
			if err != nil {
				return nil, err
			}
			if r[0] != "ok" {
				out.State = PlaceEnded
				return out, nil
			}
			tok = r[1]
			if tok == signed {
				passesC.Inc()
			}
		}
		out.State = PlaceAdmitted
		out.Pass = tok
		ms := exp.UnixMilli()
		out.PassExpiresAt = &ms
		out.NextPollMs = 0
	case pre.Err() == nil:
		out.State = PlacePrequeue
		if st.PhaseAt(now) == sales.Open {
			out.State = PlaceShuffling
		}
		out.NextPollMs = jitter(7*time.Second, 13*time.Second).Milliseconds()
		if d := st.OpensAt.Sub(now); d > 0 && d.Milliseconds() < out.NextPollMs {
			// The first poll after opening is spread over 2 to 15 s (review D-i): no stampede.
			out.NextPollMs = (d + jitter(2*time.Second, 15*time.Second)).Milliseconds()
		} else if out.State == PlaceShuffling {
			out.NextPollMs = jitter(2*time.Second, 6*time.Second).Milliseconds()
		}
	case rank.Err() == nil || slow.Err() == nil:
		ahead := rank.Val()
		admc, _ := strconv.ParseInt(fmt.Sprint(state.Val()[1]), 10, 64)
		moved := rank.Err() != nil || score.Val() >= penaltyBase
		if moved {
			// A suspect sees a normal, slowly moving place (review D-j): its draw position less
			// everyone let in since, so it only ever goes down.
			pos, _ := strconv.ParseInt(fmt.Sprint(fv[2]), 10, 64)
			if pos <= 0 {
				pos = lineLen.Val() + slow.Val()
			}
			ahead = max(1, pos-admc)
		}
		out.State = PlaceWaiting
		out.Ahead = &ahead
		if !st.Paused && !sale.Degraded {
			// Estimate from the admissions actually made in the last minute (review D-j).
			if r := measuredRate(rate.Val(), now); r > 0 {
				est := int64(float64(ahead) / r)
				out.EstimateSeconds = &est
			} else {
				est := ahead * 60 / int64(max(1, st.RatePerMinute))
				out.EstimateSeconds = &est
			}
		}
		out.NextPollMs = jitter(7*time.Second, 13*time.Second).Milliseconds()
		if ahead < int64(st.RatePerMinute)/6 {
			out.NextPollMs = jitter(2*time.Second, 6*time.Second).Milliseconds() // near the front
		}
	}
	return out, nil
}

// measuredRate is admissions per second over the last 60 s, from the admitter's per-tick record
// ("<ms>:<count>", newest first); 0 when nothing was recorded yet.
func measuredRate(ticks []string, now time.Time) float64 {
	from := now.Add(-time.Minute).UnixMilli()
	var n int64
	oldest := now.UnixMilli()
	seen := false
	for _, t := range ticks {
		at, c, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		ms, _ := strconv.ParseInt(at, 10, 64)
		if ms < from {
			break
		}
		v, _ := strconv.ParseInt(c, 10, 64)
		n += v
		oldest = min(oldest, ms)
		seen = true
	}
	if !seen {
		return 0
	}
	span := max(1.0, float64(now.UnixMilli()-oldest)/1000)
	if n == 0 {
		return 0.0001 // admissions stalled (the shopping limit): a long, honest estimate
	}
	return float64(n) / span
}
