package simulation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
)

// runToEnd starts a run with the simulator and waits until it ends.
func runToEnd(t *testing.T, s *simtest.Stack, org, saleID string, k simulation.Knobs) (*simulation.Run, simulation.Summary) {
	t.Helper()
	ctx := s.Context(t)
	run, err := s.Sim.Start(ctx, org, saleID, "test", k)
	if err != nil {
		t.Fatal(err)
	}
	s.RunSimulator(ctx)
	r := waitState(t, s, org, run.ID, 4*time.Minute, simulation.Done, simulation.Failed, simulation.Stopped)
	if r.State != simulation.Done {
		t.Fatalf("run ended %s: %v", r.State, r.Error)
	}
	t.Logf("run %s ended after %s; last report %s", r.ID, r.EndedAt.Sub(*r.OpensAt).Round(time.Second), r.Report)
	var sum simulation.Summary
	if err := json.Unmarshal(r.Summary, &sum); err != nil {
		t.Fatal(err)
	}
	return r, sum
}

// Scenarios "Fan journey" and "No shortcuts": 200 simulated fans through the real waiting room,
// checkout and providers; every paid order has a succeeded attempt at a simulated provider and the
// queue admitted at least as many fans as orders were paid.
func TestFanJourney(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 2000, 20)
	ctx := context.Background()
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	r, sum := runToEnd(t, s, ev.Org, rh.ID, simulation.Knobs{Fans: 200, BotShare: f(0), OpensInSeconds: 30, PollSeconds: 5, DurationSeconds: 60,
		DeclineRate: f(0), ProviderA: &simulation.ProviderPlan{Mode: "none"}})
	paid := count(t, s, `select count(*) from sales.orders where event_id = $1 and state in ('paid', 'issued')`, r.EventID)
	if paid < 150 {
		t.Fatalf("only %d of 200 fans paid (summary %+v)", paid, sum.Truth)
	}
	if bad := count(t, s, `select count(*) from sales.orders o where o.event_id = $1 and o.state in ('paid', 'issued') and not exists (
		select 1 from sales.payment_attempts a where a.order_id = o.id and a.state = 'succeeded' and a.provider in ('a', 'b'))`, r.EventID); bad != 0 {
		t.Fatalf("%d paid orders without a succeeded attempt", bad)
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1 and (not rehearsal or sim_run_id is distinct from $2)`, r.EventID, r.ID); n != 0 {
		t.Fatalf("%d orders not marked with the run", n)
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1 and pass_qid is not null and state in ('paid', 'issued')`, r.EventID); n != paid {
		t.Fatalf("paid orders without a pass: %d of %d", paid-n, paid)
	}
	c, err := queue.CountsOf(ctx, s.Redis, ev.Org, r.EventID, r.SaleID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.Admitted < int64(paid) || sum.Truth.FansAdmitted < paid {
		t.Fatalf("admitted %d (truth %d) < paid %d", c.Admitted, sum.Truth.FansAdmitted, paid)
	}
	// The payment mix: cards on the hosted page, Apple Pay and Google Pay.
	for _, m := range []string{"hosted_card", "apple_pay", "google_pay"} {
		if n := count(t, s, `select count(*) from sales.payment_attempts a join sales.orders o on o.id = a.order_id where o.event_id = $1 and a.method = $2 and a.state = 'succeeded'`, r.EventID, m); n == 0 {
			t.Fatalf("no succeeded %s payment", m)
		}
	}
	// Every fan who pressed Pay paid (no declines, no outage): the summary's rate is per order.
	if sum.PaymentSuccessRate != 1 {
		t.Fatalf("payment success rate %v", sum.PaymentSuccessRate)
	}
	// Review D-f: planned and arrived are both counted; the peak second is at least the average;
	// 2,000 places with 200 fans do not sell out, so there is no sell-out time.
	if sum.Truth.FansPlanned != 200 || sum.PeakRPS < sum.RPS || sum.SellOutSeconds != nil {
		t.Fatalf("summary: planned %d peak %v rps %v sell-out %v", sum.Truth.FansPlanned, sum.PeakRPS, sum.RPS, sum.SellOutSeconds)
	}
	if sum.Facts.PaidOrders != paid || sum.Facts.TicketsSold != sum.Truth.FanTickets || sum.Truth.FansSent != 200 || sum.RPS <= 0 {
		t.Fatalf("summary: facts %+v truth %+v rps %v", sum.Facts, sum.Truth, sum.RPS)
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1`, ev.Event); n != 0 {
		t.Fatalf("real event orders: %d", n)
	}
	// Review D-c: the rehearsal closes after its run.
	closed, _, err := s.Sales.Get(ctx, ev.Org, r.SaleID)
	if err != nil {
		t.Fatal(err)
	}
	if !closed.Settings.Paused || closed.Settings.OpensAt.Before(time.Now().Add(24*time.Hour)) {
		t.Fatalf("rehearsal still open after its run: %+v", closed.Settings)
	}
}

// Scenario "Outage on schedule" (with fans): while A is down for the rehearsal, new payments
// complete at B; the summary counts them, and a sale that sold its whole capacity has a sell-out
// time taken from its last paid order.
func TestOutageRunPaysAtB(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 20, 10)
	ctx := context.Background()
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	r, sum := runToEnd(t, s, ev.Org, rh.ID, simulation.Knobs{Fans: 80, BotShare: f(0), OpensInSeconds: 30, PollSeconds: 5, DurationSeconds: 60,
		DeclineRate: f(0), ProviderA: &simulation.ProviderPlan{Mode: "outage", AtSeconds: 0, ForSeconds: 600}})
	paid := count(t, s, `select count(*) from sales.orders where event_id = $1 and state in ('paid', 'issued')`, r.EventID)
	if paid == 0 {
		t.Fatal("no fan paid during the outage")
	}
	atB := count(t, s, `select count(*) from sales.payment_attempts a join sales.orders o on o.id = a.order_id
		where o.event_id = $1 and a.provider = 'b' and a.state = 'succeeded'`, r.EventID)
	atA := count(t, s, `select count(*) from sales.payment_attempts a join sales.orders o on o.id = a.order_id
		where o.event_id = $1 and a.provider = 'a' and a.state = 'succeeded'`, r.EventID)
	if atA != 0 || atB != paid || sum.Facts.PaidAtBDuringOutage != paid {
		t.Fatalf("paid %d: at A %d, at B %d, summary %d", paid, atA, atB, sum.Facts.PaidAtBDuringOutage)
	}
	// 30 places: sold out exactly when every place was paid for.
	sold := count(t, s, `select coalesce(sum(d.value::bigint), 0) from sales.orders o, jsonb_each_text(o.draws) d
		where o.event_id = $1 and o.state in ('paid', 'issued')`, r.EventID)
	if (sold == 30) != (sum.SellOutSeconds != nil) {
		t.Fatalf("sold %d of 30, sell-out %v", sold, sum.SellOutSeconds)
	}
}
