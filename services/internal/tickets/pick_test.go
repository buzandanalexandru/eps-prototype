package tickets_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Scenario "Pick a day": a flexible day ticket becomes a ticket for the picked day at a new
// generation (the feed revokes the old one); the pick is final and only the ticket's days count.
func TestPickTheDayLater(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	x.sale.Publish(t, func(doc map[string]any) {
		for _, p := range doc["products"].([]any) {
			if m := p.(map[string]any); m["productId"] == salestest.ProductDay {
				m["type"] = "flex_day"
				m["config"] = map[string]any{"days": []any{"2027-07-16", "2027-07-17"}}
				m["poolIds"] = []any{salestest.PoolArena} // a flexible ticket draws on an event pool
			}
		}
	})
	b := salestest.Buyer()
	o := x.paid(t, b, checkout.Item{ProductID: salestest.ProductDay, Qty: 1})
	if _, err := x.svc.Issue(ctx, x.sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	tk := x.ticketsOf(t, x.sale.Org, o.ID)[0]
	before, _ := x.svc.TicketFor(ctx, holder(b), tk.ID)
	if len(before.Items[0].PickDays) != 2 {
		t.Fatalf("pick days %v", before.Items[0].PickDays)
	}
	if _, err := x.svc.PickDay(ctx, holder(b), tk.ID, "2027-07-20"); code(err) != tickets.CodeBadDay {
		t.Fatalf("a day the ticket does not have: %v", err)
	}
	ev, err := x.svc.PickDay(ctx, holder(b), tk.ID, "2027-07-17")
	if err != nil {
		t.Fatal(err)
	}
	it := ev.Items[0]
	if it.Generation != 2 || it.PickedDay == nil || *it.PickedDay != "2027-07-17" || len(it.PickDays) != 0 || it.Day == nil || *it.Day != "2027-07-17" {
		t.Fatalf("after the pick: %+v", it)
	}
	if n := x.count(t, "select count(*) from sales.door_feed where ticket_id = $1 and kind = 'revoked' and generation = 2", tk.ID); n != 1 {
		t.Fatalf("%d revocations of generation 1", n)
	}
	if _, err := x.svc.PickDay(ctx, holder(b), tk.ID, "2027-07-16"); code(err) != tickets.CodeDayPicked {
		t.Fatalf("a second pick: %v", err)
	}
}
