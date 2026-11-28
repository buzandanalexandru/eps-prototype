package live_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/live"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func line(f *live.Frame, inst string) live.StockLine {
	for _, l := range f.Stock {
		if l.Instance == inst {
			return l
		}
	}
	return live.StockLine{}
}

// Scenario "Matches the store": a frame's left, held and sold equal the pool's shard counters and
// its sold total equals the paid orders' tickets; the queue's admitted total is the queue's own.
func TestFrameMatchesTheStore(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	buyer := simtest.NewBrowser(s.FanToken(t, "192.0.2.1"))
	buyer.Admit(t, s, sale.ID)
	if o := buyer.Buy(t, s, ev.Event, ev.Version, 2); o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("buy: %v", o["state"])
	}
	holder := simtest.NewBrowser(s.FanToken(t, "192.0.2.2"))
	holder.Admit(t, s, sale.ID)
	if code, out := holder.Hold(t, s, ev.Event, ev.Version, 3); code != 201 {
		t.Fatalf("hold: %d %v", code, out)
	}
	ctx := context.Background()
	f, err := s.Agg.Build(ctx, sale.ID, time.Now(), nil)
	if err != nil || f == nil {
		t.Fatalf("build: %v", err)
	}
	lv, err := (&inventory.Sharded{Redis: s.Redis}).Stock(ctx, ev.Org, ev.Event, []string{salestest.PoolArena})
	if err != nil {
		t.Fatal(err)
	}
	l := line(f, salestest.PoolArena)
	if want := lv[salestest.PoolArena]; l.Left != want.Free || l.Held != want.Held || l.Sold != want.Sold {
		t.Fatalf("frame %+v, shards %+v", l, want)
	}
	if l.Capacity != 100 || l.Sold != 2 || l.Held != 3 || l.Left != 95 || !f.StockOK {
		t.Fatalf("arena line: %+v (ready %v)", l, f.StockOK)
	}
	if f.Sales.Sold != 2 || f.Sales.PaidOrders != 1 || f.Sales.Capacity != 100+20+200 || f.Sales.RevenueMinor == 0 {
		t.Fatalf("sales: %+v", f.Sales)
	}
	c, _ := queue.CountsOf(ctx, s.Redis, ev.Org, ev.Event, sale.ID, time.Now())
	if f.Queue.AdmittedTotal != c.Admitted || c.Admitted != 2 || f.Queue.Rate != sale.Settings.RatePerMinute {
		t.Fatalf("queue: %+v vs %+v", f.Queue, c)
	}
	if f.Payments.Providers["a"].State != payments.Closed || f.Payments.SuccessRate == nil || *f.Payments.SuccessRate != 1 {
		t.Fatalf("payments: %+v", f.Payments)
	}
}

// Scenario "Provider A goes down": the frame shows A open, the timeline records the change and the
// failover counted since.
func TestBreakerAndFailoverInFrames(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	if f, _ := s.Agg.Build(ctx, sale.ID, time.Now(), nil); f.Payments.Providers["a"].State != payments.Closed {
		t.Fatal("A should start closed")
	}
	body, _ := json.Marshal(map[string]any{"outage": true, "outage_mode": "503"})
	if _, err := s.Knobs["a"].Put(ctx, ev.Org, body); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		s.Breaker.Record(ctx, ev.Org, "a", false)
	}
	b := simtest.NewBrowser(s.FanToken(t, "192.0.2.3"))
	b.Admit(t, s, sale.ID)
	if o := b.Buy(t, s, ev.Event, ev.Version, 1); o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("buy at B: %v", o["state"])
	}
	f, err := s.Agg.Build(ctx, sale.ID, time.Now().Add(time.Second), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Payments.Providers["a"].State != payments.Open {
		t.Fatalf("A: %s", f.Payments.Providers["a"].State)
	}
	var breaker bool
	for _, e := range f.Timeline {
		if e.Kind == "breaker" && e.Provider == "a" && e.To == payments.Open {
			breaker = true
		}
	}
	if !breaker {
		t.Fatalf("timeline: %+v", f.Timeline)
	}
	// The breaker was open before the payment, so B took it at once (no failover from A); force one
	// failover row to see it counted: close A, let the next payment fail over.
	s.Breaker.Reset(ctx, ev.Org, "a")
	b2 := simtest.NewBrowser(s.FanToken(t, "192.0.2.4"))
	b2.Admit(t, s, sale.ID)
	if o := b2.Buy(t, s, ev.Event, ev.Version, 1); o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("buy with failover: %v", o["state"])
	}
	g, _ := s.Agg.Build(ctx, sale.ID, time.Now().Add(2*time.Second), nil)
	if g.Payments.FailoversTotal < 1 {
		t.Fatalf("failovers: %d", g.Payments.FailoversTotal)
	}
	var failover bool
	for _, e := range g.Timeline {
		if e.Kind == "failover" && e.N >= 1 {
			failover = true
		}
	}
	if !failover || g.Payments.Providers["b"].Succeeded < 1 {
		t.Fatalf("failover not in the frame: %+v %+v", g.Timeline, g.Payments)
	}
}

// Scenario "Bot panel": the system's flagged count and the run's truth in one frame.
func TestBotPanelWithTruth(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	k, _ := queue.KeysOf(ev.Org, ev.Event, sale.ID)
	s.Redis.HSet(ctx, k.State(), "sus", 7, "prj", 3)
	rep, _ := json.Marshal(map[string]any{"runId": uuid.NewString(), "state": "running", "truth": map[string]any{"botsSent": 10000, "botsAdmitted": 12, "botTickets": 30, "fansAskedOtp": 4}})
	if err := live.StoreReport(ctx, s.Redis, sale.ID, rep); err != nil {
		t.Fatal(err)
	}
	f, err := s.Agg.Build(ctx, sale.ID, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Bots.Flagged != 7 || f.Bots.PassesRejected != 3 {
		t.Fatalf("bots: %+v", f.Bots)
	}
	var run struct {
		Truth struct {
			BotsSent, BotsAdmitted, BotTickets, FansAskedOtp int
		} `json:"truth"`
	}
	if err := json.Unmarshal(f.Run, &run); err != nil || run.Truth.BotsSent != 10000 || run.Truth.BotsAdmitted != 12 || run.Truth.BotTickets != 30 || run.Truth.FansAskedOtp != 4 {
		t.Fatalf("truth: %s", f.Run)
	}
}

// Scenario "Suggestion only": core p95 over 2 s in each of the last 30 frames suggests degraded
// mode; nothing switches it on.
func TestDegradeSuggestion(t *testing.T) {
	s := simtest.Start(t)
	_, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	slow := 2500.0
	sys := &live.System{CoreP95Ms: &slow, Pods: map[string]int{}}
	now := time.Now()
	for i := 1; i <= 30; i++ {
		f, err := s.Agg.Build(ctx, sale.ID, now.Add(time.Duration(i)*time.Second), sys)
		if err != nil {
			t.Fatal(err)
		}
		if want := i >= live.SuggestFrames; f.Suggest != want {
			t.Fatalf("frame %d: suggest %v", i, f.Suggest)
		}
	}
	cur, _ := s.Sales.Lookup(ctx, sale.ID)
	if cur.Degraded {
		t.Fatal("the switch was flipped")
	}
	fast := 100.0
	if f, _ := s.Agg.Build(ctx, sale.ID, now.Add(31*time.Second), &live.System{CoreP95Ms: &fast}); f.Suggest {
		t.Fatal("suggestion should reset when latency is back")
	}
}

// Rates are the change of the totals over the last minute.
func TestRatesFromTotals(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	k, _ := queue.KeysOf(ev.Org, ev.Event, sale.ID)
	now := time.Now()
	s.Redis.HSet(ctx, k.State(), "admc", 100)
	if _, err := s.Agg.Build(ctx, sale.ID, now, nil); err != nil {
		t.Fatal(err)
	}
	s.Redis.HSet(ctx, k.State(), "admc", 400)
	f, _ := s.Agg.Build(ctx, sale.ID, now.Add(30*time.Second), nil)
	if f.Queue.AdmittedPerMin != 600 {
		t.Fatalf("admitted per minute: %v (300 in 30 s)", f.Queue.AdmittedPerMin)
	}
}
