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

// CI regression (dbaba54): every commit of an instance queued on one spin lock, so under heavy
// contention some waited past the lock's limit and failed. Commits within the capacity now run
// side by side; resizes still exclude them (review corr L1), and only an instance whose capacity
// was lowered below what is taken makes commits check it one at a time.

// Commits running while other cores re-split the instance all succeed and keep every shard whole.
func TestCommitsAlongsideResizes(t *testing.T) {
	if testing.Short() {
		t.Skip("contention")
	}
	f := newFixture(t)
	ctx := context.Background()
	caps := map[string]int64{"arena": 4000}
	const n = 1500
	for i := 0; i < n; i++ {
		if err := f.inv.Reserve(ctx, f.hold(fmt.Sprintf("cr-%d", i), map[string]int64{"arena": 2}, time.Minute), caps); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	var bad atomic.Int64
	var current atomic.Int64
	current.Store(4000)
	stop := make(chan struct{})
	done := make(chan struct{})
	// An organiser keeps editing the capacity (above what is taken) while the sale runs, five times a
	// second: each edit re-splits the instance on the next call that sees it.
	go func() {
		defer close(done)
		for c := int64(1); ; c++ {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			current.Store(4000 + c%2)
		}
	}()
	sem := make(chan struct{}, 200)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			c := map[string]int64{"arena": current.Load()}
			if err := f.inv.Commit(ctx, f.org, f.event, fmt.Sprintf("cr-%d", i), c); err != nil {
				bad.Add(1)
				t.Logf("commit: %v", err)
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	<-done
	if bad.Load() != 0 {
		t.Fatalf("%d unexpected errors", bad.Load())
	}
	if l := f.level(t, "arena"); l.Sold != 2*n {
		t.Fatalf("level %+v, want %d sold", l, 2*n)
	}
	for _, s := range f.shards(t, "arena") {
		if s.Used > s.Alloc || s.Used < 0 || s.Used != s.Sold+s.Held {
			t.Fatalf("shard %+v broke its invariant", s)
		}
	}
}

// An instance whose capacity is lowered below what is held sells exactly up to the new capacity
// when hundreds of its holds commit at once; the rest are told it is short, and none fail otherwise.
func TestOverCapacityCommitsUnderContention(t *testing.T) {
	if testing.Short() {
		t.Skip("contention")
	}
	f := newFixture(t)
	ctx := context.Background()
	const n = 600
	for i := 0; i < n; i++ {
		if err := f.inv.Reserve(ctx, f.hold(fmt.Sprintf("ov-%d", i), map[string]int64{"arena": 1}, time.Minute), map[string]int64{"arena": 1000}); err != nil {
			t.Fatal(err)
		}
	}
	lowered := map[string]int64{"arena": 250}
	var ok, short, other atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := f.inv.Commit(ctx, f.org, f.event, fmt.Sprintf("ov-%d", i), lowered)
			var s *checkout.ErrShort
			switch {
			case err == nil:
				ok.Add(1)
			case errors.As(err, &s):
				short.Add(1)
			default:
				other.Add(1)
				t.Logf("commit: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if other.Load() != 0 {
		t.Fatalf("%d unexpected errors", other.Load())
	}
	if ok.Load() != 250 || short.Load() != n-250 {
		t.Fatalf("%d committed, %d short; want 250 and %d", ok.Load(), short.Load(), n-250)
	}
	if l := f.level(t, "arena"); l.Sold != 250 {
		t.Fatalf("level %+v, want 250 sold", l)
	}
}
