package tickets_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/door"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Scenarios "Door device reads revocations since its cursor" (only newer rows, in order, with a new
// cursor) and "Other organiser's staff are refused".
func TestRevocationFeedSinceCursor(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	d := &door.Service{Pool: x.Pool, Snaps: x.Store}
	staff := door.Staff{Org: x.sale.Org, Role: "box_office", Sub: "s"}
	_, list := x.issued(t, salestest.Buyer(), ga(3))
	if _, err := x.svc.Reissue(ctx, x.sale.Org, list[0].ID, 1, tickets.Change{Reason: "transferred", HolderKind: "account", HolderID: "u2"}); err != nil {
		t.Fatal(err)
	}
	first, err := d.Sync(ctx, staff, x.sale.Event, door.SyncRequest{})
	if err != nil || len(first.Feed) != 1 || first.Feed[0].Kind != "revoked" {
		t.Fatalf("first sync: %+v %v", first, err)
	}
	for _, id := range []string{list[1].ID, list[2].ID} {
		if _, err := x.svc.Reissue(ctx, x.sale.Org, id, 1, tickets.Change{Reason: "cancelled", Cancel: true}); err != nil {
			t.Fatal(err)
		}
	}
	next, err := d.Sync(ctx, staff, x.sale.Event, door.SyncRequest{Since: first.Cursor})
	if err != nil || len(next.Feed) != 2 || next.Feed[0].TicketID != list[1].ID || next.Feed[1].TicketID != list[2].ID || next.Cursor != first.Cursor+2 {
		t.Fatalf("since %d: %+v %v", first.Cursor, next, err)
	}
	if again, _ := d.Sync(ctx, staff, x.sale.Event, door.SyncRequest{Since: next.Cursor}); len(again.Feed) != 0 || again.Cursor != next.Cursor {
		t.Fatalf("nothing new: %+v", again)
	}
	other := x.NewSale(t, 10, 10)
	_, err = d.Sync(ctx, door.Staff{Org: other.Org, Role: "owner", Sub: "o"}, x.sale.Event, door.SyncRequest{})
	var p *apperr.Problem
	if !errors.As(err, &p) || p.Status != 404 {
		t.Fatalf("another organiser's staff: %v, want not found", err)
	}
}
