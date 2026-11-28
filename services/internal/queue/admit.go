package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	mrand "math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// Tick is how often the admitter runs (D-2: every 500 ms).
const Tick = 500 * time.Millisecond

// penaltyBase puts a suspect behind every place that will ever be handed out in order.
const penaltyBase = 1_000_000

// admitScript admits up to the token bucket's worth of places from the front of the line within
// the shopping limit (D-2): suspects met at the front go to the back (or to the slow lane), the
// slow lane is served only when the line is empty, and a paused sale admits nobody.
// KEYS: state, line, slow, admitted, active, bucket, tail.
// ARGV: now ms, rate per minute, shopping limit, paused, pass life ms, bot defence on, score key
// prefix, most per tick, suspect score, slow-lane score.
// Returns {admitted, active after, moved back, slow-laned}.
var admitScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= 'open' then return {0, 0, 0, 0} end
local now = tonumber(ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[5], '-inf', now)
local active = redis.call('ZCARD', KEYS[5])
local rate = tonumber(ARGV[2])
local last = tonumber(redis.call('HGET', KEYS[6], 'last') or ARGV[1])
local tokens = tonumber(redis.call('HGET', KEYS[6], 'tokens') or '0')
if ARGV[4] == '1' then
  redis.call('HSET', KEYS[6], 'last', now, 'tokens', 0)
  return {0, active, 0, 0}
end
local burst = math.max(1, rate / 60)
tokens = math.min(burst, tokens + math.max(0, now - last) * rate / 60000)
local room = tonumber(ARGV[3]) - active
local n = math.min(math.floor(tokens), room, tonumber(ARGV[8]))
local admitted, moved, slowed = 0, 0, 0
local life = tonumber(ARGV[5])
local admit = function(qid)
  redis.call('HSET', KEYS[4], qid, now)
  redis.call('ZADD', KEYS[5], now + life, qid)
  admitted = admitted + 1
end
local looked = 0
while admitted < n and looked < n * 4 + 20 do
  local x = redis.call('ZPOPMIN', KEYS[2], 1)
  if #x == 0 then break end
  looked = looked + 1
  local qid, score = x[1], tonumber(x[2])
  local bs = 0
  if ARGV[6] == '1' then bs = tonumber(redis.call('HGET', ARGV[7] .. qid, 'score') or '0') end
  if bs >= tonumber(ARGV[10]) then
    redis.call('ZADD', KEYS[3], score, qid)
    slowed = slowed + 1
  elseif bs >= tonumber(ARGV[9]) and score < ` + strconv.Itoa(penaltyBase) + ` then
    local tail = tonumber(redis.call('GET', KEYS[7]) or '0')
    redis.call('ZADD', KEYS[2], tail + ` + strconv.Itoa(penaltyBase) + ` + score, qid)
    moved = moved + 1
  else
    admit(qid)
  end
end
if admitted < n and redis.call('ZCARD', KEYS[2]) == 0 then
  while admitted < n do
    local x = redis.call('ZPOPMIN', KEYS[3], 1)
    if #x == 0 then break end
    admit(x[1])
  end
end
redis.call('HSET', KEYS[6], 'last', now, 'tokens', tokens - admitted)
return {admitted, active + admitted, moved, slowed}
`)

// TickResult is one admitter tick.
type TickResult struct {
	Admitted, Active, Moved, Slowed int64
}

// Admit runs one admitter tick for a sale.
func (e *Engine) Admit(ctx context.Context, sale *sales.Sale) (TickResult, error) {
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return TickResult{}, err
	}
	st := sale.Settings
	paused, bot := "0", "0"
	if st.Paused {
		paused = "1"
	}
	if st.BotDefence {
		bot = "1"
	}
	v, err := admitScript.Run(ctx, e.Redis, []string{k.State(), k.Line(), k.Slow(), k.Admitted(), k.Active(), k.Bucket(), k.Tail()},
		e.now().UnixMilli(), st.RatePerMinute, st.ActiveLimit, paused, PassLife.Milliseconds(), bot, k.prefix("q-bot"), 5000,
		botdefence.SuspectAt, botdefence.SlowLaneAt).Int64Slice()
	if err != nil {
		return TickResult{}, err
	}
	r := TickResult{v[0], v[1], v[2], v[3]}
	admittedC.Add(r.Admitted)
	penalised.Add(r.Moved)
	slowLaned.Add(r.Slowed)
	return r, nil
}

// finishScript ends the draw: places that joined during it go behind, in arrival order; the line
// opens. KEYS: pre, line, tail, state. ARGV: count drawn. Returns the tail.
var finishScript = redis.NewScript(`
local n = tonumber(ARGV[1])
local late = redis.call('ZRANGE', KEYS[1], 0, -1)
for i, qid in ipairs(late) do
  redis.call('ZADD', KEYS[2], n + i, qid)
end
redis.call('DEL', KEYS[1])
local tail = n + #late
redis.call('SET', KEYS[3], tail)
redis.call('SET', KEYS[4], 'open')
return tail
`)

// Permute is the draw (D-2): a Fisher-Yates shuffle driven by a ChaCha8 stream from a 32-byte seed,
// so the seed alone reproduces the order (the audit).
func Permute(members []string, seed [32]byte) []string {
	out := make([]string, len(members))
	copy(out, members)
	r := mrand.New(mrand.NewChaCha8(seed))
	for i := len(out) - 1; i > 0; i-- {
		j := r.IntN(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Shuffle draws the pre-queue at opening (D-2): one core at a time (a lock), a crypto-random seed
// stored for the audit, positions 1..N written in batches of 5,000, then everyone who joined during
// the draw behind in arrival order. With bot defence on, suspects are then moved to the back.
func (e *Engine) Shuffle(ctx context.Context, sale *sales.Sale) (int, error) {
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return 0, err
	}
	ok, err := e.Redis.SetNX(ctx, k.lock("shuffle"), "1", time.Minute).Result()
	if err != nil || !ok {
		return 0, err
	}
	defer e.Redis.Del(context.WithoutCancel(ctx), k.lock("shuffle"))
	if st, _ := e.Redis.Get(ctx, k.State()).Result(); st == StateOpen {
		return 0, nil
	}
	if err := e.Redis.Set(ctx, k.State(), StateShuffling, 0).Err(); err != nil {
		return 0, err
	}
	members, err := e.Redis.ZRange(ctx, k.Pre(), 0, -1).Result()
	if err != nil {
		return 0, err
	}
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return 0, err
	}
	order := Permute(members, seed)
	for i := 0; i < len(order); i += 5000 {
		end := min(len(order), i+5000)
		pipe := e.Redis.Pipeline()
		zs := make([]redis.Z, 0, end-i)
		rm := make([]any, 0, end-i)
		audit := make([]any, 0, end-i)
		for j := i; j < end; j++ {
			zs = append(zs, redis.Z{Score: float64(j + 1), Member: order[j]})
			rm = append(rm, order[j])
			b, _ := json.Marshal(map[string]any{"t": "pos", "qid": order[j], "pos": j + 1})
			audit = append(audit, b)
		}
		pipe.ZAdd(ctx, k.Line(), zs...)
		pipe.ZRem(ctx, k.Pre(), rm...)
		pipe.RPush(ctx, k.Audit(), audit...)
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, err
		}
	}
	tail, err := finishScript.Run(ctx, e.Redis, []string{k.Pre(), k.Line(), k.Tail(), k.State()}, len(order)).Int()
	if err != nil {
		return 0, err
	}
	_ = e.Redis.HSet(ctx, k.Shuffle(), "seed", hex.EncodeToString(seed[:]), "n", len(order), "at", e.now().UnixMilli()).Err()
	shuffles.Inc()
	if sale.Settings.BotDefence && e.Scorer != nil {
		if _, err := e.Penalise(ctx, sale); err != nil {
			return tail, err
		}
	}
	return tail, nil
}

// Penalise moves suspect places behind every other place (score 70 or more) or to the slow lane
// (90 or more), after the draw only (D-6). Their page keeps showing a normal position.
func (e *Engine) Penalise(ctx context.Context, sale *sales.Sale) (int, error) {
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return 0, err
	}
	tail, _ := e.Redis.Get(ctx, k.Tail()).Int64()
	moved := 0
	for start := int64(0); ; start += 5000 {
		zs, err := e.Redis.ZRangeWithScores(ctx, k.Line(), start, start+4999).Result()
		if err != nil {
			return moved, err
		}
		if len(zs) == 0 {
			break
		}
		pipe := e.Redis.Pipeline()
		cmds := make([]*redis.StringCmd, len(zs))
		for i, z := range zs {
			cmds[i] = pipe.HGet(ctx, k.Bot(fmt.Sprint(z.Member)), "score")
		}
		_, _ = pipe.Exec(ctx)
		mv := e.Redis.Pipeline()
		n := 0
		for i, z := range zs {
			s, _ := cmds[i].Int()
			if z.Score >= penaltyBase || s < botdefence.SuspectAt {
				continue
			}
			if s >= botdefence.SlowLaneAt {
				mv.ZRem(ctx, k.Line(), z.Member)
				mv.ZAdd(ctx, k.Slow(), z)
				slowLaned.Inc()
			} else {
				mv.ZAdd(ctx, k.Line(), redis.Z{Score: float64(tail+penaltyBase) + z.Score, Member: z.Member})
				penalised.Inc()
			}
			n++
		}
		if n > 0 {
			if _, err := mv.Exec(ctx); err != nil {
				return moved, err
			}
			moved += n
			start -= int64(n) // members moved out of this window shift the ranks
			if start < -5000 {
				start = -5000
			}
		}
		if len(zs) < 5000 {
			break
		}
	}
	return moved, nil
}

// Ended marks a place's pass used (its order paid) or revoked (cancelled), freeing a shopping
// place at once.
func (e *Engine) Ended(ctx context.Context, org, event, saleID, qid, how string) error {
	k, err := KeysOf(org, event, saleID)
	if err != nil {
		return err
	}
	if how != "used" {
		how = "revoked"
	}
	pipe := e.Redis.TxPipeline()
	pipe.HSet(ctx, k.Pass(qid), "state", how)
	pipe.Expire(ctx, k.Pass(qid), 24*time.Hour)
	pipe.ZRem(ctx, k.Active(), qid)
	_, err = pipe.Exec(ctx)
	return err
}

// Counts are a sale's queue figures (Studio and the metrics).
type Counts struct {
	State    string `json:"state"`
	Prequeue int64  `json:"prequeue"`
	Line     int64  `json:"line"`
	Slow     int64  `json:"slow"`
	Admitted int64  `json:"admitted"`
	Active   int64  `json:"active"`
	Seed     string `json:"seed,omitempty"`
}

// CountsOf reads a sale's queue figures.
func CountsOf(ctx context.Context, rdb *redis.Client, org, event, saleID string, now time.Time) (Counts, error) {
	k, err := KeysOf(org, event, saleID)
	if err != nil {
		return Counts{}, err
	}
	pipe := rdb.Pipeline()
	st := pipe.Get(ctx, k.State())
	pre := pipe.ZCard(ctx, k.Pre())
	line := pipe.ZCard(ctx, k.Line())
	slow := pipe.ZCard(ctx, k.Slow())
	adm := pipe.HLen(ctx, k.Admitted())
	act := pipe.ZCount(ctx, k.Active(), strconv.FormatInt(now.UnixMilli(), 10), "+inf")
	seed := pipe.HGet(ctx, k.Shuffle(), "seed")
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return Counts{}, err
	}
	return Counts{State: st.Val(), Prequeue: pre.Val(), Line: line.Val(), Slow: slow.Val(), Admitted: adm.Val(), Active: act.Val(), Seed: seed.Val()}, nil
}

// Admitter runs the draw and the admission ticks of every sale with a waiting room, each sale under
// a lease (one admitter per sale across gateways, renewed every tick).
type Admitter struct {
	Engine *Engine
	Log    *slog.Logger
	me     string
	once   sync.Once
	gauges sync.Map
}

func (a *Admitter) lease(ctx context.Context, k Keys) bool {
	a.once.Do(func() { a.me = newID() })
	key := k.lock("admitter")
	ok, err := a.Engine.Redis.SetNX(ctx, key, a.me, 5*time.Second).Result()
	if err != nil {
		return false
	}
	if ok {
		return true
	}
	if cur, _ := a.Engine.Redis.Get(ctx, key).Result(); cur == a.me {
		_ = a.Engine.Redis.Expire(ctx, key, 5*time.Second).Err()
		return true
	}
	return false
}

// RunOnce ticks every sale once.
func (a *Admitter) RunOnce(ctx context.Context) {
	e := a.Engine
	list, err := e.Redis.SMembers(ctx, salesIndex()).Result()
	if err != nil {
		return
	}
	for _, m := range list {
		p := strings.Split(m, "/")
		if len(p) != 3 {
			continue
		}
		k, err := KeysOf(p[0], p[1], p[2])
		if err != nil {
			continue
		}
		a.gauge(p[0], p[1], p[2])
		if !a.lease(ctx, k) {
			continue
		}
		sale, err := e.Sales.Lookup(ctx, p[2])
		if err != nil {
			continue
		}
		now := e.now()
		if now.After(sale.Settings.OpensAt.Add(72 * time.Hour)) {
			_ = e.Redis.SRem(ctx, salesIndex(), m).Err() // long over: stop ticking it
			continue
		}
		if sale.Settings.PhaseAt(now) != sales.Open {
			continue
		}
		if st, _ := e.Redis.Get(ctx, k.State()).Result(); st != StateOpen {
			if _, err := e.Shuffle(ctx, sale); err != nil && a.Log != nil {
				a.Log.Warn("shuffle failed", "sale", sale.ID, "error", err.Error())
			}
			continue
		}
		if _, err := e.Admit(ctx, sale); err != nil && a.Log != nil && ctx.Err() == nil {
			a.Log.Warn("admit failed", "sale", sale.ID, "error", err.Error())
		}
	}
}

// gauge registers a sale's queue gauges once (read at scrape time).
func (a *Admitter) gauge(org, event, sale string) {
	if _, loaded := a.gauges.LoadOrStore(sale, true); loaded {
		return
	}
	read := func(f func(Counts) int64) func() float64 {
		return func() float64 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c, err := CountsOf(ctx, a.Engine.Redis, org, event, sale, time.Now())
			if err != nil {
				return 0
			}
			return float64(f(c))
		}
	}
	l := `{sale="` + sale + `"}`
	metrics.NewGauge("eps_queue_prequeue_size"+l, read(func(c Counts) int64 { return c.Prequeue }))
	metrics.NewGauge("eps_queue_length"+l, read(func(c Counts) int64 { return c.Line + c.Slow }))
	metrics.NewGauge("eps_queue_admitted"+l, read(func(c Counts) int64 { return c.Admitted }))
	metrics.NewGauge("eps_queue_active_sessions"+l, read(func(c Counts) int64 { return c.Active }))
}

// Run ticks until ctx ends.
func (a *Admitter) Run(ctx context.Context) {
	t := time.NewTicker(Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.RunOnce(ctx)
		}
	}
}
