package botdefence_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Scenario "Moved back silently": a place scored 75 at position 300 after the draw is moved
// behind every other place; its page still shows a normal, slowly moving position.
func TestSuspectMovedBackSilently(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.BotDefence = true
	st.RatePerMinute = 600
	st.OpensAt = env.Clock.Now().Add(15 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	k := env.Keys(t, s)
	bot := onsaletest.NewFan()
	env.Join(t, s, bot)
	c, _ := env.Engine.Verifier().Verify(bot.Token, queue.KindQueue, env.Clock.Now(), false)
	for _, sig := range []botdefence.Signal{botdefence.NoProofOfWork, botdefence.Headless, botdefence.ManyPerIP} {
		_, _ = env.Scorer.Add(ctx, k.Bot(c.QID), sig)
	}
	if sc, _ := env.Scorer.Score(ctx, k.Bot(c.QID)); sc < 70 || sc >= 90 {
		t.Fatalf("score %d", sc)
	}
	// Scenario "Nothing visible before opening": the suspect's pre-queue page is like anyone's.
	fan := onsaletest.NewFan()
	env.Join(t, s, fan)
	a, b := env.Status(t, s, bot), env.Status(t, s, fan)
	if a.State != queue.PlacePrequeue || a.State != b.State || a.Ahead != nil || a.Paused != b.Paused || !a.OpensAt.Equal(b.OpensAt) {
		t.Fatalf("pre-queue pages differ: %+v vs %+v", a, b)
	}
	// 998 others in the pre-queue, then the draw.
	zs := []redis.Z{}
	for i := 0; i < 998; i++ {
		zs = append(zs, redis.Z{Score: float64(i), Member: fmt.Sprintf("p%04d", i)})
	}
	_ = env.Redis.ZAdd(ctx, k.Pre(), zs...).Err()
	env.Clock.Set(st.OpensAt.Add(time.Second))
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	// Put the suspect at position 300 of the draw to match the scenario.
	if _, err := env.Engine.Shuffle(ctx, cur); err != nil {
		t.Fatal(err)
	}
	if rank, _ := env.Redis.ZRank(ctx, k.Line(), c.QID).Result(); rank != 999 {
		t.Fatalf("after the draw and the penalty the suspect is at rank %d, want last (999)", rank)
	}
	// Moving it to position 300 by hand and penalising again shows the same.
	_ = env.Redis.ZAdd(ctx, k.Line(), redis.Z{Score: 300.5, Member: c.QID}).Err()
	if n, _ := env.Engine.Penalise(ctx, cur); n != 1 {
		t.Fatalf("penalised %d", n)
	}
	if rank, _ := env.Redis.ZRank(ctx, k.Line(), c.QID).Result(); rank != 999 {
		t.Fatalf("rank %d, want behind all 999 others", rank)
	}
	got := env.Status(t, s, bot)
	if got.State != queue.PlaceWaiting || got.Ahead == nil || *got.Ahead != 999 || got.EstimateSeconds == nil || got.Pass != "" {
		t.Fatalf("suspect's page %+v, want a normal waiting page", got)
	}
	// A fan who joins later still goes ahead of the suspect; the suspect is let in last.
	late := onsaletest.NewFan()
	if st := env.Join(t, s, late); *st.Ahead != 999 {
		t.Fatalf("latecomer ahead %d", *st.Ahead)
	}
	admitted := int64(0)
	_, _ = env.Engine.Admit(ctx, cur)
	for i := 0; i < 200 && admitted < 1000; i++ {
		env.Clock.Add(queue.Tick)
		r, _ := env.Engine.Admit(ctx, cur)
		admitted += r.Admitted
	}
	if ok, _ := env.Redis.HExists(ctx, k.Fan(c.QID), "adm").Result(); ok {
		t.Fatal("the suspect was let in before the last fan")
	}
}

// A score of 90 or more waits in the slow lane until the line is empty.
func TestSlowLane(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.BotDefence = true
	st.RatePerMinute = 120 // one a tick
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	k := env.Keys(t, s)
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	_, _ = env.Engine.Shuffle(ctx, cur)
	bot := onsaletest.NewFan()
	env.Join(t, s, bot)
	c, _ := env.Engine.Verifier().Verify(bot.Token, queue.KindQueue, env.Clock.Now(), false)
	for _, sig := range []botdefence.Signal{botdefence.NoProofOfWork, botdefence.Headless, botdefence.ManyPerSubnet} {
		_, _ = env.Scorer.Add(ctx, k.Bot(c.QID), sig)
	}
	fan := onsaletest.NewFan()
	env.Join(t, s, fan)
	_, _ = env.Engine.Admit(ctx, cur)
	env.Clock.Add(queue.Tick)
	r, _ := env.Engine.Admit(ctx, cur)
	if r.Slowed != 1 || r.Admitted != 1 {
		t.Fatalf("tick %+v", r)
	}
	if got := env.Status(t, s, bot); got.State != queue.PlaceWaiting {
		t.Fatalf("slow lane page %+v", got)
	}
	env.Clock.Add(queue.Tick)
	if r, _ := env.Engine.Admit(ctx, cur); r.Admitted != 1 {
		t.Fatalf("slow lane not served once the line was empty: %+v", r)
	}
}
