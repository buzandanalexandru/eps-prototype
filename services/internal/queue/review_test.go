package queue_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Review D-b (corr H2, the reviewer's repro): a join from a gateway whose clock is a little behind,
// arriving after the draw, still enters the open line (Redis state decides), and is let in.
func TestJoinAfterTheDrawFromALaggingClock(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	early := onsaletest.NewFan()
	env.Join(t, s, early)
	env.Clock.Set(st.OpensAt.Add(time.Second))
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	if _, err := env.Engine.Shuffle(ctx, cur); err != nil {
		t.Fatal(err)
	}
	lag := *env.Engine
	lag.Now = func() time.Time { return env.Clock.Now().Add(-2 * time.Second) }
	late := onsaletest.NewFan()
	late.IP = "192.0.2.99"
	got, err := lag.Join(ctx, s.ID, queue.JoinInput{Sub: late.Buyer.ID, Kind: late.Buyer.Kind, Session: late.Session,
		Pow: queue.SolvePow(s.ID, queue.SessionHash(late.Session)), UA: "Mozilla/5.0 Safari", IP: late.IP})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != queue.PlaceWaiting {
		t.Fatalf("the lagging join is %q, want in the line", got.State)
	}
	late.Token = got.QueueToken
	for i := 0; i < 10; i++ {
		env.Clock.Add(queue.Tick)
		_, _ = env.Engine.Admit(ctx, cur)
	}
	if st := env.Status(t, s, late); st.State != queue.PlaceAdmitted {
		t.Fatalf("latecomer %q after admission with room for everyone", st.State)
	}
}

// Review D-b: a fan who arrives at or after opening but before the draw ran is a latecomer: behind
// everyone who was in the pre-queue, in arrival order.
func TestArrivalAfterOpeningGoesBehindTheDraw(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	for i := 0; i < 20; i++ {
		env.Join(t, s, onsaletest.NewFan())
	}
	env.Clock.Set(st.OpensAt.Add(time.Second))
	first, second := onsaletest.NewFan(), onsaletest.NewFan()
	env.Join(t, s, first)
	env.Clock.Add(time.Second)
	env.Join(t, s, second)
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	if tail, err := env.Engine.Shuffle(ctx, cur); err != nil || tail != 22 {
		t.Fatalf("tail %d (%v)", tail, err)
	}
	a, b := env.Status(t, s, first), env.Status(t, s, second)
	if *a.Ahead != 20 || *b.Ahead != 21 {
		t.Fatalf("latecomers %d and %d ahead, want 20 and 21", *a.Ahead, *b.Ahead)
	}
}

// Review D-c (corr M2, the reviewer's repro): a draw cut short after its first batch is continued
// by the next run from the same snapshot and seed: every position once, the tail equal to the line.
func TestDrawResumesAfterACrash(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	k := env.Keys(t, s)
	zs := []redis.Z{}
	for i := 0; i < 12000; i++ {
		zs = append(zs, redis.Z{Score: float64(i), Member: fmt.Sprintf("q%05d", i)})
	}
	_ = env.Redis.ZAdd(ctx, k.Pre(), zs...).Err()
	env.Clock.Set(st.OpensAt.Add(time.Second))
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	undo := queue.CutDrawAfter(5000, errors.New("crash"))
	if _, err := env.Engine.Shuffle(ctx, cur); err == nil {
		t.Fatal("the cut draw finished")
	}
	undo()
	seed1, _ := env.Redis.HGet(ctx, k.State(), "seed").Result()
	// A fan joins while the draw is half done: a latecomer.
	mid := onsaletest.NewFan()
	env.Join(t, s, mid)
	tail, err := env.Engine.Shuffle(ctx, cur)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := env.Redis.ZCard(ctx, k.Line()).Result()
	for _, pos := range []string{"1", "5000", "5001", "12000"} {
		if c, _ := env.Redis.ZCount(ctx, k.Line(), pos, pos).Result(); c != 1 {
			t.Fatalf("%d places at position %s", c, pos)
		}
	}
	seed2, _ := env.Redis.HGet(ctx, k.State(), "seed").Result()
	if tail != 12001 || n != 12001 || seed1 != seed2 || len(seed1) != 64 {
		t.Fatalf("tail %d, line %d, seeds %q / %q", tail, n, seed1, seed2)
	}
	if got := env.Status(t, s, mid); *got.Ahead != 12000 {
		t.Fatalf("the fan who joined during the draw is %d ahead, want 12,000", *got.Ahead)
	}
}

// Review D-d (corr M3, the reviewer's repro): one account, two devices, one place; each device is
// handed a pass for its own session and either can buy; neither is scored for it.
func TestSecondDeviceUsesThePlace(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second)
	st.BotDefence = true
	sale := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, sale, st)
	phone := onsaletest.NewFan()
	env.Join(t, s, phone)
	laptop := &onsaletest.Fan{Buyer: phone.Buyer, Session: phone.Session + "-laptop", IP: "192.0.2.77"}
	env.Join(t, s, laptop)
	env.Clock.Set(st.OpensAt.Add(time.Second))
	env.Admitted(t, s, phone)
	ls := env.Status(t, s, laptop)
	if ls.State != queue.PlaceAdmitted || ls.Pass == "" || ls.Pass == phone.Pass {
		t.Fatalf("laptop %+v", ls)
	}
	b := laptop.Buyer
	b.Pass, b.Session = ls.Pass, laptop.Session
	if _, _, err := env.Svc.CreateOrder(ctx, b, sale.Request(salestestGA(1)), ""); err != nil {
		t.Fatalf("the laptop, shown 'admitted', cannot buy: %v", err)
	}
	// The phone tried the laptop's pass from its own session: a person's own browsers are not scored.
	wrong := phone.Buyer
	wrong.Pass = ls.Pass
	_, _, _ = env.Svc.CreateOrder(ctx, wrong, sale.Request(salestestGA(1)), "")
	c, _ := env.Engine.Verifier().Verify(ls.Pass, queue.KindPass, env.Clock.Now(), false)
	if sc, _ := env.Scorer.Score(ctx, env.Keys(t, s).Bot(c.QID)); sc != 0 {
		t.Fatalf("the place scored %d for its own second device", sc)
	}
}

// Review spec M3: an admission whose pass nobody collects within the fetch window frees its
// shopping place for the next fan.
func TestUncollectedPassFreesThePlace(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.ActiveLimit = 1
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	_, _ = env.Engine.Shuffle(ctx, cur)
	gone, next := onsaletest.NewFan(), onsaletest.NewFan()
	env.Join(t, s, gone)
	env.Join(t, s, next)
	_, _ = env.Engine.Admit(ctx, cur)
	env.Clock.Add(queue.Tick)
	if r, _ := env.Engine.Admit(ctx, cur); r.Active != 1 {
		t.Fatalf("active %d", r.Active)
	}
	env.Clock.Add(queue.FetchWindow + time.Second)
	if r, _ := env.Engine.Admit(ctx, cur); r.Admitted != 1 {
		t.Fatalf("the next fan was not let in after the uncollected pass: %+v", r)
	}
	if st := env.Status(t, s, gone); st.State != queue.PlaceEnded {
		t.Fatalf("the uncollected place is %q", st.State)
	}
	if st := env.Status(t, s, next); st.State != queue.PlaceAdmitted {
		t.Fatalf("next %q", st.State)
	}
}

// Review D-j: the estimate follows the admissions actually made in the last minute (here the
// shopping limit holds them back), and is hidden while paused.
func TestEstimateFromMeasuredAdmissions(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.RatePerMinute, st.ActiveLimit = 6000, 100 // the dial says 6,000, the limit lets 100 in
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	longLine(t, env, s, 2000)
	me := onsaletest.NewFan()
	env.Join(t, s, me)
	cur, _ := env.Sales.Lookup(ctx, s.ID)
	_, _ = env.Engine.Admit(ctx, cur)
	for i := 0; i < 120; i++ {
		env.Clock.Add(queue.Tick)
		_, _ = env.Engine.Admit(ctx, cur)
	}
	got := env.Status(t, s, me)
	if got.EstimateSeconds == nil || *got.EstimateSeconds < 600 {
		t.Fatalf("estimate %v for %d ahead with 100 admitted a minute (the dial alone would say 19 s)", got.EstimateSeconds, *got.Ahead)
	}
	st.Paused = true
	if _, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "u", st); err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(1100 * time.Millisecond)
	if got := env.Status(t, s, me); got.EstimateSeconds != nil || !got.Paused {
		t.Fatalf("paused page %+v", got)
	}
}

// Review sec L2: a rejoin of a live place does not count against the join limit.
func TestRejoinDoesNotCountAgainstTheLimit(t *testing.T) {
	env := onsaletest.Setup(t)
	env.Engine.JoinsPerMinute = 2
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), env.Open())
	f := onsaletest.NewFan()
	for i := 0; i < 5; i++ {
		env.Join(t, s, f)
	}
	env.Join(t, s, onsaletest.NewFan())
}

// Review D-f: a sale for signed-in fans asks guests to sign in; guest places from one address are
// limited per minute.
func TestAccountsOnlyAndGuestLimit(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.AccountsOnly = true
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	g := onsaletest.NewFan()
	_, err := env.Engine.Join(ctx, s.ID, queue.JoinInput{Sub: "guest:" + g.Buyer.ID, Kind: "guest", Session: g.Session, IP: g.IP})
	if code(err) != "EPS-SIGN-IN-REQUIRED" {
		t.Fatalf("a guest joined a signed-in-only sale: %v", err)
	}
	env.Join(t, s, onsaletest.NewFan()) // an account joins
	open := env.NewSale(t, env.Env.NewSale(t, 100, 100), env.Open())
	env.Engine.NewGuestsPerMinute = 3
	for i := 0; i < 4; i++ {
		f := onsaletest.NewFan()
		_, err := env.Engine.Join(ctx, open.ID, queue.JoinInput{Sub: "guest:" + f.Buyer.ID, Kind: "guest", Session: f.Session, IP: "192.0.2.200"})
		if i < 3 && err != nil {
			t.Fatal(err)
		}
		if i == 3 && !errors.Is(err, queue.ErrRateLimited) {
			t.Fatalf("a fourth new guest from one address: %v", err)
		}
	}
}
