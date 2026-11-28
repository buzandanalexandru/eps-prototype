package queue_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Scenario "Uniform shuffle": 10,000 pre-queue members drawn 200 times: arrival order and position
// are uncorrelated and every position is equally likely within statistical tolerance.
func TestUniformShuffle(t *testing.T) {
	const n, runs, bins = 10000, 200, 10
	members := make([]string, n)
	arrival := map[string]int{}
	for i := range members {
		members[i] = fmt.Sprintf("m%05d", i)
		arrival[members[i]] = i
	}
	var counts [bins][bins]float64 // arrival decile x position decile
	maxCorr := 0.0
	for r := 0; r < runs; r++ {
		var seed [32]byte
		_, _ = rand.Read(seed[:])
		order := queue.Permute(members, seed)
		// Pearson correlation between arrival and position for this run.
		var sxy float64
		mean := float64(n-1) / 2
		var sxx float64
		for pos, m := range order {
			a := float64(arrival[m]) - mean
			p := float64(pos) - mean
			sxy += a * p
			sxx += a * a
			counts[arrival[m]*bins/n][pos*bins/n]++
		}
		maxCorr = math.Max(maxCorr, math.Abs(sxy/sxx))
	}
	if maxCorr > 0.05 {
		t.Fatalf("a run's arrival/position correlation reached %.4f", maxCorr)
	}
	// Chi-square over the 100 cells (81 degrees of freedom): the 99.9% point is about 128.
	expected := float64(n*runs) / (bins * bins)
	chi := 0.0
	for i := range counts {
		for j := range counts[i] {
			d := counts[i][j] - expected
			chi += d * d / expected
		}
	}
	if chi > 128 {
		t.Fatalf("chi-square %.1f over 81 degrees of freedom: positions are not uniform", chi)
	}
	// The same seed gives the same order (the audit can replay a draw).
	var seed [32]byte
	a, b := queue.Permute(members, seed), queue.Permute(members, seed)
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("a draw is not reproducible from its seed")
		}
	}
	t.Logf("max correlation %.4f, chi-square %.1f", maxCorr, chi)
}

// Scenario "Latecomer": a fan who joins 3 minutes after opening with 80,000 in line is 80,001st.
func TestLatecomerJoinsBehind(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	k := env.Keys(t, s)
	// 80,000 in the pre-queue (written directly: the join path is covered by join_test).
	for i := 0; i < 80000; i += 10000 {
		zs := make([]redis.Z, 0, 10000)
		for j := i; j < i+10000; j++ {
			zs = append(zs, redis.Z{Score: float64(j), Member: fmt.Sprintf("q%06d", j)})
		}
		if err := env.Redis.ZAdd(ctx, k.Pre(), zs...).Err(); err != nil {
			t.Fatal(err)
		}
	}
	env.Clock.Set(st.OpensAt.Add(time.Second))
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	start := time.Now()
	tail, err := env.Engine.Shuffle(ctx, cur)
	if err != nil || tail != 80000 {
		t.Fatalf("draw: tail %d, %v", tail, err)
	}
	t.Logf("80,000 drawn in %s", time.Since(start))
	env.Clock.Add(3 * time.Minute)
	got := env.Join(t, s, onsaletest.NewFan())
	if got.State != queue.PlaceWaiting || got.Ahead == nil || *got.Ahead != 80000 {
		t.Fatalf("latecomer %+v, want 80,000 ahead (position 80,001)", got)
	}
	if seed, _ := env.Redis.HGet(ctx, k.State(), "seed").Result(); len(seed) != 64 {
		t.Fatalf("seed %q not stored for the audit", seed)
	}
}

// Joins during the draw are kept and placed behind it.
func TestJoinDuringTheDrawIsKept(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	for i := 0; i < 5; i++ {
		env.Join(t, s, onsaletest.NewFan())
	}
	env.Clock.Set(st.OpensAt.Add(time.Second))
	late := onsaletest.NewFan()
	if got := env.Join(t, s, late); got.State != queue.PlaceShuffling {
		t.Fatalf("join at opening before the draw: %s", got.State)
	}
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	if tail, err := env.Engine.Shuffle(ctx, cur); err != nil || tail != 6 {
		t.Fatalf("tail %d (%v), want 6", tail, err)
	}
	if got := env.Status(t, s, late); got.State != queue.PlaceWaiting || got.Ahead == nil {
		t.Fatalf("after the draw %+v", got)
	}
}
