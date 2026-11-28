package simulation_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
)

func problem(t *testing.T, err error) *apperr.Problem {
	t.Helper()
	var p *apperr.Problem
	if !errors.As(err, &p) {
		t.Fatalf("want a problem, got %v", err)
	}
	return p
}

func f(v float64) *float64 { return &v }

// Scenario "Token on a real sale": a simulator token is refused on any sale but its rehearsal, for
// joining, holding and every operation outside the checkout.
func TestSimTokenOnRealSale(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	rh := openRehearsal(t, s, real)
	run := uuid.NewString()
	// Join the real sale with a token naming the rehearsal: refused.
	b := newBrowser(s.SimToken(t, rh.EventID, run, "86.100.0.1"))
	if code, out := b.join(t, s, real.ID); code != http.StatusForbidden || out["code"] != apperr.SimRefused {
		t.Fatalf("join real sale: %d %v", code, out)
	}
	// Hold on the real event: refused before any stock or cap is touched.
	if code, out := b.hold(t, s, ev.Event, ev.Version, 1); code != http.StatusForbidden || out["code"] != apperr.SimRefused {
		t.Fatalf("hold real event: %d %v", code, out)
	}
	// A token naming another rehearsal is refused on this one.
	other := newBrowser(s.SimToken(t, uuid.NewString(), run, "86.100.0.2"))
	if code, _ := other.join(t, s, rh.ID); code != http.StatusForbidden {
		t.Fatalf("join with another rehearsal's token: %d", code)
	}
	// Operations outside the checkout are refused for simulator tokens.
	for _, u := range []string{"/v1/shop/wallets", "/v1/shop/saved-cards", "/v1/organiser/payments/providers"} {
		if code, _ := call(t, http.MethodGet, s.Core.URL+u, map[string]string{"Authorization": "Bearer " + b.token}, nil); code != http.StatusForbidden {
			t.Fatalf("%s: %d", u, code)
		}
	}
	// Its own rehearsal works.
	b.admit(t, s, rh.ID)
	if code, out := b.hold(t, s, rh.EventID, 1, 1); code != http.StatusCreated {
		t.Fatalf("hold rehearsal: %d %v", code, out)
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1`, ev.Event); n != 0 {
		t.Fatalf("real event orders: %d", n)
	}
}

// Scenario "Out of range" and "Real sale refused".
func TestKnobsAndRealSaleRefused(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	ctx := context.Background()
	rh, err := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		k     simulation.Knobs
		field string
	}{
		{simulation.Knobs{Fans: 5000}, "fans"},
		{simulation.Knobs{Fans: 5}, "fans"},
		{simulation.Knobs{DurationSeconds: 900}, "durationSeconds"},
		{simulation.Knobs{BotShare: f(0.9)}, "botShare"},
		{simulation.Knobs{PollSeconds: 1}, "pollSeconds"},
		{simulation.Knobs{PaymentMix: map[string]float64{"wallet": 10}}, "paymentMix"},
		{simulation.Knobs{ProviderA: &simulation.ProviderPlan{Mode: "outage", AtSeconds: 10, ForSeconds: 5}}, "providerA.forSeconds"},
	} {
		_, err := s.Sim.Start(ctx, ev.Org, rh.ID, "test", c.k)
		p := problem(t, err)
		if p.Code != apperr.SimInvalid || p.Data["field"] != c.field {
			t.Fatalf("%+v: %s %v", c.k, p.Code, p.Data)
		}
	}
	p := problem(t, func() error { _, err := s.Sim.Start(ctx, ev.Org, real.ID, "test", simulation.Knobs{}); return err }())
	if p.Code != apperr.SimInvalid || p.Data["field"] != "sale" {
		t.Fatalf("real sale: %s %v", p.Code, p.Data)
	}
	if n := count(t, s, `select count(*) from sales.simulation_runs`); n != 0 {
		t.Fatalf("runs written: %d", n)
	}
	// Defaults: 1,000 fans, 10% bots.
	run, err := s.Sim.Start(ctx, ev.Org, rh.ID, "test", simulation.Knobs{})
	if err != nil {
		t.Fatal(err)
	}
	if run.State != simulation.Pending || run.Knobs.Fans != 1000 || *run.Knobs.BotShare != 0.10 {
		t.Fatalf("default run: %+v", run)
	}
}

// Scenario "Second run": one active run per organiser (naming it), and per environment.
func TestSecondRun(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	ctx := context.Background()
	rh1, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	rh2, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	first, err := s.Sim.Start(ctx, ev.Org, rh1.ID, "test", simulation.Knobs{Fans: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Sim.Start(ctx, ev.Org, rh2.ID, "test", simulation.Knobs{Fans: 100})
	if p := problem(t, err); p.Code != apperr.SimActive || p.Data["runId"] != first.ID {
		t.Fatalf("second run: %s %v", p.Code, p.Data)
	}
	// Another organiser while a run is active anywhere: busy, naming nothing of the first.
	ev2, real2 := s.RealSale(t, 100, 20)
	rh3, _ := s.Sim.CreateRehearsal(ctx, ev2.Org, real2.ID, "test")
	_, err = s.Sim.Start(ctx, ev2.Org, rh3.ID, "test", simulation.Knobs{Fans: 100})
	if p := problem(t, err); p.Code != apperr.SimBusy || len(p.Data) != 0 {
		t.Fatalf("other organiser: %s %v", p.Code, p.Data)
	}
	// Organiser B cannot read organiser A's run.
	if _, err := s.Sim.Get(ctx, ev2.Org, first.ID); err == nil {
		t.Fatal("organiser B read A's run")
	}
	// A stopped pending run frees the slot.
	if r, err := s.Sim.Stop(ctx, ev.Org, first.ID); err != nil || r.State != simulation.Stopped {
		t.Fatalf("stop pending: %v %+v", err, r)
	}
	if _, err := s.Sim.Start(ctx, ev2.Org, rh3.ID, "test", simulation.Knobs{Fans: 100}); err != nil {
		t.Fatal(err)
	}
}

func waitState(t *testing.T, s *simtest.Stack, org, id string, within time.Duration, want ...string) *simulation.Run {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		r, err := s.Sim.Get(context.Background(), org, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if r.State == w {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s is %s, want %v", id, r.State, want)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Scenario "Stop a run": within 15 seconds no simulated traffic reaches the gateway and the run is
// stopped (and A's settings are restored).
func TestStopRun(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 1000, 20)
	ctx := s.Context(t)
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	run, err := s.Sim.Start(ctx, ev.Org, rh.ID, "test", simulation.Knobs{Fans: 200, OpensInSeconds: 30, ProviderA: &simulation.ProviderPlan{Mode: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	s.RunSimulator(ctx)
	waitState(t, s, ev.Org, run.ID, 20*time.Second, simulation.Running)
	time.Sleep(2 * time.Second) // fans are joining the pre-queue
	if s.GatewayHits.Load() == 0 {
		t.Fatal("no simulated traffic")
	}
	stopAt := time.Now()
	if _, err := s.Sim.Stop(ctx, ev.Org, run.ID); err != nil {
		t.Fatal(err)
	}
	r := waitState(t, s, ev.Org, run.ID, 15*time.Second, simulation.Stopped)
	if time.Since(stopAt) > 15*time.Second || r.EndedAt == nil || r.Summary == nil {
		t.Fatalf("stop took %s, summary %v", time.Since(stopAt), r.Summary != nil)
	}
	hits := s.GatewayHits.Load()
	time.Sleep(3 * time.Second)
	if after := s.GatewayHits.Load(); after != hits {
		t.Fatalf("traffic after stop: %d more requests", after-hits)
	}
}

func knobsOf(t *testing.T, s *simtest.Stack, provider, org, event string) map[string]any {
	t.Helper()
	raw, err := s.Knobs[provider].Get(context.Background(), payments.MerchantOf(payments.Scope(org, event)))
	if err != nil {
		t.Fatal(err)
	}
	var k map[string]any
	_ = json.Unmarshal(raw, &k)
	return k
}

// Scenario "Simulator lost": no report for 30 seconds ends the run as failed and restores A.
func TestSimulatorLost(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	ctx := context.Background()
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	run, err := s.Sim.Start(ctx, ev.Org, rh.ID, "test", simulation.Knobs{Fans: 100, DeclineRate: f(0.2)})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Sim.Claim(ctx)
	if err != nil || a == nil || a.RunID != run.ID {
		t.Fatalf("claim: %v %+v", err, a)
	}
	if k := knobsOf(t, s, "a", ev.Org, rh.EventID); k["decline_rate"] != 0.2 {
		t.Fatalf("decline rate not set for the rehearsal: %v", k)
	}
	if k := knobsOf(t, s, "a", ev.Org, ""); k["decline_rate"] != 0.0 {
		t.Fatalf("the organiser's own knobs changed: %v", k)
	}
	if _, err := s.Pool.Exec(ctx, `update sales.simulation_runs set claimed_at = now() - interval '31 seconds' where id = $1`, run.ID); err != nil {
		t.Fatal(err)
	}
	s.Sim.Sweep(ctx)
	r, _ := s.Sim.Get(ctx, ev.Org, run.ID)
	if r.State != simulation.Failed || r.Error == nil {
		t.Fatalf("lost run: %s", r.State)
	}
	if k := knobsOf(t, s, "a", ev.Org, rh.EventID); k["decline_rate"] != 0.0 || k["outage"] == true {
		t.Fatalf("knobs not restored: %v", k)
	}
	// A pending run nobody claims for a minute fails too.
	rh2, _ := s.Sim.ResetRehearsal(ctx, ev.Org, rh.ID, "test")
	run2, err := s.Sim.Start(ctx, ev.Org, rh2.ID, "test", simulation.Knobs{Fans: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Pool.Exec(ctx, `update sales.simulation_runs set created_at = now() - interval '61 seconds' where id = $1`, run2.ID)
	s.Sim.Sweep(ctx)
	if r, _ := s.Sim.Get(ctx, ev.Org, run2.ID); r.State != simulation.Failed {
		t.Fatalf("unclaimed run: %s", r.State)
	}
}

// Scenario "Outage on schedule": A's settings for the rehearsal change at 20 s after opening and
// return at 50 s; a report after the run's end tells the simulator to stop.
func TestOutageSchedule(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	ctx := context.Background()
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	run, err := s.Sim.Start(ctx, ev.Org, rh.ID, "test", simulation.Knobs{Fans: 100, DurationSeconds: 60,
		ProviderA: &simulation.ProviderPlan{Mode: "outage", AtSeconds: 20, ForSeconds: 30}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Sim.Claim(ctx)
	if err != nil || a == nil {
		t.Fatal(err)
	}
	clock := a.OpensAt
	s.Sim.Now = func() time.Time { return clock }
	report := func(at time.Duration) simulation.Reply {
		clock = a.OpensAt.Add(at)
		rep, err := s.Sim.ReceiveReport(ctx, ev.Org, run.ID, simulation.Report{Phase: "running"})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	report(19 * time.Second)
	if k := knobsOf(t, s, "a", ev.Org, rh.EventID); k["outage"] == true {
		t.Fatal("outage before its time")
	}
	report(20 * time.Second)
	if k := knobsOf(t, s, "a", ev.Org, rh.EventID); k["outage"] != true {
		t.Fatalf("no outage at 20 s: %v", k)
	}
	if k := knobsOf(t, s, "b", ev.Org, rh.EventID); k["outage"] == true {
		t.Fatal("B is down too")
	}
	if k := knobsOf(t, s, "a", ev.Org, ""); k["outage"] == true {
		t.Fatal("the organiser's real merchant is down")
	}
	report(49 * time.Second)
	if k := knobsOf(t, s, "a", ev.Org, rh.EventID); k["outage"] != true {
		t.Fatal("outage ended early")
	}
	report(50 * time.Second)
	if k := knobsOf(t, s, "a", ev.Org, rh.EventID); k["outage"] == true {
		t.Fatalf("outage not ended at 50 s: %v", k)
	}
	r, _ := s.Sim.Get(ctx, ev.Org, run.ID)
	if r.OutageOnAt == nil || r.OutageOffAt == nil || r.State != simulation.Running {
		t.Fatalf("run: %+v", r)
	}
	if rep := report(60*time.Second + simulation.Grace + time.Second); !rep.Stop {
		t.Fatal("a run past its duration and grace is not told to stop")
	}
}

// Review D-e: the simulator stops every fan by itself when the core stops answering its reports.
func TestSimulatorStopsWhenTheCoreIsGone(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 1000, 20)
	ctx := s.Context(t)
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	run, err := s.Sim.Start(ctx, ev.Org, rh.ID, "test", simulation.Knobs{Fans: 200, OpensInSeconds: 30, ProviderA: &simulation.ProviderPlan{Mode: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	s.Engine.LostAfter = 3 * time.Second
	s.RunSimulator(ctx)
	waitState(t, s, ev.Org, run.ID, 20*time.Second, simulation.Running)
	time.Sleep(time.Second)
	s.Internal.Close() // the core's internal API is gone (a network split, a dead core)
	time.Sleep(6 * time.Second)
	hits := s.GatewayHits.Load()
	time.Sleep(4 * time.Second)
	if after := s.GatewayHits.Load(); after != hits {
		t.Fatalf("%d requests after the simulator lost the core", after-hits)
	}
}
