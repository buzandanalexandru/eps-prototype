package checkout_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenario "Hold lapses": within 5 seconds of the deadline the stock is back and the order expired.
func TestHoldLapses(t *testing.T) {
	env := salestest.Setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 3})
	sale.SetHoldDeadline(t, o.ID, time.Now().Add(500*time.Millisecond))
	go (&checkout.Sweeper{Svc: env.Svc}).Run(ctx, time.Second)
	deadline := time.Now().Add(500*time.Millisecond + 5*time.Second)
	for time.Now().Before(deadline) {
		got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
		if got.State == checkout.Expired {
			if used := sale.Used(t)[salestest.PoolArena]; used != 0 {
				t.Fatalf("expired but %d still used", used)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the hold was not released within 5 seconds of its deadline")
}

// Scenario "Payment in flight keeps the hold".
func TestPaymentInFlightKeepsTheHold(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	beginPaying(t, env, sale.Org, o.ID)
	// The open attempt (as the payments service records it).
	if _, err := env.Pool.Exec(ctx, `insert into sales.payment_attempts (id, organiser_id, purpose, order_id, buyer_id, provider, provider_key, method, amount_minor, currency, state, deadline_at)
		values (gen_random_uuid(), $1, 'order', $2, 'b', 'a', gen_random_uuid()::text, 'hosted_card', 100, 'RON', 'submitted', now() + interval '2 minutes')`, sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
	if got.HoldExpiresAt.Before(time.Now().Add(checkout.AttemptLimit)) {
		t.Fatalf("hold ends %s, before the attempt limit", got.HoldExpiresAt)
	}
	// Even with the deadline in the past, a paying order is not expired by the sweeper.
	sale.SetHoldDeadline(t, o.ID, time.Now().Add(-time.Minute))
	if _, err := (&checkout.Sweeper{Svc: env.Svc}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = env.Svc.GetIn(ctx, sale.Org, o.ID)
	if got.State != checkout.Paying {
		t.Fatalf("state %s, want paying", got.State)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 1 {
		t.Fatalf("used %d, want the hold kept", used)
	}
}

// Inventory holds without a live order (a crash between taking stock and recording the order)
// are released by the orphan sweep.
func TestOrphanHoldsReleased(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1}) // loads the counters
	inv := env.Svc.Inv
	h := checkout.Hold{OrganiserID: sale.Org, EventID: sale.Event, ID: "0e000000-0000-4000-8000-000000000001", Draws: map[string]int64{salestest.PoolArena: 4}, Deadline: time.Now().Add(-2 * time.Minute)}
	if err := inv.Reserve(ctx, h, map[string]int64{salestest.PoolArena: 100}); err != nil {
		t.Fatal(err)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 5 {
		t.Fatalf("used %d", used)
	}
	n, err := (&checkout.Sweeper{Svc: env.Svc, Index: env.Index}).SweepOrphans(ctx, time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("released %d (%v), want 1", n, err)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 1 {
		t.Fatalf("used %d after the orphan sweep, want 1", used)
	}
}

// Review H4: a paying order whose attempt is no longer open (the press ended without one) is
// expired at its deadline instead of staying paying for ever.
func TestPayingWithoutOpenAttemptExpires(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	beginPaying(t, env, sale.Org, o.ID)
	sale.SetHoldDeadline(t, o.ID, time.Now().Add(-time.Second))
	if n, err := (&checkout.Sweeper{Svc: env.Svc}).RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("expired %d (%v)", n, err)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 0 {
		t.Fatalf("used %d", used)
	}
}

// Review H2: 1,000 holds lapsing at once are all released within the 5-second rule.
func TestThousandHoldsLapseWithinFiveSeconds(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 2000, 2000)
	for i := 0; i < 1000; i++ {
		sale.Hold(t, salestest.Guest(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	}
	if _, err := env.Pool.Exec(ctx, "update sales.orders set hold_expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	n, err := (&checkout.Sweeper{Svc: env.Svc}).RunOnce(ctx)
	if err != nil || n != 1000 {
		t.Fatalf("expired %d (%v)", n, err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s", d)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 0 {
		t.Fatalf("used %d", used)
	}
	t.Logf("1,000 holds released in %s", time.Since(start))
}
