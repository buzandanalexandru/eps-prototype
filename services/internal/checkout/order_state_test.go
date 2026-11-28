package checkout_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func beginPaying(t *testing.T, env *salestest.Env, org, orderID string) {
	t.Helper()
	err := db.WithTx(context.Background(), env.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, _, err := env.Svc.BeginPayingTx(context.Background(), tx, orderID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Scenario "Allowed path to issued".
func TestAllowedPathToIssued(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	beginPaying(t, env, sale.Org, o.ID)
	if out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "provider confirmed", nil); err != nil || !out.Paid {
		t.Fatalf("complete: %+v %v", out, err)
	}
	if err := env.Svc.Issue(ctx, sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
	want := []string{"held", "paying", "paid", "issued"}
	if len(got.History) != len(want) {
		t.Fatalf("history %+v", got.History)
	}
	for i, h := range got.History {
		if h.To != want[i] || h.At.IsZero() {
			t.Fatalf("history[%d] = %+v, want %s with a time", i, h, want[i])
		}
	}
}

// Scenario "Racing transitions": two transitions of one order at the same moment, one wins; and
// the sweeper racing a payment never leaves stock and state apart.
func TestRacingTransitions(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	var wg sync.WaitGroup
	moved := make(chan string, 2)
	for _, to := range []string{checkout.Expired, checkout.Paying} {
		wg.Add(1)
		go func(to string) {
			defer wg.Done()
			_ = db.WithTx(ctx, env.Pool, db.RoleCore, sale.Org, func(tx pgx.Tx) error {
				_, ok, err := checkout.Transition(ctx, tx, o.ID, to, "race", "")
				if ok {
					moved <- to
				}
				return err
			})
		}(to)
	}
	wg.Wait()
	close(moved)
	n := 0
	for range moved {
		n++
	}
	if n != 1 {
		t.Fatalf("%d transitions won, want exactly 1", n)
	}

	// The sweeper and a payment success on a payment-failed order whose deadline passed.
	for i := 0; i < 10; i++ {
		o := sale.Hold(t, salestest.Guest(), checkout.Item{ProductID: salestest.ProductPlus, Qty: 1})
		beginPaying(t, env, sale.Org, o.ID)
		_ = db.WithTx(ctx, env.Pool, db.RoleCore, sale.Org, func(tx pgx.Tx) error { return env.Svc.FailPayingTx(ctx, tx, o.ID, "EPS-PAY-DECLINED") })
		sale.SetHoldDeadline(t, o.ID, time.Now().Add(-time.Second))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = env.Svc.Expire(ctx, sale.Org, o.ID) }()
		go func() {
			defer wg.Done()
			if _, err := env.Svc.Complete(ctx, sale.Org, o.ID, "late success", nil); err != nil {
				t.Log("complete:", err)
			}
		}()
		wg.Wait()
		got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
		if got.State != checkout.Paid {
			t.Fatalf("round %d: state %s; the payment must end paid (stock was free)", i, got.State)
		}
	}
	// Every paid order's stock is committed and nothing else is held.
	if used := sale.Used(t)[salestest.PoolPlus]; used != 10 {
		t.Fatalf("GA Plus used %d, want 10 (the 10 paid orders)", used)
	}
}

// Scenario "Forbidden transition".
func TestForbiddenTransition(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	sale.SetHoldDeadline(t, o.ID, time.Now().Add(-time.Second))
	if _, err := env.Svc.Expire(ctx, sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(ctx, env.Pool, db.RoleCore, sale.Org, func(tx pgx.Tx) error {
		cur, ok, err := checkout.Transition(ctx, tx, o.ID, checkout.Paying, "late click", "")
		if ok || cur != checkout.Expired {
			t.Fatalf("expired → paying moved=%v (state %s)", ok, cur)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
	if got.State != checkout.Expired {
		t.Fatalf("state %s", got.State)
	}
	if checkout.CanMove(checkout.Expired, checkout.Paying) || checkout.CanMove(checkout.Issued, checkout.Held) {
		t.Fatal("the table allows a forbidden move")
	}
}

// Scenario "Cancel releases stock".
func TestCancelReleasesStock(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 2, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	got, err := env.Svc.Cancel(ctx, o)
	if err != nil || got.State != checkout.Cancelled {
		t.Fatalf("cancel: %v %v", got, err)
	}
	if used := sale.Used(t)[salestest.PoolArena]; used != 0 {
		t.Fatalf("still used %d", used)
	}
	// Free for another fan at once.
	sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
}
