package payments

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Breaker states.
const (
	Closed   = "closed"
	Open     = "open"
	HalfOpen = "half_open"
)

// Breaker rules (D-4, review H2/L4): it opens when over the last 20 s at least 20 calls were made
// and half or more failed, or after 5 failures in a row; after 15 s it lets up to 5 probe calls
// through (a probe slot not used within 10 s is given out again); 4 successful probes close it
// and a second failed probe reopens it for twice as long (at most 120 s).
const (
	WindowSeconds   = 20
	MinCalls        = 20
	FailureShareBps = 5000
	MaxConsecutive  = 5
	OpenFor         = 15 * time.Second
	MaxOpenFor      = 120 * time.Second
	Probes          = 5
	ProbeSuccesses  = 4
	ProbeFailures   = 2
	ProbeSlotIdle   = 10 * time.Second
)

// Breaker is one circuit breaker per provider and merchant (organiser), shared by every core
// through Redis. Every transition runs in one Lua script (review H2), so cores never interleave a
// read and a write; Read has no side effects. Declines are not failures (they are the card's
// answer, not the provider's health). If Redis cannot be reached, calls are allowed.
type Breaker struct {
	Redis *redis.Client
	Now   func() time.Time
	// OnChange is told about every state change (the payment.provider_state event).
	OnChange func(ctx context.Context, org, provider, from, to string)
}

// State is a breaker's shared state.
type State struct {
	Provider    string    `json:"provider"`
	State       string    `json:"state"`
	OpenedAt    time.Time `json:"openedAt,omitempty"`
	OpenFor     float64   `json:"openForSeconds,omitempty"`
	Calls       int64     `json:"calls"`
	Failures    int64     `json:"failures"`
	Consecutive int64     `json:"consecutiveFailures"`
}

var breakerChanges = map[string]*metrics.Counter{}

var changesMu sync.Mutex

func countChange(provider, to string) {
	k := provider + "/" + to
	changesMu.Lock()
	defer changesMu.Unlock()
	c, ok := breakerChanges[k]
	if !ok {
		c = metrics.NewCounter("eps_breaker_transitions_total", "Circuit breaker state changes.", "provider", provider, "to", to)
		breakerChanges[k] = c
	}
	c.Inc()
}

func (b *Breaker) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Scope is the breaker's (and the provider merchant's) scope: the organiser, or "<organiser>/<event>"
// for a rehearsal (step 6, design D-1), so a rehearsed outage never opens the organiser's breaker.
func Scope(org, rehearsalEvent string) string {
	if rehearsalEvent == "" {
		return org
	}
	return org + "/" + rehearsalEvent
}

// MerchantOf is the merchant account a scope pays under at the providers: the organiser id, or
// "rh-<event>" for a rehearsal (at most 64 characters).
func MerchantOf(scope string) string {
	if _, ev, ok := strings.Cut(scope, "/"); ok {
		return "rh-" + ev
	}
	return scope
}

func splitScope(scope string) (org, event string) {
	org, event, _ = strings.Cut(scope, "/")
	return org, event
}

func (b *Breaker) stateKey(scope, provider string) string {
	org, ev := splitScope(scope)
	k, _ := redisx.Key(org, ev, "pay-breaker", provider)
	return k
}

func (b *Breaker) bucketKeys(scope, provider string) []string {
	now := b.now().Unix()
	org, ev := splitScope(scope)
	out := make([]string, 0, WindowSeconds)
	for s := now - WindowSeconds + 1; s <= now; s++ {
		k, _ := redisx.Key(org, ev, "pay-win", provider, strconv.FormatInt(s, 10))
		out = append(out, k)
	}
	return out
}

// KEYS: state. ARGV: now ms, open-for ms (default), probes, probe slot idle ms.
// Returns 0 refused, 1 allowed, 2 allowed after moving open → half_open.
var allowScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'state') or 'closed'
if st == 'closed' then return 1 end
local now = tonumber(ARGV[1])
if st == 'open' then
  local oa = tonumber(redis.call('HGET', KEYS[1], 'opened_at') or '0')
  local of = tonumber(redis.call('HGET', KEYS[1], 'open_for') or ARGV[2])
  if now < oa + of then return 0 end
  redis.call('HSET', KEYS[1], 'state', 'half_open', 'probes', 1, 'probe_ok', 0, 'probe_fail', 0, 'probe_at', now)
  return 2
end
local p = tonumber(redis.call('HGET', KEYS[1], 'probes') or '0')
local pa = tonumber(redis.call('HGET', KEYS[1], 'probe_at') or '0')
if p < tonumber(ARGV[3]) then
  redis.call('HSET', KEYS[1], 'probes', p + 1, 'probe_at', now)
  return 1
end
if now - pa > tonumber(ARGV[4]) then
  -- The probes given out were never used (no call followed): give a slot out again.
  redis.call('HSET', KEYS[1], 'probe_at', now)
  return 1
end
return 0
`)

// KEYS: state, then the window's buckets, oldest to current.
// ARGV: now ms, ok (1|0), open-for ms, max open-for ms, min calls, failure share bps, max
// consecutive, probe successes, probe failures. Returns {from, to}.
var recordScript = redis.NewScript(`
local cur = KEYS[#KEYS]
local ok = ARGV[2] == '1'
redis.call('HINCRBY', cur, ok and 'ok' or 'err', 1)
redis.call('EXPIRE', cur, 60)
redis.call('EXPIRE', KEYS[1], 86400)
local st = redis.call('HGET', KEYS[1], 'state') or 'closed'
local consec
if ok then redis.call('HSET', KEYS[1], 'consecutive', 0); consec = 0 else consec = redis.call('HINCRBY', KEYS[1], 'consecutive', 1) end
local now = ARGV[1]
if st == 'closed' then
  if ok then return {st, st} end
  local calls, fails = 0, 0
  for i = 2, #KEYS do
    local o = tonumber(redis.call('HGET', KEYS[i], 'ok') or '0')
    local e = tonumber(redis.call('HGET', KEYS[i], 'err') or '0')
    calls = calls + o + e
    fails = fails + e
  end
  if consec >= tonumber(ARGV[7]) or (calls >= tonumber(ARGV[5]) and fails * 10000 >= calls * tonumber(ARGV[6])) then
    redis.call('HSET', KEYS[1], 'state', 'open', 'opened_at', now, 'open_for', ARGV[3])
    return {'closed', 'open'}
  end
  return {st, st}
end
if st == 'half_open' then
  if ok then
    local n = redis.call('HINCRBY', KEYS[1], 'probe_ok', 1)
    if n >= tonumber(ARGV[8]) then
      redis.call('HSET', KEYS[1], 'state', 'closed', 'consecutive', 0, 'open_for', ARGV[3])
      for i = 2, #KEYS do redis.call('DEL', KEYS[i]) end
      return {'half_open', 'closed'}
    end
  else
    local f = redis.call('HINCRBY', KEYS[1], 'probe_fail', 1)
    if f >= tonumber(ARGV[9]) then
      local of = tonumber(redis.call('HGET', KEYS[1], 'open_for') or ARGV[3]) * 2
      if of < 2 * tonumber(ARGV[3]) then of = 2 * tonumber(ARGV[3]) end
      if of > tonumber(ARGV[4]) then of = tonumber(ARGV[4]) end
      redis.call('HSET', KEYS[1], 'state', 'open', 'opened_at', now, 'open_for', of)
      return {'half_open', 'open'}
    end
  end
end
return {st, st}
`)

// Read returns the breaker's state without changing it (an open breaker whose time is over reads
// as half-open).
func (b *Breaker) Read(ctx context.Context, org, provider string) State {
	st := State{Provider: provider, State: Closed}
	if b.Redis == nil {
		return st
	}
	m, err := b.Redis.HGetAll(ctx, b.stateKey(org, provider)).Result()
	if err != nil {
		return st
	}
	if s := m["state"]; s != "" {
		st.State = s
	}
	if ms, err := strconv.ParseInt(m["opened_at"], 10, 64); err == nil {
		st.OpenedAt = time.UnixMilli(ms)
	}
	openFor := OpenFor
	if ms, err := strconv.ParseInt(m["open_for"], 10, 64); err == nil && ms > 0 {
		openFor = time.Duration(ms) * time.Millisecond
	}
	st.OpenFor = openFor.Seconds()
	st.Consecutive, _ = strconv.ParseInt(m["consecutive"], 10, 64)
	if st.State == Open && !b.now().Before(st.OpenedAt.Add(openFor)) {
		st.State = HalfOpen
	}
	pipe := b.Redis.Pipeline()
	var cmds []*redis.MapStringStringCmd
	for _, k := range b.bucketKeys(org, provider) {
		cmds = append(cmds, pipe.HGetAll(ctx, k))
	}
	_, _ = pipe.Exec(ctx)
	for _, c := range cmds {
		v := c.Val()
		ok, _ := strconv.ParseInt(v["ok"], 10, 64)
		er, _ := strconv.ParseInt(v["err"], 10, 64)
		st.Calls += ok + er
		st.Failures += er
	}
	return st
}

// Allow reports whether a call may go to the provider now; it reads only the state hash (review
// L2). Call it right before the call it guards (review H2).
func (b *Breaker) Allow(ctx context.Context, org, provider string) bool {
	if b.Redis == nil {
		return true
	}
	n, err := allowScript.Run(ctx, b.Redis, []string{b.stateKey(org, provider)},
		b.now().UnixMilli(), OpenFor.Milliseconds(), Probes, ProbeSlotIdle.Milliseconds()).Int()
	if err != nil {
		return true // Redis down: each core decides alone
	}
	if n == 2 {
		b.changed(ctx, org, provider, Open, HalfOpen)
	}
	return n > 0
}

// Record counts a call's health: ok false for a pre-acceptance failure (refused connection or a
// 503), true for an answer. Unknown outcomes are not counted.
func (b *Breaker) Record(ctx context.Context, org, provider string, ok bool) {
	if b.Redis == nil {
		return
	}
	okv := "0"
	if ok {
		okv = "1"
	}
	keys := append([]string{b.stateKey(org, provider)}, b.bucketKeys(org, provider)...)
	res, err := recordScript.Run(ctx, b.Redis, keys, b.now().UnixMilli(), okv, OpenFor.Milliseconds(), MaxOpenFor.Milliseconds(),
		MinCalls, FailureShareBps, MaxConsecutive, ProbeSuccesses, ProbeFailures).StringSlice()
	if err != nil || len(res) != 2 || res[0] == res[1] {
		return
	}
	b.changed(ctx, org, provider, res[0], res[1])
}

func (b *Breaker) changed(ctx context.Context, scope, provider, from, to string) {
	countChange(provider, to)
	if b.OnChange != nil {
		org, _ := splitScope(scope)
		b.OnChange(ctx, org, provider, from, to)
	}
}

// Reset closes a breaker (tests and operators).
func (b *Breaker) Reset(ctx context.Context, org, provider string) {
	if b.Redis != nil {
		_ = b.Redis.Del(ctx, b.stateKey(org, provider)).Err()
	}
}
