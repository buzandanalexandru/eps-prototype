package payments_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// Scenario "Opens under failure": 12 of 20 calls in 20 s fail; every core sees it open.
func TestBreakerOpensUnderFailure(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	c := &clock{t: time.Now()}
	var changes []string
	core1 := &payments.Breaker{Redis: env.Redis, Now: c.now, OnChange: func(_ context.Context, _, p, from, to string) { changes = append(changes, from+">"+to) }}
	core2 := &payments.Breaker{Redis: env.Redis, Now: c.now} // another core instance
	org := "11111111-1111-4111-8111-111111111111"
	// Interleaved so no 5 failures come in a row: the share rule decides.
	pattern := []bool{true, false, false, true, false, false, true, false, true, false, false, true, false, true, false, false, true, false, true, false}
	fails := 0
	for i, ok := range pattern {
		if !ok {
			fails++
		}
		core1.Record(ctx, org, "a", ok)
		if i < len(pattern)-1 && core1.Read(ctx, org, "a").State != payments.Closed {
			t.Fatalf("opened after %d calls", i+1)
		}
		c.t = c.t.Add(500 * time.Millisecond)
	}
	if fails != 12 {
		t.Fatalf("pattern has %d failures", fails)
	}
	if st := core2.Read(ctx, org, "a"); st.State != payments.Open {
		t.Fatalf("the other core sees %s", st.State)
	}
	if core2.Allow(ctx, org, "a") {
		t.Fatal("the other core still routes to A")
	}
	if !core2.Allow(ctx, org, "b") {
		t.Fatal("B must stay available")
	}
	if len(changes) != 1 || changes[0] != "closed>open" {
		t.Fatalf("state events %v", changes)
	}
	// Five failures in a row also open it (another merchant's breaker is untouched by the first).
	org2 := "22222222-2222-4222-8222-222222222222"
	if st := core1.Read(ctx, org2, "a"); st.State != payments.Closed {
		t.Fatalf("another organiser's breaker is %s", st.State)
	}
	for i := 0; i < 5; i++ {
		core1.Record(ctx, org2, "a", false)
	}
	if st := core1.Read(ctx, org2, "a"); st.State != payments.Open {
		t.Fatalf("after 5 failures in a row: %s", st.State)
	}
}

// Scenario "Recovers": after 15 s it lets probes through; 4 of 5 successes close it.
func TestBreakerRecovers(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	c := &clock{t: time.Now()}
	br := &payments.Breaker{Redis: env.Redis, Now: c.now}
	org := "33333333-3333-4333-8333-333333333333"
	for i := 0; i < 5; i++ {
		br.Record(ctx, org, "a", false)
	}
	c.t = c.t.Add(14 * time.Second)
	if br.Allow(ctx, org, "a") {
		t.Fatal("open breaker let a call through before 15 s")
	}
	c.t = c.t.Add(2 * time.Second)
	if st := br.Read(ctx, org, "a"); st.State != payments.HalfOpen {
		t.Fatalf("after 16 s: %s", st.State)
	}
	allowed := 0
	for i := 0; i < 7; i++ {
		if br.Allow(ctx, org, "a") {
			allowed++
		}
	}
	if allowed != payments.Probes {
		t.Fatalf("%d probes allowed, want %d", allowed, payments.Probes)
	}
	// One failed probe is tolerated (4 of 5 may still succeed); a second reopens it for 30 s.
	br.Record(ctx, org, "a", false)
	if st := br.Read(ctx, org, "a"); st.State != payments.HalfOpen {
		t.Fatalf("after one failed probe: %+v", st)
	}
	br.Record(ctx, org, "a", false)
	if st := br.Read(ctx, org, "a"); st.State != payments.Open || st.OpenFor != 30 {
		t.Fatalf("after a failed probe: %+v", st)
	}
	c.t = c.t.Add(31 * time.Second)
	// Scenario: 4 of 5 probes succeed (one fails) and the breaker closes.
	for i, ok := range []bool{true, false, true, true, true} {
		if !br.Allow(ctx, org, "a") {
			t.Fatalf("probe %d refused", i+1)
		}
		br.Record(ctx, org, "a", ok)
	}
	if st := br.Read(ctx, org, "a"); st.State != payments.Closed {
		t.Fatalf("after 4 successful probes: %s", st.State)
	}
	if !br.Allow(ctx, org, "a") {
		t.Fatal("A does not take attempts again")
	}
}

// Review H2 (reported by the correctness review): probe slots handed out but never used (no call
// followed) do not keep the breaker refusing for ever.
func TestBreakerUnusedProbesAreGivenOutAgain(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	c := &clock{t: time.Now()}
	b := &payments.Breaker{Redis: env.Redis, Now: c.now}
	org := "44444444-4444-4444-8444-444444444444"
	for i := 0; i < 5; i++ {
		b.Record(ctx, org, "a", false)
	}
	c.t = c.t.Add(16 * time.Second)
	for i := 0; i < 5; i++ {
		_ = b.Allow(ctx, org, "a")
	}
	if b.Allow(ctx, org, "a") {
		t.Fatal("a sixth probe at once")
	}
	c.t = c.t.Add(10 * time.Minute)
	if !b.Allow(ctx, org, "a") {
		t.Fatalf("10 minutes later the breaker is %s and still refuses every call", b.Read(ctx, org, "a").State)
	}
	// Read changes nothing.
	before := b.Read(ctx, org, "a")
	_ = b.Read(ctx, org, "a")
	if after := b.Read(ctx, org, "a"); after.State != before.State {
		t.Fatal("Read changed the state")
	}
}
