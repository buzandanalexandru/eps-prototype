package door_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/door"
)

func isNotFound(err error) bool {
	var p *apperr.Problem
	return errors.As(err, &p) && p.Status == 404
}

// Scenarios "Box office prepares a device" (its sync package: manifest, cursor, server time) and
// "Staff of another organiser" (refused, no event data).
func TestDoorAccess(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	ids := x.ticketsFor(t, 3)
	for _, role := range []string{"owner", "admin", "box_office"} {
		s, err := x.door.Sync(ctx, x.staff(role), x.sale.Event, door.SyncRequest{Manifest: true})
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if len(s.Manifest) != 3 || s.ServerTime.IsZero() || s.Event == nil || len(s.Products) == 0 {
			t.Fatalf("%s's sync package: %d tickets, event %v", role, len(s.Manifest), s.Event)
		}
	}
	other := x.NewSale(t, 10, 10)
	for _, st := range []door.Staff{
		{Org: other.Org, Role: "owner", Sub: "x"},    // another organiser's owner
		{Org: x.sale.Org, Role: "finance", Sub: "x"}, // a role without door rights
		{Org: x.sale.Org, Role: "staff", Sub: "x"},   // step 0's staff role
		{Org: "", Role: "owner", Sub: "fan"},         // not organiser staff at all
	} {
		s, err := x.door.Sync(ctx, st, x.sale.Event, door.SyncRequest{Manifest: true})
		if !isNotFound(err) || s != nil {
			t.Fatalf("%+v: %v, want not found and no data", st, err)
		}
		if _, err := x.door.Register(ctx, st, x.sale.Event, "Gate A"); !isNotFound(err) {
			t.Fatalf("%+v registered a device: %v", st, err)
		}
		if _, err := x.door.Upload(ctx, st, x.sale.Event, "00000000-0000-4000-8000-000000000001", []door.Scan{{ID: "00000000-0000-4000-8000-000000000002", TicketID: ids[0], Generation: 1, ScannedAt: t0}}); !isNotFound(err) {
			t.Fatalf("%+v uploaded scans: %v", st, err)
		}
	}
	// The other organiser's own event is theirs alone.
	if _, err := x.door.Sync(ctx, x.staff("owner"), other.Event, door.SyncRequest{}); !isNotFound(err) {
		t.Fatalf("our staff read another organiser's event: %v", err)
	}
}
