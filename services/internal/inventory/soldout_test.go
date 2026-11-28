package inventory_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Scenario "Held, not sold": a pool with 0 free and 40 held reads "all held", not sold out; once
// the holds are paid it is sold out.
func TestHeldIsNotSoldOut(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 40}
	for i := 0; i < 20; i++ {
		if err := f.inv.Reserve(ctx, f.hold(fmt.Sprintf("h%d", i), map[string]int64{"arena": 2}, time.Minute), caps); err != nil {
			t.Fatal(err)
		}
	}
	l := f.level(t, "arena")
	if l.Free != 0 || l.Held != 40 || l.SoldOut() || !l.AllHeld() {
		t.Fatalf("level %+v: want 0 free, 40 held, all held (not sold out)", l)
	}
	for i := 0; i < 20; i++ {
		if err := f.inv.Commit(ctx, f.org, f.event, fmt.Sprintf("h%d", i), caps); err != nil {
			t.Fatal(err)
		}
	}
	if l := f.level(t, "arena"); !l.SoldOut() || l.AllHeld() || l.Sold != 40 {
		t.Fatalf("level %+v: want sold out", l)
	}
}

// The shop's answer for "Held, not sold": a full pool with live holds is sold out for now but
// marked allHeld ("All tickets are held right now; some may come back"); once paid, plain sold out.
func TestShopSaysAllHeld(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 4, 100)
	inv := env.Svc.Inv.(*inventory.Sharded)
	u := checkout.Usage{Svc: env.Svc, HeldBy: inv}
	av := &snapshot.AvailabilityService{Store: env.Store, Usage: u, Held: u, TTL: time.Millisecond}
	a := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	b := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	got, err := av.Get(ctx, sale.Event)
	if err != nil {
		t.Fatal(err)
	}
	ga := find(got, salestest.ProductGA)
	if ga == nil || ga.Status != snapshot.SoldOut || !ga.AllHeld {
		t.Fatalf("GA %+v, want sold out with allHeld", ga)
	}
	for _, o := range []*checkout.Order{a, b} {
		if out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "pay-"+o.ID[:8], "test", nil); err != nil || !out.Paid {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond)
	got, _ = av.Get(ctx, sale.Event)
	if ga := find(got, salestest.ProductGA); ga.Status != snapshot.SoldOut || ga.AllHeld {
		t.Fatalf("after payment GA %+v, want plain sold out", ga)
	}
}

func find(a snapshot.Availability, id string) *snapshot.ProductAvailability {
	for i := range a.Products {
		if a.Products[i].ProductID == id {
			return &a.Products[i]
		}
	}
	return nil
}
