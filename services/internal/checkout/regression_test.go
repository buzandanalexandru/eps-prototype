package checkout_test

import (
	"context"
	"sync"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Review H1 (reported by the correctness review): two late successes of one attempt at once
// (webhook and reconciler) on an expired order take its stock once.
func TestConcurrentLateSuccessTakesStockOnce(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 10, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	expire(t, env, sale, o)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := env.Svc.Complete(ctx, sale.Org, o.ID, "attempt-1", "late", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
	if got.State != checkout.Paid {
		t.Fatalf("state %s", got.State)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 2 {
		t.Fatalf("used %d, want 2", used)
	}
}

// Review D-a: a hand-off from an earlier snapshot version is not priced from it.
func TestEarlierVersionIsToldPricesChanged(t *testing.T) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, 100, 100)
	v1 := sale.Version
	sale.Publish(t, nil)
	req := sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	req.Version = v1
	_, _, err := env.Svc.CreateOrder(context.Background(), salestest.Buyer(), req, "")
	if code(err) != "EPS-PRICES-CHANGED" {
		t.Fatalf("want EPS-PRICES-CHANGED, got %v", err)
	}
}
