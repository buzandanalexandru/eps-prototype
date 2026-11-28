package queue_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

func longLine(t *testing.T, env *onsaletest.Env, s *sales.Sale, n int) {
	t.Helper()
	ctx := context.Background()
	k := env.Keys(t, s)
	for i := 0; i < n; i += 10000 {
		zs := []redis.Z{}
		for j := i; j < min(n, i+10000); j++ {
			zs = append(zs, redis.Z{Score: float64(j + 1), Member: fmt.Sprintf("q%06d", j)})
		}
		if err := env.Redis.ZAdd(ctx, k.Line(), zs...).Err(); err != nil {
			t.Fatal(err)
		}
	}
	_ = env.Redis.HSet(ctx, k.State(), "tail", n, "state", queue.StateOpen).Err()
}

func ticks(t *testing.T, env *onsaletest.Env, s *sales.Sale, n int) int64 {
	t.Helper()
	ctx := context.Background()
	total := int64(0)
	for i := 0; i < n; i++ {
		env.Clock.Add(queue.Tick)
		cur, err := env.Sales.Lookup(ctx, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		r, err := env.Engine.Admit(ctx, cur)
		if err != nil {
			t.Fatal(err)
		}
		total += r.Admitted
	}
	return total
}

// Scenario "Rate respected": at 1,200 a minute with a long line, 1,140 to 1,260 are admitted in
// each minute.
func TestRateRespected(t *testing.T) {
	env := onsaletest.Setup(t)
	st := env.Open()
	st.RatePerMinute = 1200
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	longLine(t, env, s, 10000)
	ticks(t, env, s, 1)
	for minute := 0; minute < 3; minute++ {
		n := ticks(t, env, s, 120)
		if n < 1140 || n > 1260 {
			t.Fatalf("minute %d admitted %d, want 1,140 to 1,260", minute, n)
		}
	}
}

// Scenario "Shopping limit": with 1,500 shopping and a limit of 1,500 no one else is admitted
// until a pass ends.
func TestShoppingLimit(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.RatePerMinute = 60000
	st.ActiveLimit = 1500
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	longLine(t, env, s, 5000)
	ticks(t, env, s, 1)
	if n := ticks(t, env, s, 20); n != 1500 {
		t.Fatalf("admitted %d, want 1,500", n)
	}
	if n := ticks(t, env, s, 10); n != 0 {
		t.Fatalf("admitted %d above the limit", n)
	}
	// A pass ends (its order was paid): one more comes in.
	if err := env.Engine.Ended(ctx, s.OrganiserID, s.EventID, s.ID, "q000000", "used"); err != nil {
		t.Fatal(err)
	}
	if n := ticks(t, env, s, 2); n != 1 {
		t.Fatalf("admitted %d after one pass ended, want 1", n)
	}
	// Passes end on their own after 10 minutes.
	env.Clock.Add(queue.PassLife)
	if n := ticks(t, env, s, 2); n == 0 {
		t.Fatal("no one admitted after the passes ran out")
	}
}

// Scenario "Pause": a paused sale admits nobody; waiting fans see paused; the rate change is live.
func TestPauseAndLiveRate(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.RatePerMinute = 1200
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	_, _ = env.Engine.Shuffle(ctx, cur)
	longLine(t, env, s, 0)
	me := onsaletest.NewFan()
	env.Join(t, s, me)
	longLine(t, env, s, 3000)
	st.Paused = true
	if _, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "test-user", st); err != nil {
		t.Fatal(err)
	}
	if n := ticks(t, env, s, 20); n != 0 {
		t.Fatalf("admitted %d while paused", n)
	}
	if got := env.Status(t, s, me); !got.Paused {
		t.Fatalf("status %+v does not say paused", got)
	}
	st.Paused = false
	st.RatePerMinute = 600
	if _, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "test-user", st); err != nil {
		t.Fatal(err)
	}
	ticks(t, env, s, 1)
	if n := ticks(t, env, s, 120); n < 570 || n > 630 {
		t.Fatalf("admitted %d in a minute at 600", n)
	}
	_ = time.Second
}
