package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
)

// Scenario "Repeated commit": the sold count rises once.
func TestRepeatedCommit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 1000}
	if err := f.inv.Reserve(ctx, f.hold("paid", map[string]int64{"arena": 3}, time.Minute), caps); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := f.inv.Commit(ctx, f.org, f.event, "paid", caps); err != nil {
			t.Fatal(err)
		}
	}
	if l := f.level(t, "arena"); l.Sold != 3 || l.Held != 0 || l.Free != 997 {
		t.Fatalf("level %+v", l)
	}
	// Releasing a committed hold says so and changes nothing; releasing twice is harmless.
	if r, _ := f.inv.Release(ctx, f.org, f.event, "paid"); r != checkout.ReleaseCommitted {
		t.Fatalf("release of a committed hold: %v", r)
	}
	if err := f.inv.Reserve(ctx, f.hold("gone", map[string]int64{"arena": 2}, time.Minute), caps); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		_, _ = f.inv.Release(ctx, f.org, f.event, "gone")
	}
	if l := f.level(t, "arena"); l.Sold != 3 || l.Held != 0 || l.Free != 997 {
		t.Fatalf("after releases %+v", l)
	}
}

// Scenario "Commit after expiry": commit fails and the stock is unchanged.
func TestCommitAfterExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 1000}
	if err := f.inv.Reserve(ctx, f.hold("late", map[string]int64{"arena": 2}, 100*time.Millisecond), caps); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	// The order sweeper ends the order and releases the hold.
	if err := f.inv.ReleaseMany(ctx, f.org, f.event, []string{"late"}); err != nil {
		t.Fatal(err)
	}
	before := f.level(t, "arena")
	if err := f.inv.Commit(ctx, f.org, f.event, "late", caps); !errors.Is(err, checkout.ErrNoHold) {
		t.Fatalf("want ErrNoHold, got %v", err)
	}
	if after := f.level(t, "arena"); after != before || after.Sold != 0 || after.Free != 1000 {
		t.Fatalf("stock changed: %+v -> %+v", before, after)
	}
}

// Unsell gives a committed hold's units back exactly once.
func TestUnsellOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 600}
	if err := f.inv.Reserve(ctx, f.hold("plan", map[string]int64{"arena": 4}, time.Minute), caps); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.Commit(ctx, f.org, f.event, "plan", caps); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.inv.Unsell(ctx, f.org, f.event, "plan"); err != nil {
			t.Fatal(err)
		}
	}
	if l := f.level(t, "arena"); l != (inventory.Level{Free: 600}) {
		t.Fatalf("level %+v", l)
	}
}
