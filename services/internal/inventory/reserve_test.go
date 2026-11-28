package inventory_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
)

// Scenario "Order larger than one shard": a hold of 4 arrives and its first shard has 1 left; it
// takes from more than one shard and succeeds.
func TestOrderLargerThanOneShard(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 1000} // 4 shards of 250
	first := inventory.StartShard("fill", 4)
	if err := f.inv.Reserve(ctx, f.hold("fill", map[string]int64{"arena": 249}, time.Minute), caps); err != nil {
		t.Fatal(err)
	}
	if s := f.shards(t, "arena")[first]; s.Alloc-s.Used != 1 {
		t.Fatalf("shard %d has %d free, want 1", first, s.Alloc-s.Used)
	}
	id := ""
	for k := 0; id == ""; k++ {
		if c := fmt.Sprintf("big-%d", k); inventory.StartShard(c, 4) == first {
			id = c
		}
	}
	if err := f.inv.Reserve(ctx, f.hold(id, map[string]int64{"arena": 4}, time.Minute), caps); err != nil {
		t.Fatalf("a hold of 4 across shards: %v", err)
	}
	touched := 0
	for _, s := range f.shards(t, "arena") {
		if s.Held > 0 && s.Shard != first {
			touched++
		}
	}
	if touched < 1 || f.shards(t, "arena")[first].Alloc != f.shards(t, "arena")[first].Used {
		t.Fatalf("the hold of 4 did not spill over: %+v", f.shards(t, "arena"))
	}
	if l := f.level(t, "arena"); l.Held != 253 || l.Free != 747 {
		t.Fatalf("level %+v", l)
	}
}

// Scenario "Second pool short": stock taken from the first pool is returned and nothing is held.
func TestSecondPoolShort(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 1000, "plus": 1}
	err := f.inv.Reserve(ctx, f.hold("two-pools", map[string]int64{"arena": 2, "plus": 2}, time.Minute), caps)
	var short *checkout.ErrShort
	if !errors.As(err, &short) || short.Instance != "plus" {
		t.Fatalf("want short on plus, got %v", err)
	}
	if a, p := f.level(t, "arena"), f.level(t, "plus"); a.Held != 0 || p.Held != 0 || a.Free != 1000 || p.Free != 1 {
		t.Fatalf("stock taken: arena %+v plus %+v", a, p)
	}
	if st, _ := f.inv.HoldState(ctx, f.org, f.event, "two-pools"); st != "" {
		t.Fatalf("hold record left in state %q", st)
	}
	// The same id can be used again once there is room.
	caps["plus"] = 10
	if err := f.inv.Reserve(ctx, f.hold("two-pools", map[string]int64{"arena": 2, "plus": 2}, time.Minute), caps); err != nil {
		t.Fatal(err)
	}
}

// A retried reserve with the same hold id takes nothing more.
func TestReserveIsIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 100}
	h := f.hold("again", map[string]int64{"arena": 3}, time.Minute)
	for i := 0; i < 3; i++ {
		if err := f.inv.Reserve(ctx, h, caps); err != nil {
			t.Fatal(err)
		}
	}
	if l := f.level(t, "arena"); l.Held != 3 {
		t.Fatalf("held %d, want 3", l.Held)
	}
}
