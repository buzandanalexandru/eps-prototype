package checkout_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenario "Concurrent holds never oversell": 500 holds of 1 on 100 places give exactly 100.
func TestConcurrentHoldsNeverOversell(t *testing.T) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, 100, 1000)
	ctx := context.Background()
	var ok, soldOut, other atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := env.Svc.CreateOrder(ctx, salestest.Guest(), sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1}), "")
			switch code(err) {
			case "":
				ok.Add(1)
			case apperr.HoldSoldOut:
				soldOut.Add(1)
			default:
				other.Add(1)
				t.Log(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 100 || soldOut.Load() != 400 || other.Load() != 0 {
		t.Fatalf("%d held, %d sold out, %d other errors; want 100, 400, 0", ok.Load(), soldOut.Load(), other.Load())
	}
	used := sale.Used(t)[salestest.PoolArena]
	if used != 100 {
		t.Fatalf("pool used %d, want exactly 100 (never below zero left)", used)
	}
}
