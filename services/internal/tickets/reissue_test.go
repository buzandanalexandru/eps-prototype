package tickets_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Scenario "Reissue revokes the old generation".
func TestReissueRevokesOldGeneration(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	_, list := x.issued(t, salestest.Buyer(), ga(1))
	tk := list[0]
	b := salestest.Buyer()
	gen, err := x.svc.Reissue(ctx, x.sale.Org, tk.ID, 1, tickets.Change{Reason: "transferred", HolderKind: "account", HolderID: b.ID})
	if err != nil || gen != 2 {
		t.Fatalf("reissue: gen %d, %v", gen, err)
	}
	now := x.ticketsOf(t, x.sale.Org, list[0].OrderID)[0]
	if now.Generation != 2 || now.HolderID != b.ID {
		t.Fatalf("after reissue %+v", now)
	}
	if n := x.count(t, "select count(*) from sales.door_feed where ticket_id = $1 and kind = 'revoked' and generation = 2", tk.ID); n != 1 {
		t.Fatalf("%d revocation rows saying generations below 2 are invalid, want 1", n)
	}
}

// Scenario "Concurrent reissues": one wins, the other conflicts, the generation moves by one.
func TestConcurrentReissues(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	_, list := x.issued(t, salestest.Buyer(), ga(1))
	tk := list[0]
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := salestest.Buyer()
			_, err := x.svc.Reissue(ctx, x.sale.Org, tk.ID, 1, tickets.Change{Reason: "transferred", HolderKind: "account", HolderID: b.ID})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, tickets.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatalf("%d succeeded, %d conflicted; want 1 and 1", ok, conflicts)
	}
	if g := x.ticketsOf(t, x.sale.Org, tk.OrderID)[0].Generation; g != 2 {
		t.Fatalf("generation %d, want 2", g)
	}
}
