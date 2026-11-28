package queue

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

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

var (
	joins      = metrics.NewCounter("eps_queue_joins_total", "Places taken in waiting rooms.")
	rejoins    = metrics.NewCounter("eps_queue_rejoins_total", "Joins that returned an existing place.")
	admittedC  = metrics.NewCounter("eps_queue_admitted_total", "Fans admitted from waiting rooms.")
	passesC    = metrics.NewCounter("eps_queue_passes_issued_total", "Passes signed for admitted fans.")
	joinLimit  = metrics.NewCounter("eps_queue_join_rate_limited_total", "Joins refused by the per-address limit.")
	shuffles   = metrics.NewCounter("eps_queue_shuffles_total", "Pre-queues drawn at opening.")
	penalised  = metrics.NewCounter("eps_queue_suspects_moved_total", "Suspect places moved to the back after the draw.")
	slowLaned  = metrics.NewCounter("eps_queue_slow_lane_total", "Places moved to the slow lane.")
	statusHits = metrics.NewCounter("eps_queue_status_total", "Status answers.")
)

// Engine runs the waiting rooms on Redis. The gateway holds the Signer; the core only verifies.
type Engine struct {
	Redis  *redis.Client
	Sales  *sales.Service
	Signer *Signer
	Scorer *botdefence.Scorer
	Now    func() time.Time
	// JoinsPerMinute limits joins per client address (D-5; 0 means 10) and per /24 (×20).
	JoinsPerMinute int
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Keys of one sale (one hash tag).
type Keys struct{ org, event, sale string }

// KeysOf names a sale's keys.
func KeysOf(org, event, sale string) (Keys, error) {
	if _, err := redisx.HotKey(org, event, sale, "q"); err != nil {
		return Keys{}, err
	}
	return Keys{org, event, sale}, nil
}

func (k Keys) k(kind string, rest ...string) string {
	key, err := redisx.HotKey(k.org, k.event, k.sale, kind, rest...)
	if err != nil {
		panic(fmt.Sprintf("queue: bad key part: %v", err))
	}
	return key
}

func (k Keys) prefix(kind string) string {
	s := k.k(kind, "x")
	return s[:len(s)-1]
}

// Exported names for the core and the worker.
func (k Keys) State() string            { return k.k("q-state") }
func (k Keys) Pre() string              { return k.k("q-pre") }
func (k Keys) Line() string             { return k.k("q-line") }
func (k Keys) Slow() string             { return k.k("q-slow") }
func (k Keys) Tail() string             { return k.k("q-tail") }
func (k Keys) Admitted() string         { return k.k("q-admitted") }
func (k Keys) Active() string           { return k.k("q-active") }
func (k Keys) Bucket() string           { return k.k("q-bucket") }
func (k Keys) Shuffle() string          { return k.k("q-shuffle") }
func (k Keys) Audit() string            { return k.k("q-audit") }
func (k Keys) Pass(qid string) string   { return k.k("q-pass", qid) }
func (k Keys) Bot(qid string) string    { return k.k("q-bot", qid) }
func (k Keys) Entry(qid string) string  { return k.k("q-entry", qid) }
func (k Keys) Place(who string) string  { return k.k("q-place", who) }
func (k Keys) lock(kind string) string  { return k.k("q-lock", kind) }
func (k Keys) rl(rest ...string) string { return k.k("q-rl", rest...) }

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
	for i := 0; ; i++ {
		n := strconv.Itoa(i)
		if PowOK(saleID, sid, n) {
			return n
		}
	}
}

// joinScript takes (or finds) a place. KEYS: place, state, pre, line, tail, entry, admitted,
// active. ARGV: new qid, now ms, queue on, sale open, pass life ms, pass key prefix, who, sid, ip.
// Returns {qid, fresh (1|0), where}.
var joinScript = redis.NewScript(`
local now = tonumber(ARGV[2])
local cur = redis.call('GET', KEYS[1])
if cur then
  local adm = redis.call('HGET', KEYS[7], cur)
  local ended = false
  if adm then
    local st = redis.call('HGET', ARGV[6] .. cur, 'state')
    if st == 'used' or st == 'revoked' or now >= tonumber(adm) + tonumber(ARGV[5]) then ended = true end
  end
  if not ended then return {cur, 0, adm and 'admitted' or 'kept'} end
end
local qid = ARGV[1]
redis.call('SET', KEYS[1], qid, 'EX', 172800)
redis.call('HSET', KEYS[6], 'who', ARGV[7], 'sid', ARGV[8], 'ip', ARGV[9], 'joined', now)
redis.call('EXPIRE', KEYS[6], 172800)
if ARGV[3] == '0' then
  redis.call('HSET', KEYS[7], qid, now)
  redis.call('ZADD', KEYS[8], now + tonumber(ARGV[5]), qid)
  return {qid, 1, 'admitted'}
end
if ARGV[4] == '1' and redis.call('GET', KEYS[2]) == 'open' then
  local pos = redis.call('INCR', KEYS[5])
  redis.call('ZADD', KEYS[4], pos, qid)
  return {qid, 1, 'line'}
end
redis.call('ZADD', KEYS[3], now, qid)
return {qid, 1, 'pre'}
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
	Pass            string    `json:"pass,omitempty"`
	PassExpiresAt   *int64    `json:"passExpiresAt,omitempty"`
	NextPollMs      int64     `json:"nextPollMs"`
	ServerTime      time.Time `json:"serverTime"`
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

// IdentityHash is a place's owner as stored (an account or guest id, hashed).
func IdentityHash(sale, sub string) string {
	h := sha256.Sum256([]byte(sale + "|" + sub))
	return hex.EncodeToString(h[:16])
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
	base := &Status{SaleID: sale.ID, EventID: sale.EventID, OpensAt: st.OpensAt, PrequeueOpensAt: st.PrequeueOpensAt(), ServerTime: now, NextPollMs: e.pollDelay(10_000)}
	phase := st.PhaseAt(now)
	if phase == sales.Before {
		base.State = PlaceBefore
		base.NextPollMs = min(60_000, max(1_000, time.Until(st.PrequeueOpensAt()).Milliseconds()))
		return base, nil
	}
	if !st.QueueOn && phase != sales.Open {
		base.State = PlaceNotOpen
		base.NextPollMs = min(30_000, max(500, st.OpensAt.Sub(now).Milliseconds()))
		return base, nil
	}
	if in.Sub == "" || len(in.Session) < 16 || len(in.Session) > 200 {
		return nil, ErrBadToken
	}
	// Join limits per client address (D-5): 10 a minute per address, 200 per /24.
	per := int64(e.JoinsPerMinute)
	if per <= 0 {
		per = 10
	}
	minute := strconv.FormatInt(now.Unix()/60, 10)
	if in.IP != "" {
		if !e.limit(ctx, k.rl("ip", ipPart(in.IP), minute), per, time.Minute) {
			joinLimit.Inc()
			return nil, ErrRateLimited
		}
		if sn := botdefence.Subnet(in.IP); sn != "" && !e.limit(ctx, k.rl("sn", sn, minute), per*20, time.Minute) {
			joinLimit.Inc()
			return nil, ErrRateLimited
		}
	}
	sid := SessionHash(in.Session)
	who := IdentityHash(sale.ID, in.Sub)
	qid := newID()
	open := "0"
	if phase == sales.Open {
		open = "1"
	}
	queueOn := "1"
	if !st.QueueOn {
		queueOn = "0"
	}
	res, err := joinScript.Run(ctx, e.Redis, []string{k.Place(who), k.State(), k.Pre(), k.Line(), k.Tail(), k.Entry(qid), k.Admitted(), k.Active()},
		qid, now.UnixMilli(), queueOn, open, PassLife.Milliseconds(), k.prefix("q-pass"), who, sid, in.IP).Slice()
	if err != nil {
		return nil, err
	}
	qid = fmt.Sprint(res[0])
	fresh := res[1].(int64) == 1
	if fresh {
		joins.Inc()
		_ = e.Redis.SAdd(ctx, salesIndex(), sale.OrganiserID+"/"+sale.EventID+"/"+sale.ID).Err()
		b, _ := json.Marshal(map[string]any{"t": "join", "qid": qid, "who": who, "at": now.UnixMilli()})
		_ = e.Redis.RPush(ctx, k.Audit(), b).Err()
		if fmt.Sprint(res[2]) == "admitted" {
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
	tok := e.Signer.Sign(Claims{Kind: KindQueue, Org: sale.OrganiserID, Evt: sale.EventID, Sale: sale.ID, Sub: in.Sub, Sid: sid, QID: qid,
		Iat: now.Unix(), Exp: now.Add(QueueTokenLife).Unix()})
	out, err := e.status(ctx, sale, k, qid, in.Sub, sid, now)
	if err != nil {
		return nil, err
	}
	out.QueueToken = tok
	return out, nil
}

func ipPart(ip string) string {
	b := []byte(ip)
	for i, c := range b {
		if c == ':' {
			b[i] = '_'
		}
	}
	return string(b)
}

// scoreJoin records the join's signals (D-6): proof of work, automation markers, a hosting
// address, many places from one address or /24, one fingerprint shared by several people.
func (e *Engine) scoreJoin(ctx context.Context, k Keys, saleID, qid, sid string, in JoinInput) {
	bk := k.Bot(qid)
	add := func(s botdefence.Signal) { _, _ = e.Scorer.Add(ctx, bk, s) }
	if !PowOK(saleID, sid, in.Pow) {
		add(botdefence.NoProofOfWork)
	}
	if botdefence.IsHeadless(in.UA) {
		add(botdefence.Headless)
	}
	if in.IP != "" {
		if botdefence.InDataCentre(in.IP) {
			add(botdefence.DataCentre)
		}
		ipk := k.k("q-ip", ipPart(in.IP))
		pipe := e.Redis.Pipeline()
		pipe.SAdd(ctx, ipk, qid)
		pipe.Expire(ctx, ipk, 48*time.Hour)
		n := pipe.SCard(ctx, ipk)
		var sn *redis.IntCmd
		if s := botdefence.Subnet(in.IP); s != "" {
			snk := k.k("q-sn", s)
			pipe.SAdd(ctx, snk, qid)
			pipe.Expire(ctx, snk, 48*time.Hour)
			sn = pipe.SCard(ctx, snk)
		}
		if _, err := pipe.Exec(ctx); err == nil {
			if n.Val() > 3 {
				add(botdefence.ManyPerIP)
				// The earlier places from that address are scored too.
				if n.Val() == 4 {
					for _, other := range e.Redis.SMembers(ctx, ipk).Val() {
						if other != qid {
							_, _ = e.Scorer.Add(ctx, k.Bot(other), botdefence.ManyPerIP)
						}
					}
				}
			}
			if sn != nil && sn.Val() > 50 {
				add(botdefence.ManyPerSubnet)
			}
		}
	}
	if in.Print != "" && len(in.Print) <= 128 {
		fk := k.k("q-print", ipPart(in.Print))
		pipe := e.Redis.Pipeline()
		pipe.SAdd(ctx, fk, IdentityHash(saleID, in.Sub))
		pipe.Expire(ctx, fk, 48*time.Hour)
		n := pipe.SCard(ctx, fk)
		if _, err := pipe.Exec(ctx); err == nil && n.Val() > 1 {
			add(botdefence.SharedPrint)
		}
	}
}

// pollDelay is the next poll in ms: base ± 3 s of jitter.
func (e *Engine) pollDelay(base int64) int64 {
	var b [2]byte
	_, _ = rand.Read(b[:])
	j := int64(b[0])<<8 | int64(b[1])
	return base - 3000 + j%6001
}

// Verifier checks the engine's own tokens.
func (e *Engine) Verifier() *Verifier { return NewVerifier(e.Signer.Key.Public().(ed25519.PublicKey)) }

// StatusOf answers a poll: the place's position and estimate, or its pass once admitted. A poll
// faster than every 2 s gets the last answer again (and counts as a bot signal).
func (e *Engine) StatusOf(ctx context.Context, saleID, queueToken, session, sub string) (*Status, error) {
	if e.Signer == nil {
		return nil, ErrBadToken
	}
	c, err := e.Verifier().Verify(queueToken, KindQueue, e.now(), false)
	if err != nil || c.Sale != saleID || c.Sid != SessionHash(session) || c.Sub != sub {
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
	if sale.Settings.BotDefence && e.Scorer != nil {
		_, _ = e.Scorer.Poll(ctx, k.Bot(c.QID), now.UnixMilli())
	}
	cacheKey := k.rl("st", c.QID)
	if last, err := e.Redis.Get(ctx, cacheKey).Result(); err == nil {
		var s Status
		if json.Unmarshal([]byte(last), &s) == nil && now.Sub(s.ServerTime) < 2*time.Second && now.Sub(s.ServerTime) >= 0 {
			s.ServerTime = now
			return &s, nil
		}
	}
	s, err := e.status(ctx, sale, k, c.QID, c.Sub, c.Sid, now)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(s)
	_ = e.Redis.Set(ctx, cacheKey, b, 2*time.Second).Err()
	return s, nil
}

// status works out a place's state at now.
func (e *Engine) status(ctx context.Context, sale *sales.Sale, k Keys, qid, sub, sid string, now time.Time) (*Status, error) {
	st := sale.Settings
	out := &Status{SaleID: sale.ID, EventID: sale.EventID, OpensAt: st.OpensAt, PrequeueOpensAt: st.PrequeueOpensAt(), ServerTime: now, Paused: st.Paused}
	statusHits.Inc()
	pipe := e.Redis.Pipeline()
	adm := pipe.HGet(ctx, k.Admitted(), qid)
	pre := pipe.ZScore(ctx, k.Pre(), qid)
	rank := pipe.ZRank(ctx, k.Line(), qid)
	slow := pipe.ZRank(ctx, k.Slow(), qid)
	lineLen := pipe.ZCard(ctx, k.Line())
	state := pipe.Get(ctx, k.State())
	pass := pipe.HMGet(ctx, k.Pass(qid), "state", "tok")
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	switch {
	case adm.Err() == nil:
		at, _ := adm.Int64()
		exp := time.UnixMilli(at).Add(PassLife)
		pv := pass.Val()
		if s := fmt.Sprint(pv[0]); s == "used" || s == "revoked" || !now.Before(exp) {
			out.State = PlaceEnded
			out.NextPollMs = 0
			return out, nil
		}
		tok := ""
		if pv[1] != nil {
			tok = fmt.Sprint(pv[1])
		}
		if tok == "" {
			tok = e.Signer.Sign(Claims{Kind: KindPass, Org: sale.OrganiserID, Evt: sale.EventID, Sale: sale.ID, Sub: sub, Sid: sid, QID: qid,
				Iat: now.Unix(), Exp: exp.Unix()})
			ok, err := e.Redis.HSetNX(ctx, k.Pass(qid), "tok", tok).Result()
			if err != nil {
				return nil, err
			}
			if ok {
				_ = e.Redis.HSet(ctx, k.Pass(qid), "state", "active", "sid", sid).Err()
				_ = e.Redis.ExpireAt(ctx, k.Pass(qid), exp.Add(24*time.Hour)).Err()
				passesC.Inc()
			} else {
				tok, _ = e.Redis.HGet(ctx, k.Pass(qid), "tok").Result()
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
		out.NextPollMs = e.pollDelay(10_000)
		if d := st.OpensAt.Sub(now).Milliseconds(); d > 0 && d < out.NextPollMs {
			out.NextPollMs = d + 500
		}
	case rank.Err() == nil || slow.Err() == nil:
		ahead := rank.Val()
		if rank.Err() != nil {
			ahead = lineLen.Val() + slow.Val()
		}
		out.State = PlaceWaiting
		out.Ahead = &ahead
		est := ahead * 60 / int64(max(1, st.RatePerMinute))
		out.EstimateSeconds = &est
		out.NextPollMs = e.pollDelay(10_000)
		if ahead < int64(st.RatePerMinute)/6 {
			out.NextPollMs = e.pollDelay(4_000) // near the front: look a little sooner
		}
	default:
		if state.Val() == StateShuffling {
			out.State = PlaceShuffling
			out.NextPollMs = 2000
			break
		}
		return nil, ErrBadToken // the place is gone (the store was reset): join again
	}
	return out, nil
}
