package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Scenario "Position moves": after 2,000 are admitted ahead, the fan's number ahead drops by 2,000
// and the estimate is recalculated from the rate.
func TestPositionMoves(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.RatePerMinute = 2000
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	longLine(t, env, s, 3000) // 3,000 already in line (the join path is covered by join_test)
	me := onsaletest.NewFan()
	before := env.Join(t, s, me)
	if before.Ahead == nil || *before.Ahead != 3000 || *before.EstimateSeconds != 90 {
		t.Fatalf("before %+v", before)
	}
	// One minute of admissions at 2,000 a minute (ticks of 500 ms).
	admitted := int64(0)
	if _, err := env.Engine.Admit(ctx, cur); err != nil { // starts the bucket
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		env.Clock.Add(queue.Tick)
		r, err := env.Engine.Admit(ctx, cur)
		if err != nil {
			t.Fatal(err)
		}
		admitted += r.Admitted
	}
	if admitted != 2000 {
		t.Fatalf("admitted %d in a minute, want 2,000", admitted)
	}
	after := env.Status(t, s, me)
	if after.State != queue.PlaceWaiting || *after.Ahead != 1000 || *after.EstimateSeconds != 30 {
		t.Fatalf("after %+v, want 1,000 ahead and 30 s", after)
	}
	if after.NextPollMs < 1000 || after.NextPollMs > 13000 {
		t.Fatalf("next poll in %d ms", after.NextPollMs)
	}
	_ = time.Second
}
