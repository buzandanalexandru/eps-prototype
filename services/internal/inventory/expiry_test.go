package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
)

// Scenario "Crash after a partial hold": the core stops after taking stock from one pool of a
// two-pool order; the taken stock returns once its deadline passes (the shard sweeper, every
// second in the worker).
func TestCrashAfterPartialHold(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 100, "plus": 100}
	// A full reserve to split both pools, released again.
	if err := f.inv.Reserve(ctx, f.hold("warm", map[string]int64{"arena": 1, "plus": 1}, time.Minute), caps); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inv.Release(ctx, f.org, f.event, "warm"); err != nil {
		t.Fatal(err)
	}
	// The crash: a reserve that stops after its first pool (record left "reserving").
	deadline := time.Now().Add(300 * time.Millisecond)
	if err := f.inv.ReservePartial(ctx, f.hold("crashed", map[string]int64{"arena": 3, "plus": 2}, 0), deadline, "arena"); err != nil {
		t.Fatal(err)
	}
	if l := f.level(t, "arena"); l.Held != 3 {
		t.Fatalf("arena held %d", l.Held)
	}
	sw := &inventory.ShardSweeper{Inv: f.inv}
	sw.RunOnce(ctx) // before the deadline: nothing moves
	if l := f.level(t, "arena"); l.Held != 3 {
		t.Fatalf("released before its deadline: %+v", l)
	}
	time.Sleep(time.Until(deadline) + 50*time.Millisecond)
	start := time.Now()
	sw.RunOnce(ctx)
	if l := f.level(t, "arena"); l.Held != 0 || l.Free != 100 {
		t.Fatalf("after the deadline arena is %+v", l)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %s", d)
	}
	if st, _ := f.inv.HoldState(ctx, f.org, f.event, "crashed"); st != "released" {
		t.Fatalf("record state %q", st)
	}
}

// An extended hold is not swept at its first deadline; a released hold whose shard step was cut
// short is settled by the sweeper.
func TestSweeperFollowsTheRecord(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 100}
	if err := f.inv.Reserve(ctx, f.hold("ext", map[string]int64{"arena": 2}, 200*time.Millisecond), caps); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.Extend(ctx, f.org, f.event, "ext", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	(&inventory.ShardSweeper{Inv: f.inv}).RunOnce(ctx)
	if l := f.level(t, "arena"); l.Held != 2 {
		t.Fatalf("an extended hold was swept: %+v", l)
	}
}
