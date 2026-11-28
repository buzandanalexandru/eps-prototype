package checkout_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func expire(t *testing.T, env *salestest.Env, sale *salestest.Sale, o *checkout.Order) {
	t.Helper()
	sale.SetHoldDeadline(t, o.ID, time.Now().Add(-time.Second))
	got, err := env.Svc.Expire(context.Background(), sale.Org, o.ID)
	if err != nil || got.State != checkout.Expired {
		t.Fatalf("expire: %v %v", got, err)
	}
}

// Scenario "Stock still free": a late success re-takes the stock and pays the order.
func TestLatePaymentStockStillFree(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 10, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	expire(t, env, sale, o)
	out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "a1", "late webhook", nil)
	if err != nil || !out.Paid || out.Refund {
		t.Fatalf("late success: %+v %v", out, err)
	}
	if out.Order.State != checkout.Paid {
		t.Fatalf("state %s", out.Order.State)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 2 {
		t.Fatalf("used %d, want the 2 re-taken", used)
	}
}

// Scenario "Stock gone": the order is refunded with EPS-HOLD-EXPIRED and nobody else's stock moves.
func TestLatePaymentStockGone(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 2, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	expire(t, env, sale, o)
	other := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "a1", "late webhook", nil)
	if err != nil || out.Paid || !out.Refund {
		t.Fatalf("late success on sold stock: %+v %v", out, err)
	}
	if out.Order.State != checkout.Refunded || out.Order.LastErrorCode == nil || *out.Order.LastErrorCode != apperr.HoldExpired {
		t.Fatalf("order %s / %v, want refunded with %s", out.Order.State, out.Order.LastErrorCode, apperr.HoldExpired)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 2 {
		t.Fatalf("used %d, want only the other fan's 2", used)
	}
	if got, _ := env.Svc.GetIn(ctx, sale.Org, other.ID); got.State != checkout.Held {
		t.Fatalf("the other fan's order is %s", got.State)
	}
}
