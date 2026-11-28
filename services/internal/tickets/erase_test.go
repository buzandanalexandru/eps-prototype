package tickets_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Scenario "Delete my personal data": across organisers the person's orders, guest orders with
// their email, answers, tickets, wallets, saved cards, cap and waiting-room identity rows and
// rehearsal orders lose everything that names them; amounts, states and the ledger stay.
func TestEraseMyData(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	me := salestest.Buyer()
	email := "ana.pop@example.test"
	mine := x.sale.Hold(t, me, ga(1))
	if _, err := x.Svc.SetDetails(ctx, mine, checkout.Details{Name: "Ana Pop", Email: email}); err != nil {
		t.Fatal(err)
	}
	if out, err := x.Svc.Complete(ctx, x.sale.Org, mine.ID, "pm", "test", nil); err != nil || !out.Paid {
		t.Fatal(err)
	}
	if _, err := x.svc.Issue(ctx, x.sale.Org, mine.ID); err != nil {
		t.Fatal(err)
	}
	list := x.ticketsOf(t, x.sale.Org, mine.ID)
	// A guest order placed earlier with the same email, at another organiser.
	other := x.NewSale(t, 50, 50)
	g := salestest.Guest()
	go1 := other.Hold(t, g, ga(1))
	if _, err := x.Svc.SetDetails(ctx, go1, checkout.Details{Name: "Ana Pop", Email: email}); err != nil {
		t.Fatal(err)
	}
	if out, err := x.Svc.Complete(ctx, other.Org, go1.ID, "pg", "test", nil); err != nil || !out.Paid {
		t.Fatal(err)
	}
	// A rehearsal order of the same person (the presenter joined a rehearsal).
	reh := x.paid(t, me, ga(1))
	_, _ = x.Pool.Exec(ctx, "update sales.orders set rehearsal = true where id = $1", reh.ID)
	x.topUp(t, me.ID, 3000)
	// Cap ledger and waiting-room rows.
	_, _ = x.Pool.Exec(ctx, "insert into sales.cap_ledger (organiser_id, event_id, identity_hash, order_id, tickets) values ($1, $2, 'h1', $3, 1)", x.sale.Org, x.sale.Event, mine.ID)
	var sale string
	if err := x.Pool.QueryRow(ctx, "insert into sales.sales (id, organiser_id, event_id, name, created_by) values (gen_random_uuid(), $1, $2, 'S', 'test') returning id::text", x.sale.Org, x.sale.Event).Scan(&sale); err != nil {
		t.Fatal(err)
	}
	hash := queue.IdentityHash(x.svc.IdentitySecret, sale, me.ID)
	if _, err := x.Pool.Exec(ctx, "insert into sales.queue_entries (organiser_id, sale_id, qid, identity_hash, joined_at) values ($1, $2, 'qid-12345678', $3, now())", x.sale.Org, sale, hash); err != nil {
		t.Fatal(err)
	}
	var journal int
	_ = x.Pool.QueryRow(ctx, "select count(*) from sales.journal_lines").Scan(&journal)

	out, err := x.svc.Erase(ctx, tickets.Person{Sub: me.ID, Email: email})
	if err != nil {
		t.Fatal(err)
	}
	if out.Organisers != 2 || out.Orders != 3 || out.Wallets < 1 || out.QueueEntries != 1 {
		t.Fatalf("erased %+v", out)
	}
	for _, q := range []string{
		"select count(*) from sales.orders where buyer_id = $1",
		"select count(*) from sales.tickets where holder_id = $1 or prev_holder_id = $1",
		"select count(*) from sales.wallet_accounts where owner = $1",
		"select count(*) from sales.payment_attempts where buyer_id = $1",
		"select count(*) from sales.saved_payment_methods where owner_id = $1",
	} {
		for _, who := range []string{me.ID, g.ID} {
			if n := x.count(t, q, who); n != 0 {
				t.Fatalf("%s for %s: %d rows still name the person", q, who, n)
			}
		}
	}
	if n := x.count(t, "select count(*) from sales.orders where buyer_email = $1 or buyer_name is not null and id = any($2::uuid[])", email, []string{mine.ID, go1.ID, reh.ID}); n != 0 {
		t.Fatalf("%d orders keep a name or email", n)
	}
	if n := x.count(t, "select count(*) from sales.orders where id = any($1::uuid[]) and total_minor > 0", []string{mine.ID, go1.ID, reh.ID}); n != 3 {
		t.Fatal("the orders' amounts were not kept")
	}
	if n := x.count(t, "select count(*) from sales.cap_ledger where order_id = $1", mine.ID); n != 0 {
		t.Fatal("cap ledger rows kept")
	}
	if n := x.count(t, "select count(*) from sales.journal_lines"); n != journal {
		t.Fatalf("ledger lines %d, want %d (append-only, kept)", n, journal)
	}
	if s := x.ticketsOf(t, x.sale.Org, list[0].OrderID)[0].State; s != "cancelled" {
		t.Fatalf("held ticket %s, want cancelled", s)
	}
	if n := x.count(t, "select count(*) from sales.door_feed where ticket_id = $1 and kind = 'revoked' and data->>'reason' = 'erased'", list[0].ID); n != 1 {
		t.Fatal("the door was not told")
	}
	// Refused while an order is in progress.
	you := salestest.Buyer()
	x.sale.Hold(t, you, ga(1))
	if _, err := x.svc.Erase(ctx, tickets.Person{Sub: you.ID}); code(err) != tickets.CodeLiveOrder {
		t.Fatalf("erase with a live order: %v", err)
	}
}
