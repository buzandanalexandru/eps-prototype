package botdefence_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func pay(t *testing.T, env *onsaletest.Env, sale *salestest.Sale, o *checkout.Order) {
	t.Helper()
	out, err := env.Svc.Complete(context.Background(), sale.Org, o.ID, "pay-"+o.ID[:8], "test payment", nil)
	if err != nil || !out.Paid {
		t.Fatalf("pay: %+v %v", out, err)
	}
	if err := env.Life.End(context.Background(), out.Order, "used"); err != nil {
		t.Fatal(err)
	}
}

// Scenario "Cap across presale and general sale": with a cap of 6, a fan who bought 4 in the
// presale cannot hold 3 in the general sale; the answer says 2 more can be bought.
func TestCapAcrossSaleMoments(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	sale := env.Env.NewSale(t, 100, 100)
	pre := env.Open()
	pre.OpensAt = env.Clock.Now().Add(-10 * time.Minute).Truncate(time.Second)
	pre.CapPerPerson, pre.CapPerOrder = 6, 4
	presale := env.NewSale(t, sale, pre)
	fan := env.Admitted(t, presale, onsaletest.NewFan())
	pay(t, env, sale, sale.Hold(t, fan.Buyer, ga(4)))
	general := env.NewSale(t, sale, func() (st salesSettings) {
		st = env.Open()
		st.CapPerPerson, st.CapPerOrder = 6, 4
		return
	}())
	again := &onsaletest.Fan{Buyer: checkout.Buyer{ID: fan.Buyer.ID, Kind: "account"}, Session: fan.Session + "-2", IP: fan.IP}
	env.Admitted(t, general, again)
	_, _, err := env.Svc.CreateOrder(ctx, again.Buyer, sale.Request(ga(3)), "")
	c, d := problem(err)
	if c != apperr.CapReached || d["left"] != 2 || d["max"] != 6 {
		t.Fatalf("got %v %v", err, d)
	}
	o := sale.Hold(t, again.Buyer, ga(2))
	if o.SaleID == nil || *o.SaleID != general.ID {
		t.Fatalf("held under sale %v", o.SaleID)
	}
}

// Scenario "Guest email variants": ana.pop+1@gmail.com and anapop@gmail.com count toward one cap.
func TestGuestEmailVariants(t *testing.T) {
	if botdefence.NormaliseEmail(" Ana.Pop+1@GMail.com") != "anapop@gmail.com" || botdefence.NormaliseEmail("ana.pop+x@example.com") != "ana.pop@example.com" {
		t.Fatal("normalisation")
	}
	env := onsaletest.Setup(t)
	ctx := context.Background()
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.CapPerPerson, st.CapPerOrder = 6, 4
	s := env.NewSale(t, sale, st)
	g1 := &onsaletest.Fan{Buyer: salestest.Guest(), Session: "guest-one-session-secret-000000", IP: "192.0.2.21"}
	env.Admitted(t, s, g1)
	o := sale.Hold(t, g1.Buyer, ga(4))
	o, err := env.Svc.SetDetails(ctx, o, checkout.Details{Name: "Ana Pop", Email: "ana.pop+1@gmail.com"})
	if err != nil {
		t.Fatal(err)
	}
	pay(t, env, sale, o)
	g2 := &onsaletest.Fan{Buyer: salestest.Guest(), Session: "guest-two-session-secret-000000", IP: "192.0.2.22"}
	env.Admitted(t, s, g2)
	o2 := sale.Hold(t, g2.Buyer, ga(3)) // under its own guest id until it gives an email
	_, err = env.Svc.SetDetails(ctx, o2, checkout.Details{Name: "Ana Pop", Email: "anapop@gmail.com"})
	if c, d := problem(err); c != apperr.CapReached || d["left"] != 2 {
		t.Fatalf("the second email variant: %v %v", err, d)
	}
}

// Scenario "Parallel holds at the cap": two sessions of one account each ask for 3 with 4 left on
// the cap; exactly one succeeds.
func TestParallelHoldsAtTheCap(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	id := botdefence.IdentityHash("org", botdefence.Identity("account", "user-1", ""))
	var ok atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, _, err := env.Caps.Hold(ctx, "00000000-0000-4000-8000-0000000000aa", "00000000-0000-4000-8000-0000000000bb", id, []string{"order-a", "order-b"}[i], 3, 6, 2, time.Now().Add(time.Minute))
			if err != nil {
				t.Error(err)
			}
			if got {
				ok.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Fatalf("%d of 2 parallel holds passed the cap, want exactly 1", ok.Load())
	}
	// A lapsed hold stops counting by itself.
	caps := &botdefence.Caps{Redis: env.Redis, Now: func() time.Time { return time.Now().Add(2 * time.Minute) }}
	if got, left, _ := caps.Hold(ctx, "00000000-0000-4000-8000-0000000000aa", "00000000-0000-4000-8000-0000000000bb", id, "order-c", 3, 6, 2, time.Now().Add(5*time.Minute)); !got || left != 1 {
		t.Fatalf("after the first hold lapsed: %v, %d left", got, left)
	}
}

// Scenario "Too many in one order": 5 tickets with a cap of 4 per order is refused with the cap.
func TestPerOrderCap(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.CapPerOrder, st.CapPerPerson = 4, 6
	s := env.NewSale(t, sale, st)
	f := env.Admitted(t, s, onsaletest.NewFan())
	_, _, err := env.Svc.CreateOrder(context.Background(), f.Buyer, sale.Request(ga(5)), "")
	if c, d := problem(err); c != apperr.HoldLimit || d["max"] != 4 {
		t.Fatalf("got %v %v", err, d)
	}
	if used := sale.Used(t); len(used) != 0 {
		t.Fatalf("stock taken: %v", used)
	}
}
