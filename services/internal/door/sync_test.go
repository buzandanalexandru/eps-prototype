package door_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/door"
)

// Scenario "Two offline devices admit the same ticket": one entry (the first by device time, even
// when it arrives second) and one conflict, which both devices then read from the feed.
func TestTwoOfflineDevicesAdmitTheSameTicket(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	ids := x.ticketsFor(t, 1)
	gateA, gateB := x.device(t, "Gate A"), x.device(t, "Gate B")
	st := x.staff("box_office")
	start, _ := x.door.Sync(ctx, st, x.sale.Event, door.SyncRequest{Manifest: true})
	// B scanned later by its clock but uploads first.
	rb, err := x.door.Upload(ctx, st, x.sale.Event, gateB, []door.Scan{{ID: uuid.NewString(), TicketID: ids[0], Generation: 1, ScannedAt: t0.Add(2 * time.Minute)}})
	if err != nil || rb[0].Result != "admitted" {
		t.Fatalf("B: %+v %v", rb, err)
	}
	ra, err := x.door.Upload(ctx, st, x.sale.Event, gateA, []door.Scan{{ID: uuid.NewString(), TicketID: ids[0], Generation: 1, ScannedAt: t0}})
	if err != nil || ra[0].Result != "admitted" {
		t.Fatalf("A (first by device time): %+v %v", ra, err)
	}
	var admitted, conflicts int
	_ = x.Pool.QueryRow(ctx, "select count(*) filter (where result = 'admitted'), count(*) filter (where result = 'conflict') from sales.ticket_scans where ticket_id = $1", ids[0]).Scan(&admitted, &conflicts)
	if admitted != 1 || conflicts != 1 {
		t.Fatalf("%d entries and %d conflicts, want 1 and 1", admitted, conflicts)
	}
	var gate string
	_ = x.Pool.QueryRow(ctx, "select gate from sales.ticket_scans where ticket_id = $1 and result = 'admitted'", ids[0]).Scan(&gate)
	if gate != "Gate A" {
		t.Fatalf("the entry that counts is at %s, want Gate A (first by device time)", gate)
	}
	// Both devices see the conflict in their feed.
	for _, dev := range []string{gateA, gateB} {
		s, err := x.door.Sync(ctx, st, x.sale.Event, door.SyncRequest{Since: start.Cursor, DeviceID: dev})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range s.Feed {
			if f.Kind == "conflict" && f.TicketID == ids[0] {
				found = true
			}
		}
		if !found {
			t.Fatalf("device %s sees no conflict in %+v", dev, s.Feed)
		}
	}
	// A third scan later than both is a conflict naming the first entry.
	rc, _ := x.door.Upload(ctx, st, x.sale.Event, gateB, []door.Scan{{ID: uuid.NewString(), TicketID: ids[0], Generation: 1, ScannedAt: t0.Add(5 * time.Minute)}})
	if rc[0].Result != "conflict" || rc[0].FirstGate == nil || *rc[0].FirstGate != "Gate A" {
		t.Fatalf("third scan: %+v", rc[0])
	}
}

// Scenario "Scans uploaded after reconnect": 20 offline scans reach the server exactly once, even
// when the upload is retried (also concurrently, as a flaky network does).
func TestOfflineScansUploadExactlyOnce(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	ids := x.ticketsFor(t, 20)
	dev := x.device(t, "Gate B")
	st := x.staff("box_office")
	var scans []door.Scan
	for i, id := range ids {
		scans = append(scans, door.Scan{ID: uuid.NewString(), TicketID: id, Generation: 1, ScannedAt: t0.Add(time.Duration(i) * time.Second)})
	}
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := x.door.Upload(ctx, st, x.sale.Event, dev, scans)
			if err == nil {
				for _, r := range res {
					if r.Result != "admitted" {
						err = fmt.Errorf("scan %s: %s", r.ID, r.Result)
					}
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n, entries int
	_ = x.Pool.QueryRow(ctx, "select count(*) from sales.ticket_scans where event_id = $1", x.sale.Event).Scan(&n)
	_ = x.Pool.QueryRow(ctx, "select count(*) from sales.door_feed where event_id = $1 and kind = 'entry'", x.sale.Event).Scan(&entries)
	if n != 20 || entries != 20 {
		t.Fatalf("%d scans and %d entry rows after retried uploads, want 20 and 20", n, entries)
	}
}
