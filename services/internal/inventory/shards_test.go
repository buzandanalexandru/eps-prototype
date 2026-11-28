package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
)

// Scenario "Pool split": an Arena pool of 8,000 is spread over 32 counters adding up to 8,000.
func TestPoolSplit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.inv.Reserve(ctx, f.hold("h-split-1", map[string]int64{"arena": 1}, time.Minute), map[string]int64{"arena": 8000}); err != nil {
		t.Fatal(err)
	}
	sh := f.shards(t, "arena")
	if len(sh) != 32 {
		t.Fatalf("%d shards, want 32", len(sh))
	}
	var alloc, used int64
	for _, s := range sh {
		alloc += s.Alloc
		used += s.Used
		if s.Alloc < 249 || s.Alloc > 251 {
			t.Fatalf("shard %d holds %d, want about 250", s.Shard, s.Alloc)
		}
	}
	if alloc != 8000 || used != 1 {
		t.Fatalf("allocations add up to %d (used %d), want 8,000 (1)", alloc, used)
	}
}

func TestShardCountAndSplit(t *testing.T) {
	for cap, want := range map[int64]int{0: 1, 1: 1, 250: 1, 251: 2, 5000: 20, 8000: 32, 100000: 32} {
		if got := inventory.ShardCount(cap); got != want {
			t.Errorf("ShardCount(%d) = %d, want %d", cap, got, want)
		}
	}
	parts := inventory.Split(10, 3)
	if parts[0]+parts[1]+parts[2] != 10 || parts[0] != 4 || parts[2] != 3 {
		t.Fatalf("Split(10, 3) = %v", parts)
	}
}

// A capacity change re-splits the shards: each keeps what it took, free units are spread again.
func TestCapacityChangeResplits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i, id := range []string{"h-a", "h-b", "h-c"} {
		if err := f.inv.Reserve(ctx, f.hold(id, map[string]int64{"arena": int64(i + 1)}, time.Minute), map[string]int64{"arena": 300}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.inv.Reserve(ctx, f.hold("h-d", map[string]int64{"arena": 1}, time.Minute), map[string]int64{"arena": 1000}); err != nil {
		t.Fatal(err)
	}
	sh := f.shards(t, "arena")
	var alloc, used int64
	for _, s := range sh {
		alloc += s.Alloc
		used += s.Used
		if s.Used > s.Alloc {
			t.Fatalf("shard %d took %d above its %d", s.Shard, s.Used, s.Alloc)
		}
	}
	if len(sh) != 4 || alloc != 1000 || used != 7 {
		t.Fatalf("%d shards, alloc %d, used %d; want 4, 1000, 7", len(sh), alloc, used)
	}
	if l := f.level(t, "arena"); l.Free != 993 || l.Held != 7 {
		t.Fatalf("level %+v", l)
	}
}
