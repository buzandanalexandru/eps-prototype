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
	n, err := (&checkout.Sweeper{Svc: env.Svc}).SweepOrphans(ctx, time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("released %d (%v), want 1", n, err)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 1 {
		t.Fatalf("used %d after the orphan sweep, want 1", used)
	}
}
