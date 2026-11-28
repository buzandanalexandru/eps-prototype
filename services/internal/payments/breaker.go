package payments

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Breaker states.
const (
	Closed   = "closed"
	Open     = "open"
	HalfOpen = "half_open"
)

// Breaker rules (D-4).
const (
	WindowSeconds   = 20
	MinCalls        = 20
	FailureShareBps = 5000
	MaxConsecutive  = 5
	OpenFor         = 15 * time.Second
	MaxOpenFor      = 120 * time.Second
	Probes          = 5
	ProbeSuccesses  = 4
)

// Breaker is one circuit breaker per provider and merchant (organiser), shared by every core
// through Redis: per-second buckets of successes and failures over the last 20 seconds and the
// state hash. Declines are not failures (they are the card's answer, not the provider's health).
// If Redis cannot be reached, calls are allowed (each core then decides alone).
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

func (b *Breaker) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b *Breaker) stateKey(org, provider string) string {
	k, _ := redisx.Key(org, "", "pay-breaker", provider)
	return k
}

func (b *Breaker) bucketKey(org, provider string, sec int64) string {
	k, _ := redisx.Key(org, "", "pay-win", provider, strconv.FormatInt(sec, 10))
	return k
}

// window sums the last WindowSeconds buckets.
func (b *Breaker) window(ctx context.Context, org, provider string) (calls, fails int64) {
	now := b.now().Unix()
	pipe := b.Redis.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, WindowSeconds)
	for s := now - WindowSeconds + 1; s <= now; s++ {
		cmds = append(cmds, pipe.HGetAll(ctx, b.bucketKey(org, provider, s)))
	}
	_, _ = pipe.Exec(ctx)
	for _, c := range cmds {
		m := c.Val()
		ok, _ := strconv.ParseInt(m["ok"], 10, 64)
		er, _ := strconv.ParseInt(m["err"], 10, 64)
		calls += ok + er
		fails += er
	}
	return calls, fails
}

// Read returns the breaker's state (moving open to half-open once its time is over).
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
		b.set(ctx, org, provider, Open, HalfOpen, map[string]any{"state": HalfOpen, "probes": 0, "probe_ok": 0})
		st.State = HalfOpen
	}
	st.Calls, st.Failures = b.window(ctx, org, provider)
	return st
}

func (b *Breaker) set(ctx context.Context, org, provider, from, to string, fields map[string]any) {
	_ = b.Redis.HSet(ctx, b.stateKey(org, provider), fields).Err()
	_ = b.Redis.Expire(ctx, b.stateKey(org, provider), 24*time.Hour).Err()
	if from != to && b.OnChange != nil {
		b.OnChange(ctx, org, provider, from, to)
	}
}

// Allow reports whether a new attempt may go to the provider now (a half-open breaker lets up to
// five probe attempts through).
func (b *Breaker) Allow(ctx context.Context, org, provider string) bool {
	if b.Redis == nil {
		return true
	}
	st := b.Read(ctx, org, provider)
	switch st.State {
	case Open:
		return false
	case HalfOpen:
		n, err := b.Redis.HIncrBy(ctx, b.stateKey(org, provider), "probes", 1).Result()
		return err != nil || n <= Probes
	}
	return true
}

// Record counts a call's health: ok false for a 5xx, a connection error or a timeout.
func (b *Breaker) Record(ctx context.Context, org, provider string, ok bool) {
	if b.Redis == nil {
		return
	}
	bk := b.bucketKey(org, provider, b.now().Unix())
	field := "ok"
	if !ok {
		field = "err"
	}
	pipe := b.Redis.Pipeline()
	pipe.HIncrBy(ctx, bk, field, 1)
	pipe.Expire(ctx, bk, 60*time.Second)
	if ok {
		pipe.HSet(ctx, b.stateKey(org, provider), "consecutive", 0)
	} else {
		pipe.HIncrBy(ctx, b.stateKey(org, provider), "consecutive", 1)
	}
	_, _ = pipe.Exec(ctx)

	st := b.Read(ctx, org, provider)
	nowMs := b.now().UnixMilli()
	switch st.State {
	case Closed:
		tripped := st.Consecutive >= MaxConsecutive || (st.Calls >= MinCalls && st.Failures*10000 >= st.Calls*FailureShareBps)
		if !ok && tripped {
			b.set(ctx, org, provider, Closed, Open, map[string]any{"state": Open, "opened_at": nowMs, "open_for": OpenFor.Milliseconds()})
		}
	case HalfOpen:
		if !ok {
			// A failed probe reopens it, for twice as long (at most 120 s).
			next := time.Duration(st.OpenFor*float64(time.Second)) * 2
			if next < 2*OpenFor {
				next = 2 * OpenFor
			}
			if next > MaxOpenFor {
				next = MaxOpenFor
			}
			b.set(ctx, org, provider, HalfOpen, Open, map[string]any{"state": Open, "opened_at": nowMs, "open_for": next.Milliseconds()})
			return
		}
		n, _ := b.Redis.HIncrBy(ctx, b.stateKey(org, provider), "probe_ok", 1).Result()
		if n >= ProbeSuccesses {
			b.set(ctx, org, provider, HalfOpen, Closed, map[string]any{"state": Closed, "consecutive": 0, "open_for": OpenFor.Milliseconds()})
			// A fresh window after recovery: the failures that opened it no longer count.
			for s := b.now().Unix() - WindowSeconds; s <= b.now().Unix(); s++ {
				_ = b.Redis.Del(ctx, b.bucketKey(org, provider, s)).Err()
			}
		}
	}
}

// Reset closes a breaker (tests and operators).
func (b *Breaker) Reset(ctx context.Context, org, provider string) {
	if b.Redis != nil {
		_ = b.Redis.Del(ctx, b.stateKey(org, provider)).Err()
	}
}
