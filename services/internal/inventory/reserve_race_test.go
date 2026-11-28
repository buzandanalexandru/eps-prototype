package inventory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
)

// Scenario "Heavy contention": 20,000 concurrent holds of 1 to 4 tickets on a pool of 5,000 never
// take more than 5,000 and every shard stays at zero or above; committing half of them keeps the
// invariant.
func TestHeavyContentionNeverOversells(t *testing.T) {
	if testing.Short() {
		t.Skip("20,000 holds")
	}
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 5000}
	var ok, short, other atomic.Int64
	var taken atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, 200)
	start := time.Now()
	for i := 0; i < 20000; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			q := int64(i%4 + 1)
			id := fmt.Sprintf("race-%d", i)
			err := f.inv.Reserve(ctx, f.hold(id, map[string]int64{"arena": q}, time.Minute), caps)
			var s *checkout.ErrShort
			switch {
			case err == nil:
				ok.Add(1)
				taken.Add(q)
				if i%2 == 0 {
					if err := f.inv.Commit(ctx, f.org, f.event, id, caps); err != nil {
						other.Add(1)
						t.Logf("commit %s: %v", id, err)
					}
				}
			case errors.As(err, &s):
				short.Add(1)
			default:
				other.Add(1)
				t.Log(err)
			}
		}(i)
	}
	wg.Wait()
	t.Logf("20,000 holds in %s: %d held (%d units), %d short", time.Since(start), ok.Load(), taken.Load(), short.Load())
	if other.Load() != 0 {
		t.Fatalf("%d unexpected errors", other.Load())
	}
	if taken.Load() > 5000 {
		t.Fatalf("%d units taken from 5,000", taken.Load())
	}
	l := f.level(t, "arena")
	if l.Held+l.Sold != taken.Load() || l.Held+l.Sold+l.Free != 5000 {
		t.Fatalf("level %+v, taken %d", l, taken.Load())
	}
	// Only the last few units may be left (a 4-ticket order cannot fit in 3).
	if l.Free > 3*20 {
		t.Fatalf("%d units left unsold with 20,000 asking", l.Free)
	}
	for _, s := range f.shards(t, "arena") {
		if s.Used > s.Alloc || s.Used < 0 || s.Used != s.Sold+s.Held {
			t.Fatalf("shard %+v broke its invariant", s)
		}
	}
}
