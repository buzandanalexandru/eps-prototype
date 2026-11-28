package inventory_test

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Scenario "Held, not sold": a pool with 0 free and 40 held reads "all held", not sold out; once
// the holds are paid it is sold out.
func TestHeldIsNotSoldOut(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 40}
	for i := 0; i < 20; i++ {
		if err := f.inv.Reserve(ctx, f.hold(fmt.Sprintf("h%d", i), map[string]int64{"arena": 2}, time.Minute), caps); err != nil {
			t.Fatal(err)
		}
	}
	l := f.level(t, "arena")
	if l.Free != 0 || l.Held != 40 || l.SoldOut() || !l.AllHeld() {
		t.Fatalf("level %+v: want 0 free, 40 held, all held (not sold out)", l)
	}
	for i := 0; i < 20; i++ {
		if err := f.inv.Commit(ctx, f.org, f.event, fmt.Sprintf("h%d", i), caps); err != nil {
			t.Fatal(err)
		}
	}
	if l := f.level(t, "arena"); !l.SoldOut() || l.AllHeld() || l.Sold != 40 {
		t.Fatalf("level %+v: want sold out", l)
	}
}
