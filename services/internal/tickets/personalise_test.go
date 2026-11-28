package tickets_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// gaWith republishes the sale with the GA pass changed by f.
func (x *env) gaWith(t *testing.T, f func(p map[string]any)) {
	t.Helper()
	x.sale.Publish(t, func(doc map[string]any) {
		for _, p := range doc["products"].([]any) {
			if m := p.(map[string]any); m["productId"] == salestest.ProductGA {
				f(m)
			}
		}
	})
}

// Scenarios "Name a ticket before the deadline" (the code appears once it is named) and
// "Deadline has passed" (the name is locked).
func TestPersonaliseUntilTheDeadline(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	deadline := x.now.Add(48 * time.Hour).UTC().Format(time.RFC3339)
	x.gaWith(t, func(p map[string]any) { p["named"] = true; p["personaliseBy"] = deadline })
	b := salestest.Buyer()
	o, list := x.issued(t, b, ga(1))
	tk := list[0]
	if _, err := x.svc.CodesFor(ctx, holder(b), tk.ID); code(err) != tickets.CodeNeedsName {
		t.Fatalf("codes before naming: %v, want %s", err, tickets.CodeNeedsName)
	}
	ev, err := x.svc.Personalise(ctx, holder(b), tk.ID, "  Ana   Pop ")
	if err != nil {
		t.Fatal(err)
	}
	it := ev.Items[0]
	if it.HolderName == nil || *it.HolderName != "Ana Pop" || it.NeedsName || it.NameLocked {
		t.Fatalf("after naming: %+v", it)
	}
	if c, err := x.svc.CodesFor(ctx, holder(b), tk.ID); err != nil || len(c.Codes) == 0 {
		t.Fatalf("codes after naming: %v", err)
	}
	if n := x.count(t, "select count(*) from sales.door_feed where ticket_id = $1 and kind = 'manifest' and data->>'name' = 'Ana Pop'", tk.ID); n != 1 {
		t.Fatalf("%d manifest rows with the name, want 1 (the door learns it)", n)
	}
	if _, err := x.svc.Personalise(ctx, holder(salestest.Buyer()), tk.ID, "Someone"); code(err) != tickets.CodeNotFound {
		t.Fatalf("a stranger renamed it: %v", err)
	}
	// After the deadline the name is locked.
	x.now = x.now.Add(72 * time.Hour)
	if _, err := x.svc.Personalise(ctx, holder(b), tk.ID, "Ion Pop"); code(err) != tickets.CodeNameLocked {
		t.Fatalf("rename after the deadline: %v, want %s", err, tickets.CodeNameLocked)
	}
	if n := x.count(t, "select count(*) from sales.tickets where order_id = $1 and holder_name = 'Ana Pop'", o.ID); n != 1 {
		t.Fatal("the name changed after the deadline")
	}
}
