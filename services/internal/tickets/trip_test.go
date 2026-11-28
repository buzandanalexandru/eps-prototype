package tickets_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Scenario "Ticket moves to another fan": a reissued ticket leaves the old holder's trip and
// appears in the new holder's; the trip spans organisers and shows paid orders still issuing.
func TestTripFollowsTheHolder(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	a, b := salestest.Buyer(), salestest.Buyer()
	_, list := x.issued(t, a, ga(2))
	other := x.NewSale(t, 50, 50) // a second organiser
	o2 := other.Hold(t, a, ga(1))
	if out, err := x.Svc.Complete(ctx, other.Org, o2.ID, "p2", "test", nil); err != nil || !out.Paid {
		t.Fatal(err)
	}
	trip, err := x.svc.TripOf(ctx, holder(a))
	if err != nil {
		t.Fatal(err)
	}
	if len(trip.Events) != 2 {
		t.Fatalf("%d events in A's trip, want 2 (two organisers)", len(trip.Events))
	}
	var items, pending int
	for _, e := range trip.Events {
		items += len(e.Items)
		pending += len(e.Pending)
		if e.Name.EN == "" || e.BrandID == "" {
			t.Fatalf("event without its snapshot details: %+v", e)
		}
	}
	if items != 2 || pending != 1 {
		t.Fatalf("A holds %d items and %d pending orders, want 2 and 1 (the second order is still issuing)", items, pending)
	}
	if _, err := x.svc.Reissue(ctx, x.sale.Org, list[0].ID, 1, tickets.Change{Reason: "transferred", HolderKind: "account", HolderID: b.ID}); err != nil {
		t.Fatal(err)
	}
	ta, _ := x.svc.TripOf(ctx, holder(a))
	tb, _ := x.svc.TripOf(ctx, holder(b))
	count := func(tr *tickets.Trip, id string) int {
		n := 0
		for _, e := range tr.Events {
			for _, it := range e.Items {
				if it.ID == id {
					n++
				}
			}
		}
		return n
	}
	if count(ta, list[0].ID) != 0 || count(tb, list[0].ID) != 1 || count(ta, list[1].ID) != 1 {
		t.Fatal("the reissued ticket did not move from A's trip to B's")
	}
}
