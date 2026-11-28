package checkout_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func code(err error) string {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// Scenario "Hold succeeds across shared pools".
func TestHoldAcrossSharedPools(t *testing.T) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, 1000, 100)
	start := time.Now()
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductPlus, Qty: 2})
	if o.State != checkout.Held {
		t.Fatalf("state %s", o.State)
	}
	if d := o.HoldExpiresAt.Sub(start); d < 8*time.Minute-2*time.Second || d > 8*time.Minute+2*time.Second {
		t.Fatalf("deadline %s ahead, want 8 minutes", d)
	}
	if o.SnapshotVersion != sale.Version {
		t.Fatalf("order records version %d, want %d", o.SnapshotVersion, sale.Version)
	}
	used := sale.Used(t)
	if used[salestest.PoolArena] != 2 || used[salestest.PoolPlus] != 2 {
		t.Fatalf("pools used %v, want 2 and 2", used)
	}
}

// Scenario "One pool short means nothing is held".
func TestOnePoolShortHoldsNothing(t *testing.T) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, 1000, 1)
	_, _, err := env.Svc.CreateOrder(context.Background(), salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductPlus, Qty: 2}), "")
	if code(err) != apperr.HoldSoldOut {
		t.Fatalf("want %s, got %v", apperr.HoldSoldOut, err)
	}
	var p *apperr.Problem
	errors.As(err, &p)
	if p.Data["productId"] != salestest.ProductPlus {
		t.Fatalf("the problem names %v, want the GA Plus product", p.Data["productId"])
	}
	if used := sale.Used(t); used[salestest.PoolArena] != 0 || used[salestest.PoolPlus] != 0 {
		t.Fatalf("stock taken: %v", used)
	}
}

// Scenario "Capacity lowered below what is taken".
func TestCapacityLoweredBelowTaken(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 10, 100)
	var held []*checkout.Order
	for i := 0; i < 5; i++ {
		held = append(held, sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2}))
	}
	// 10 of 10 held; a publish lowers the capacity to 9 (the step 3 gate bypassed).
	sale.SetCapacity(t, 9, 100)
	if _, _, err := env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1}), ""); code(err) != apperr.HoldSoldOut {
		t.Fatalf("a new hold over capacity: want sold out, got %v", err)
	}
	// Confirming the older holds never sells above 9: four pay (8 sold), the fifth cannot.
	paid := 0
	refunded := 0
	for _, o := range held {
		out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "pay-"+o.ID[:8], "test payment", nil)
		if err != nil {
			t.Fatal(err)
		}
		if out.Paid {
			paid++
		}
		if out.Refund {
			refunded++
		}
	}
	if paid != 4 || refunded != 1 {
		t.Fatalf("paid %d and refunded %d, want 4 and 1 (never above 9 sold)", paid, refunded)
	}
}

// Scenario "Hidden product or off-sale event".
func TestHiddenProductOtherSnapshotOrOffSaleRefused(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	other := env.NewSale(t, 100, 100)
	b := salestest.Buyer()
	if _, _, err := env.Svc.CreateOrder(ctx, b, sale.Request(checkout.Item{ProductID: salestest.ProductHidden, Qty: 1}), ""); code(err) != apperr.HoldInvalid {
		t.Fatalf("hidden product: %v", err)
	}
	// A product id that is not in this snapshot.
	if _, _, err := env.Svc.CreateOrder(ctx, b, sale.Request(checkout.Item{ProductID: "0d000000-0000-4000-8000-0000000000ff", Qty: 1}), ""); code(err) != apperr.HoldInvalid {
		t.Fatalf("product of another snapshot: %v", err)
	}
	// A version above the live one.
	req := sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	req.Version = sale.Version + 1
	if _, _, err := env.Svc.CreateOrder(ctx, b, req, ""); code(err) != apperr.HoldInvalid {
		t.Fatalf("unknown version: %v", err)
	}
	other.OffSale(t)
	if _, _, err := env.Svc.CreateOrder(ctx, b, other.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1}), ""); code(err) != apperr.EventOffSale {
		t.Fatalf("off-sale event: %v", err)
	}
	if used := sale.Used(t); len(used) != 0 {
		t.Fatalf("stock taken: %v", used)
	}
	// The per-order maximum and an add-on without its ticket are refused too.
	if _, _, err := env.Svc.CreateOrder(ctx, b, sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 7}), ""); code(err) != apperr.HoldLimit {
		t.Fatalf("over the per-order maximum: %v", err)
	}
	if _, _, err := env.Svc.CreateOrder(ctx, b, sale.Request(checkout.Item{ProductID: salestest.ProductAddOn, Qty: 1}), ""); code(err) != apperr.HoldInvalid {
		t.Fatalf("add-on alone: %v", err)
	}
}

// Scenario "Second hold returns the first order".
func TestSecondHoldReturnsTheFirstOrder(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	b := salestest.Guest()
	first := sale.Hold(t, b, checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	second, existing, err := env.Svc.CreateOrder(ctx, b, sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 3}), "")
	if err != nil {
		t.Fatal(err)
	}
	if !existing || second.ID != first.ID {
		t.Fatalf("second hold made order %s (existing %v), want %s", second.ID, existing, first.ID)
	}
	if used := sale.Used(t); used[salestest.PoolArena] != 2 {
		t.Fatalf("pool used %d, want 2", used[salestest.PoolArena])
	}
}

// Scenario "Double click on Continue".
func TestIdempotentOrderCreation(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	b := salestest.Buyer()
	req := sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	a, _, err := env.Svc.CreateOrder(ctx, b, req, "key-double-click-1")
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := env.Svc.CreateOrder(ctx, b, req, "key-double-click-1")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != c.ID {
		t.Fatalf("two orders: %s and %s", a.ID, c.ID)
	}
	if used := sale.Used(t); used[salestest.PoolArena] != 1 {
		t.Fatalf("stock taken %d times", used[salestest.PoolArena])
	}
	req.Items[0].Qty = 2
	if _, _, err := env.Svc.CreateOrder(ctx, b, req, "key-double-click-1"); code(err) != apperr.IdempotencyMismatch {
		t.Fatalf("same key, other body: %v", err)
	}
}

// Redis lost its counters (a restart): the next hold rebuilds them from the orders.
func TestCountersRebuiltFromOrders(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 3, 100)
	sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	if err := env.Redis.FlushAll(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 2}), ""); code(err) != apperr.HoldSoldOut {
		t.Fatalf("after a Redis restart 2 of 3 are still held: want sold out, got %v", err)
	}
	sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
}

// A day ticket draws on the chosen day; a day that does not exist is refused.
func TestDayInstances(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductDay, Qty: 2, Instance: "2027-07-16"})
	if used := sale.Used(t); used[salestest.PoolDay+"@2027-07-16"] != 2 {
		t.Fatalf("day used %v", used)
	}
	if _, _, err := env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductDay, Qty: 1, Instance: "2027-07-20"}), ""); code(err) != apperr.HoldInvalid {
		t.Fatalf("unknown day: %v", err)
	}
	if _, _, err := env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductDay, Qty: 1}), ""); code(err) != apperr.HoldInvalid {
		t.Fatalf("no day chosen: %v", err)
	}
}
