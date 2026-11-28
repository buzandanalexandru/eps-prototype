package tickets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Scenarios "Plan goes overdue" and "Overdue amount paid": an overdue instalment suspends the
// order's tickets (no codes, "Payment overdue", a suspension in the feed); paying it lifts the
// suspension and codes come back. Events arriving out of order converge on the plan's truth.
func TestOverdueInstalmentSuspendsAndPayingLifts(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	b := salestest.Buyer()
	o, list := x.issued(t, b, ga(1))
	var plan string
	if err := x.Pool.QueryRow(ctx, `insert into sales.instalment_plans (organiser_id, order_id, state, deposit_minor, total_minor) values ($1, $2, 'active', 1000, 55248) returning id::text`, o.OrganiserID, o.ID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := x.Pool.Exec(ctx, `insert into sales.instalment_parts (organiser_id, plan_id, n, due_on, amount_minor, state, grace_until) values ($1, $2, 1, current_date, 27124, 'overdue', current_date + 7)`, o.OrganiserID, plan); err != nil {
		t.Fatal(err)
	}
	ev := func(typ string) outbox.Envelope {
		return outbox.Envelope{ID: typ + o.ID, Type: typ, OrganiserID: o.OrganiserID, Payload: []byte(`{"orderId":"` + o.ID + `"}`)}
	}
	// The "paid" event of an earlier part arriving late changes nothing: the plan is overdue now.
	for _, typ := range []string{"order.instalment_paid", "order.instalment_overdue"} {
		if err := x.svc.HandleEvent(ctx, ev(typ)); err != nil {
			t.Fatal(err)
		}
	}
	if s := x.ticketsOf(t, o.OrganiserID, o.ID)[0].State; s != "suspended" {
		t.Fatalf("state %s, want suspended", s)
	}
	_, err := x.svc.CodesFor(ctx, holder(b), list[0].ID)
	var p *apperr.Problem
	if !errors.As(err, &p) || p.Code != tickets.CodeSuspended {
		t.Fatalf("codes while overdue: %v, want %s", err, tickets.CodeSuspended)
	}
	ev1, err := x.svc.TicketFor(ctx, holder(b), list[0].ID)
	if err != nil || ev1.Items[0].Overdue == nil || ev1.Items[0].Overdue.AmountMinor != 27124 {
		t.Fatalf("ticket view while overdue: %+v %v", ev1, err)
	}
	if n := x.count(t, "select count(*) from sales.door_feed where ticket_id = $1 and kind = 'suspended'", list[0].ID); n != 1 {
		t.Fatalf("%d suspensions in the feed", n)
	}
	// Paid.
	if _, err := x.Pool.Exec(ctx, "update sales.instalment_parts set state = 'paid', paid_at = now() where plan_id = $1", plan); err != nil {
		t.Fatal(err)
	}
	if err := x.svc.HandleEvent(ctx, ev("order.instalment_paid")); err != nil {
		t.Fatal(err)
	}
	if n := x.count(t, "select count(*) from sales.door_feed where ticket_id = $1 and kind = 'lifted'", list[0].ID); n != 1 {
		t.Fatalf("%d lifts in the feed", n)
	}
	if c, err := x.svc.CodesFor(ctx, holder(b), list[0].ID); err != nil || len(c.Codes) == 0 {
		t.Fatalf("codes after paying: %v", err)
	}
	// A plan that defaults cancels the order (step 4): its tickets are cancelled and revoked.
	if _, err := x.Pool.Exec(ctx, "update sales.orders set state = 'cancelled' where id = $1", o.ID); err != nil {
		t.Fatal(err)
	}
	if err := x.svc.HandleEvent(ctx, ev("order.cancelled")); err != nil {
		t.Fatal(err)
	}
	if s := x.ticketsOf(t, o.OrganiserID, o.ID)[0].State; s != "cancelled" {
		t.Fatalf("state %s after the order was cancelled", s)
	}
}
