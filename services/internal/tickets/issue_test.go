package tickets_test

import (
	"context"
	"sync"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func paidEvent(o *checkout.Order) outbox.Envelope {
	return outbox.Envelope{ID: "ev-" + o.ID, Type: "order.paid", OrganiserID: o.OrganiserID, EventID: o.EventID, Payload: []byte(`{"orderId":"` + o.ID + `"}`)}
}

// Scenario "Paid order gets its tickets": one ticket per paid unit, generation 1, valid, the
// order's snapshot version, the order issued and one order.issued event listing them.
func TestPaidOrderGetsItsTickets(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	o := x.paid(t, salestest.Buyer(), ga(2), addon(1))
	if err := x.svc.HandleEvent(ctx, paidEvent(o)); err != nil {
		t.Fatal(err)
	}
	list := x.ticketsOf(t, x.sale.Org, o.ID)
	if len(list) != 3 {
		t.Fatalf("%d tickets, want 3", len(list))
	}
	for _, tk := range list {
		if tk.Generation != 1 || tk.State != "valid" || tk.EventID != o.EventID {
			t.Fatalf("ticket %+v", tk)
		}
	}
	if n := x.count(t, "select count(*) from sales.tickets where order_id = $1 and snapshot_version = $2", o.ID, o.SnapshotVersion); n != 3 {
		t.Fatalf("%d tickets with the order's snapshot version", n)
	}
	if n := x.count(t, "select count(*) from sales.outbox where type = 'order.issued' and payload->>'orderId' = $1 and jsonb_array_length(payload->'ticketIds') = 3", o.ID); n != 1 {
		t.Fatalf("%d order.issued events listing 3 tickets, want 1", n)
	}
	got, _ := x.Svc.GetIn(ctx, x.sale.Org, o.ID)
	if got.State != checkout.Issued {
		t.Fatalf("order %s, want issued", got.State)
	}
}

// Scenario "Declined or expired order gets no ticket" (and a rehearsal order never gets one).
func TestNoTicketWithoutPayment(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	held := x.sale.Hold(t, salestest.Buyer(), ga(1))
	// Even a stray order.paid for an unpaid order issues nothing.
	if err := x.svc.HandleEvent(ctx, paidEvent(held)); err != nil {
		t.Fatal(err)
	}
	if n, _ := x.svc.Issue(ctx, x.sale.Org, held.ID); n != 0 {
		t.Fatalf("issued %d for a held order", n)
	}
	cancelled := x.sale.Hold(t, salestest.Buyer(), ga(1))
	if _, err := x.Svc.Cancel(ctx, cancelled); err != nil {
		t.Fatal(err)
	}
	if _, err := x.svc.Issue(ctx, x.sale.Org, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	// A rehearsal order (step 6): paid, never issued.
	reh := x.paid(t, salestest.Buyer(), ga(1))
	if _, err := x.Pool.Exec(ctx, "update sales.orders set rehearsal = true where id = $1", reh.ID); err != nil {
		t.Fatal(err)
	}
	if !checkout.Issuable(&checkout.Order{}) || checkout.Issuable(&checkout.Order{Rehearsal: true}) {
		t.Fatal("Issuable rule changed")
	}
	if err := x.svc.HandleEvent(ctx, paidEvent(reh)); err != nil {
		t.Fatal(err)
	}
	if n := x.count(t, "select count(*) from sales.tickets where order_id = any($1::uuid[])", []string{held.ID, cancelled.ID, reh.ID}); n != 0 {
		t.Fatalf("%d tickets for unpaid, cancelled or rehearsal orders", n)
	}
	if n, err := x.svc.Backstop(ctx, 0, 100); err != nil || n != 0 {
		t.Fatalf("backstop issued %d (%v): a rehearsal order is never issued", n, err)
	}
}

// Scenario "Event delivered twice": duplicates and two replicas at once still make one ticket per
// unit and one order.issued event.
func TestIssuingIsIdempotent(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	o := x.paid(t, salestest.Buyer(), ga(3))
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- x.svc.HandleEvent(ctx, paidEvent(o))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := x.svc.HandleEvent(ctx, paidEvent(o)); err != nil {
		t.Fatal(err)
	}
	if n := len(x.ticketsOf(t, x.sale.Org, o.ID)); n != 3 {
		t.Fatalf("%d tickets after duplicate deliveries, want 3", n)
	}
	if n := x.count(t, "select count(*) from sales.outbox where type = 'order.issued' and payload->>'orderId' = $1", o.ID); n != 1 {
		t.Fatalf("%d order.issued events, want 1", n)
	}
}

// The step 7 deploy: orders step 4's stub marked issued without tickets get them from the backfill,
// and paid orders whose event was lost get them from the backstop.
func TestBackfillAndBackstop(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	stub := x.paid(t, salestest.Buyer(), ga(2))
	if err := x.Svc.Issue(ctx, x.sale.Org, stub.ID); err != nil { // step 4's stub: issued, no tickets
		t.Fatal(err)
	}
	lost := x.paid(t, salestest.Buyer(), ga(1))
	n, err := x.svc.RunBackfill(ctx)
	if err != nil || n != 1 {
		t.Fatalf("backfill issued %d orders (%v), want 1", n, err)
	}
	if got := len(x.ticketsOf(t, x.sale.Org, stub.ID)); got != 2 {
		t.Fatalf("stub-issued order has %d tickets, want 2", got)
	}
	if n, _ := x.svc.RunBackfill(ctx); n != 0 {
		t.Fatalf("second backfill issued %d, want 0 (idempotent)", n)
	}
	if n, err := x.svc.Backstop(ctx, 0, 100); err != nil || n != 1 {
		t.Fatalf("backstop issued %d (%v), want the lost order", n, err)
	}
	if got := len(x.ticketsOf(t, x.sale.Org, lost.ID)); got != 1 {
		t.Fatalf("lost order has %d tickets", got)
	}
}
