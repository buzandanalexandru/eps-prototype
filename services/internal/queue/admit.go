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

var tickSeconds = metrics.NewHistogram("eps_queue_admit_tick_seconds", "Time of one admitter tick of one sale.", []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1})

// admitScript admits up to the token bucket's worth of places from the front of the line within
// the shopping limit (D-2): suspects met at the front go to the back (or to the slow lane), the
// slow lane is served only when the line is empty, and a paused sale admits nobody. An admitted
// place holds a shopping place for the fetch window until its pass is collected (review spec M3).
// KEYS: state, line, slow, active, rate.
// ARGV: test now (0: TIME), rate per minute, shopping limit, paused, fetch window ms, bot defence
// on, fan key prefix, most per tick, suspect score, slow-lane score.
// Returns {admitted, active after, moved back, slow-laned}.
var admitScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'state') ~= 'open' then return {0, 0, 0, 0} end
local now = tonumber(ARGV[1])
if now == 0 then local t = redis.call('TIME'); now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000) end
redis.call('ZREMRANGEBYSCORE', KEYS[4], '-inf', now)
local active = redis.call('ZCARD', KEYS[4])
local rate = tonumber(ARGV[2])
local b = redis.call('HMGET', KEYS[1], 'last', 'tokens', 'tail')
local last = tonumber(b[1] or now)
local tokens = tonumber(b[2] or '0')
local tail = tonumber(b[3] or '0')
if ARGV[4] == '1' then
  redis.call('HSET', KEYS[1], 'last', now, 'tokens', 0)
  return {0, active, 0, 0}
end
local burst = math.max(1, rate / 60)
tokens = math.min(burst, tokens + math.max(0, now - last) * rate / 60000)
local room = tonumber(ARGV[3]) - active
local n = math.min(math.floor(tokens), room, tonumber(ARGV[8]))
local admitted, moved, slowed = 0, 0, 0
local fetch = tonumber(ARGV[5])
local admit = function(qid)
  redis.call('HSET', ARGV[7] .. qid, 'adm', now)
  redis.call('ZADD', KEYS[4], now + fetch, qid)
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
    if score < ` + strconv.Itoa(penaltyBase) + ` then redis.call('HSET', ARGV[7] .. qid, 'pos', score) end
    slowed = slowed + 1
  elseif bs >= tonumber(ARGV[9]) and score < ` + strconv.Itoa(penaltyBase) + ` then
    redis.call('ZADD', KEYS[2], tail + ` + strconv.Itoa(penaltyBase) + ` + score, qid)
    redis.call('HSET', ARGV[7] .. qid, 'pos', score)
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
redis.call('HSET', KEYS[1], 'last', now, 'tokens', tokens - admitted)
if admitted > 0 then redis.call('HINCRBY', KEYS[1], 'admc', admitted) end
redis.call('LPUSH', KEYS[5], now .. ':' .. admitted)
redis.call('LTRIM', KEYS[5], 0, 119)
return {admitted, active + admitted, moved, slowed}
`)

// TickResult is one admitter tick.
type TickResult struct {
	Admitted, Active, Moved, Slowed int64
}

// Admit runs one admitter tick for a sale.
func (e *Engine) Admit(ctx context.Context, sale *sales.Sale) (TickResult, error) {
	start := time.Now()
	defer func() { tickSeconds.Observe(time.Since(start).Seconds()) }()
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
	v, err := admitScript.Run(ctx, e.Redis, []string{k.State(), k.Line(), k.Slow(), k.Active(), k.Rate()},
		e.testNow(), st.RatePerMinute, st.ActiveLimit, paused, FetchWindow.Milliseconds(), bot, k.prefix("qf"), 5000,
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

// finishScript ends the draw (review D-c): everyone still in the pre-queue (arrived at or after
// opening, or during the draw) goes behind the drawn places in arrival order; the line opens.
// KEYS: pre, line, state, members. ARGV: count drawn. Returns the tail.
var finishScript = redis.NewScript(`
local n = tonumber(ARGV[1])
local late = redis.call('ZRANGE', KEYS[1], 0, -1)
for i, qid in ipairs(late) do
  redis.call('ZADD', KEYS[2], n + i, qid)
end
redis.call('DEL', KEYS[1], KEYS[4])
local tail = n + #late
redis.call('HSET', KEYS[3], 'tail', tail, 'state', 'open', 'drawn', n)
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

// renewScript extends a lock only for its holder.
var renewScript = redis.NewScript(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE', KEYS[1], ARGV[2]) end return 0`)

// Shuffle draws the pre-queue at opening (D-2, review D-c), resumable: the members who arrived
// before opening are snapshotted with a crypto-random seed first (the audit keeps that one seed);
// positions are then written in batches of 5,000, each batch recording how far the draw got, under
// a short lock renewed per batch. A draw cut short is continued by the next tick from the same
// snapshot and seed, so positions are never handed out twice. Joins during the draw wait in the
// pre-queue and go behind it in arrival order. With bot defence on, suspects then go to the back.
func (e *Engine) Shuffle(ctx context.Context, sale *sales.Sale) (int, error) {
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return 0, err
	}
	me := newID()
	ok, err := e.Redis.SetNX(ctx, k.lock("draw"), me, 10*time.Second).Result()
	if err != nil || !ok {
		return 0, err
	}
	defer func() { _ = unlockScript.Run(context.WithoutCancel(ctx), e.Redis, []string{k.lock("draw")}, me).Err() }()
	d, err := e.Redis.HMGet(ctx, k.State(), "state", "seed", "n", "drawn").Result()
	if err != nil {
		return 0, err
	}
	if fmt.Sprint(d[0]) == StateOpen {
		return 0, nil
	}
	var seed [32]byte
	n, drawn := 0, 0
	if d[1] == nil {
		// The snapshot: who arrived before opening, in arrival order, with the seed.
		if _, err := rand.Read(seed[:]); err != nil {
			return 0, err
		}
		members, err := e.Redis.ZRangeArgs(ctx, redis.ZRangeArgs{Key: k.Pre(), Start: "-inf", Stop: "(" + strconv.FormatInt(sale.Settings.OpensAt.UnixMilli(), 10), ByScore: true}).Result()
		if err != nil {
			return 0, err
		}
		pipe := e.Redis.TxPipeline()
		pipe.Del(ctx, k.Members())
		for i := 0; i < len(members); i += 5000 {
			chunk := make([]any, 0, 5000)
			for _, m := range members[i:min(len(members), i+5000)] {
				chunk = append(chunk, m)
			}
			pipe.RPush(ctx, k.Members(), chunk...)
		}
		pipe.HSet(ctx, k.State(), "state", StateShuffling, "seed", hex.EncodeToString(seed[:]), "n", len(members), "drawn", 0, "at", e.now().UnixMilli())
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, err
		}
		n = len(members)
	} else {
		b, err := hex.DecodeString(fmt.Sprint(d[1]))
		if err != nil || len(b) != 32 {
			return 0, errors.New("queue: bad draw seed")
		}
		copy(seed[:], b)
		n, _ = strconv.Atoi(fmt.Sprint(d[2]))
		drawn, _ = strconv.Atoi(fmt.Sprint(d[3]))
	}
	members, err := e.Redis.LRange(ctx, k.Members(), 0, -1).Result()
	if err != nil {
		return 0, err
	}
	if len(members) != n {
		return 0, fmt.Errorf("queue: draw snapshot has %d of %d members", len(members), n)
	}
	order := Permute(members, seed)
	for i := drawn; i < len(order); i += 5000 {
		end := min(len(order), i+5000)
		pipe := e.Redis.TxPipeline()
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
		pipe.HSet(ctx, k.State(), "drawn", end)
		if _, err := pipe.Exec(ctx); err != nil {
			return 0, err
		}
		if n, err := renewScript.Run(ctx, e.Redis, []string{k.lock("draw")}, me, 10000).Int(); err != nil || n == 0 {
			return 0, errors.New("queue: lost the draw lock")
		}
	}
	tail, err := finishScript.Run(ctx, e.Redis, []string{k.Pre(), k.Line(), k.State(), k.Members()}, n).Int()
	if err != nil {
		return 0, err
	}
	shuffles.Inc()
	if sale.Settings.BotDefence && e.Scorer != nil {
		if _, err := e.Penalise(ctx, sale); err != nil {
			return tail, err
		}
	}
	return tail, nil
}

var unlockScript = redis.NewScript(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`)

// Penalise moves suspect places behind every other place (score 70 or more) or to the slow lane
// (90 or more), after the draw only (D-6). Their page keeps showing a normal position.
func (e *Engine) Penalise(ctx context.Context, sale *sales.Sale) (int, error) {
	k, err := KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return 0, err
	}
	tail, _ := e.Redis.HGet(ctx, k.State(), "tail").Int64()
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
			q := fmt.Sprint(z.Member)
			mv.HSet(ctx, k.Fan(q), "pos", int64(z.Score))
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
		}
		if len(zs) < 5000 {
			break
		}
	}
	return moved, nil
}

// Ended marks a place's passes used (its order paid) or revoked (cancelled), freeing a shopping
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
	pipe.HSet(ctx, k.Fan(qid), "pst", how)
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
	st := pipe.HMGet(ctx, k.State(), "state", "admc", "seed")
	pre := pipe.ZCard(ctx, k.Pre())
	line := pipe.ZCard(ctx, k.Line())
	slow := pipe.ZCard(ctx, k.Slow())
	act := pipe.ZCount(ctx, k.Active(), strconv.FormatInt(now.UnixMilli(), 10), "+inf")
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return Counts{}, err
	}
	v := st.Val()
	str := func(x any) string {
		if x == nil {
			return ""
		}
		return fmt.Sprint(x)
	}
	admc, _ := strconv.ParseInt(str(v[1]), 10, 64)
	return Counts{State: str(v[0]), Prequeue: pre.Val(), Line: line.Val(), Slow: slow.Val(), Admitted: admc, Active: act.Val(), Seed: str(v[2])}, nil
}

// Admitter runs the draw and the admission ticks of every sale with a waiting room, each sale under
// a lease (one admitter per sale across gateways, renewed every tick), a bounded number of sales at
// once (review sec M5).
type Admitter struct {
	Engine *Engine
	Log    *slog.Logger
	// Parallel is how many sales tick at once (default 8).
	Parallel int
	me       string
	once     sync.Once
	gauges   sync.Map
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
	n, _ := renewScript.Run(ctx, a.Engine.Redis, []string{key}, a.me, 5000).Int()
	return n == 1
}

// RunOnce ticks every sale once.
func (a *Admitter) RunOnce(ctx context.Context) {
	e := a.Engine
	list, err := e.Redis.SMembers(ctx, salesIndex()).Result()
	if err != nil {
		return
	}
	par := a.Parallel
	if par <= 0 {
		par = 8
	}
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for _, m := range list {
		wg.Add(1)
		sem <- struct{}{}
		go func(m string) {
			defer wg.Done()
			defer func() { <-sem }()
			a.tick(ctx, m)
		}(m)
	}
	wg.Wait()
}

func (a *Admitter) tick(ctx context.Context, m string) {
	e := a.Engine
	p := strings.Split(m, "/")
	if len(p) != 3 {
		return
	}
	k, err := KeysOf(p[0], p[1], p[2])
	if err != nil {
		return
	}
	sale, err := e.Sales.Lookup(ctx, p[2])
	now := e.now()
	if err != nil || now.After(sale.Settings.OpensAt.Add(72*time.Hour)) {
		if err == nil || strings.Contains(err.Error(), "not found") {
			// Long over or deleted: stop ticking it and drop its gauges.
			_ = e.Redis.SRem(ctx, salesIndex(), m).Err()
			a.dropGauges(p[2])
		}
		return
	}
	a.gauge(p[0], p[1], p[2])
	if !a.lease(ctx, k) || sale.Settings.PhaseAt(now) != sales.Open {
		return
	}
	if st, _ := e.Redis.HGet(ctx, k.State(), "state").Result(); st != StateOpen {
		if _, err := e.Shuffle(ctx, sale); err != nil && a.Log != nil {
			a.Log.Warn("shuffle failed", "sale", sale.ID, "error", err.Error())
		}
		return
	}
	if _, err := e.Admit(ctx, sale); err != nil && a.Log != nil && ctx.Err() == nil {
		a.Log.Warn("admit failed", "sale", sale.ID, "error", err.Error())
	}
}

var gaugeNames = []string{"eps_queue_prequeue_size", "eps_queue_length", "eps_queue_admitted", "eps_queue_active_sessions"}

// gauge registers a sale's queue gauges once (read at scrape time). With several gateways each
// reports the same Redis figures: a dashboard takes the max per sale, never the sum.
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
	metrics.NewGauge(gaugeNames[0]+l, read(func(c Counts) int64 { return c.Prequeue }))
	metrics.NewGauge(gaugeNames[1]+l, read(func(c Counts) int64 { return c.Line + c.Slow }))
	metrics.NewGauge(gaugeNames[2]+l, read(func(c Counts) int64 { return c.Admitted }))
	metrics.NewGauge(gaugeNames[3]+l, read(func(c Counts) int64 { return c.Active }))
}

func (a *Admitter) dropGauges(sale string) {
	a.gauges.Delete(sale)
	for _, n := range gaugeNames {
		metrics.RemoveGauge(n + `{sale="` + sale + `"}`)
	}
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
