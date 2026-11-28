package simulation

import (
	"context"
	"encoding/json"
	"log/slog"
	mrand "math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// Engine is the simulator service (cmd/simulator): it asks the core for a run every 2 seconds and
// drives it, one run at a time, with at most MaxInflight requests in flight.
type Engine struct {
	Endpoints   Endpoints
	Token       string // SIM_CONTROL_TOKEN
	Minter      *Minter
	MaxInflight int
	Log         *slog.Logger
	HTTP        *http.Client
	// Poll is how often it asks for work (default 2 s); Report how often it reports (1 s).
	Poll, Report time.Duration
	// Finish is how long fans in flight may finish after the run's end (default 60 s).
	Finish time.Duration
	// MaxRPS caps the simulator's requests per second over all fans (default 1,500; review D-b).
	MaxRPS int
	// Lease, when set, is the cluster-wide right to run (one rehearsal at a time on shared nodes).
	Lease Lease
	// LostAfter stops every fan when no report has succeeded for this long (default 25 s; review D-e).
	LostAfter time.Duration
}

var (
	gaugeOnce sync.Once
	gaugeMu   sync.Mutex
	gaugeT    *tally
)

func (e *Engine) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	n := e.MaxInflight
	if n <= 0 {
		n = 200
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns, tr.MaxIdleConnsPerHost, tr.MaxConnsPerHost = 2*n, n, n
	e.HTTP = &http.Client{Timeout: 15 * time.Second, Transport: tr}
	return e.HTTP
}

// Run asks for work until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	gaugeOnce.Do(registerSimGauges)
	poll := e.Poll
	if poll <= 0 {
		poll = 2 * time.Second
	}
	log := e.Log
	if log == nil {
		log = slog.Default()
	}
	for ctx.Err() == nil {
		if e.Lease != nil {
			if !e.pending(ctx) {
				sleep(ctx, poll)
				continue
			}
			ok, err := e.Lease.Acquire(ctx)
			if err != nil {
				log.Warn("simulator cluster lease unavailable", "error", err.Error())
			}
			if !ok || err != nil {
				// Fail closed: tell the core so a waiting run ends with a clear reason.
				e.claimBusy(ctx)
				sleep(ctx, poll)
				continue
			}
		}
		a, err := e.claim(ctx)
		if err != nil {
			log.Warn("simulator claim failed", "error", err.Error())
		}
		if a != nil {
			log.Info("simulation run started", "run", a.RunID, "fans", a.Knobs.Fans, "opensAt", a.OpensAt)
			e.Drive(ctx, a)
			log.Info("simulation run ended", "run", a.RunID)
			if e.Lease != nil {
				e.Lease.Release(context.WithoutCancel(ctx))
			}
			continue
		}
		if e.Lease != nil {
			e.Lease.Release(ctx)
		}
		sleep(ctx, poll)
	}
}

// pending asks the core whether a run waits (without claiming it).
func (e *Engine) pending(ctx context.Context) bool {
	c := &caller{http: e.client(), slots: make(chan struct{}, 1), st: &stats{}}
	ans := c.do(ctx, "claim", http.MethodPost, e.Endpoints.Internal+"/internal/sim/claim", map[string]string{"Authorization": "Bearer " + e.Token}, map[string]any{"peek": true})
	var out struct {
		Pending bool `json:"pending"`
	}
	return ans.ok() && json.Unmarshal(ans.body, &out) == nil && out.Pending
}

// claimBusy tells the core that the cluster's lease is held elsewhere (pending runs end with that).
func (e *Engine) claimBusy(ctx context.Context) {
	c := &caller{http: e.client(), slots: make(chan struct{}, 1), st: &stats{}}
	_ = c.do(ctx, "claim", http.MethodPost, e.Endpoints.Internal+"/internal/sim/claim", map[string]string{"Authorization": "Bearer " + e.Token}, map[string]any{"clusterBusy": true})
}

func (e *Engine) claim(ctx context.Context) (*Assignment, error) {
	c := &caller{http: e.client(), slots: make(chan struct{}, 1), st: &stats{}}
	ans := c.do(ctx, "claim", http.MethodPost, e.Endpoints.Internal+"/internal/sim/claim", map[string]string{"Authorization": "Bearer " + e.Token}, map[string]any{})
	if ans.status == http.StatusNoContent || ans.status == 0 && ctx.Err() != nil {
		return nil, nil
	}
	if !ans.ok() {
		return nil, &claimError{ans.status}
	}
	var a Assignment
	if err := json.Unmarshal(ans.body, &a); err != nil {
		return nil, err
	}
	a.Knobs = a.Knobs.Defaults()
	return &a, nil
}

type claimError struct{ status int }

func (c *claimError) Error() string { return "claim answered " + http.StatusText(c.status) }

// runner is one run in progress.
type runner struct {
	a      *Assignment
	ep     Endpoints
	minter *Minter
	call   *caller
	t      *tally
	qty    Weighted
	pay    Weighted
	cum    []int64
}

func (r *runner) pickOffer(rng *mrand.Rand) Offer {
	total := r.cum[len(r.cum)-1]
	x := rng.Int64N(total)
	for i, c := range r.cum {
		if x < c {
			return r.a.Offers[i]
		}
	}
	return r.a.Offers[0]
}

// Drive runs one assignment to its end: arrivals on schedule, a report every second, stop on the
// core's word, finishing after the duration (or 30 s after a sell-out), at most Finish more for
// fans in flight.
func (e *Engine) Drive(ctx context.Context, a *Assignment) {
	n := e.MaxInflight
	if n <= 0 {
		n = 200
	}
	r := &runner{a: a, ep: e.Endpoints, minter: e.Minter, t: &tally{fans: map[string]int{}, bots: map[string]int{}, state: map[int]string{}},
		qty: NewWeighted(a.Knobs.QtyMix), pay: NewWeighted(a.Knobs.PaymentMix)}
	rps := e.MaxRPS
	if rps <= 0 {
		rps = 1500
	}
	r.call = &caller{http: e.client(), slots: make(chan struct{}, n), st: &stats{start: time.Now()}, pace: newPacer(rps)}
	var total int64
	for _, o := range a.Offers {
		total += max(1, o.Weight)
		r.cum = append(r.cum, total)
	}
	gaugeMu.Lock()
	gaugeT = r.t
	gaugeMu.Unlock()
	seed := mrand.NewPCG(uint64(time.Now().UnixNano()), 7)
	rng := mrand.New(seed)
	clients := population(a, time.Now(), rng)
	for _, c := range clients {
		r.t.set(c, "arriving")
	}
	r.t.truth.FansSent, r.t.truth.BotsSent = 0, 0
	for _, c := range clients {
		if c.bot != "" {
			r.t.truth.BotsSent++
		} else {
			r.t.truth.FansSent++
		}
	}
	runCtx, stopAll := context.WithCancel(ctx)
	defer stopAll()
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		s := &session{r: r, c: c, rng: mrand.New(mrand.NewPCG(rng.Uint64(), uint64(c.i)))}
		go func() {
			defer wg.Done()
			s.journey(runCtx)
		}()
	}
	allDone := make(chan struct{})
	go func() { wg.Wait(); close(allDone) }()
	every := e.Report
	if every <= 0 {
		every = time.Second
	}
	finish := e.Finish
	if finish <= 0 {
		finish = 60 * time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	end := a.OpensAt.Add(time.Duration(a.Knobs.DurationSeconds) * time.Second)
	var finishing time.Time
	phase := "running"
	lost := e.LostAfter
	if lost <= 0 {
		lost = 25 * time.Second
	}
	lastOK, lastLease := time.Now(), time.Now()
	for {
		select {
		case <-allDone:
			e.report(ctx, r, "done")
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		now := time.Now()
		r.t.mu.Lock()
		soldOut := r.t.soldOut
		r.t.mu.Unlock()
		if finishing.IsZero() && (now.After(end) || (!soldOut.IsZero() && now.Sub(soldOut) > 30*time.Second)) {
			finishing, phase = now, "finishing"
		}
		if !finishing.IsZero() && now.Sub(finishing) > finish {
			stopAll()
			<-allDone
			e.report(ctx, r, "done")
			return
		}
		rep, ok := e.report(ctx, r, phase)
		if ok {
			lastOK = now
		}
		// The core is unreachable: stop every fan rather than drive traffic nobody can stop.
		if rep.Stop || now.Sub(lastOK) > lost {
			stopAll()
			<-allDone
			e.report(ctx, r, "done")
			return
		}
		if e.Lease != nil && now.Sub(lastLease) >= 10*time.Second {
			if held, err := e.Lease.Acquire(ctx); err != nil || !held {
				stopAll() // the cluster's lease was lost: stop at once
				<-allDone
				e.report(ctx, r, "done")
				return
			}
			lastLease = now
		}
	}
}

func (e *Engine) report(ctx context.Context, r *runner, phase string) (Reply, bool) {
	now := time.Now()
	r.t.mu.Lock()
	rep := Report{Phase: phase, Truth: r.t.truth, Fans: copyMap(r.t.fans), Bots: copyMap(r.t.bots)}
	if !r.t.soldOut.IsZero() {
		rep.Truth.SoldOutAt = r.t.soldOut.UTC().Format(time.RFC3339Nano)
	}
	if phase == "done" {
		rep.BotPlaces = append([]string(nil), r.t.botPlaces...)
	}
	r.t.mu.Unlock()
	rep.Requests, rep.RPS = r.call.st.snapshot(now)
	c := &caller{http: e.client(), slots: make(chan struct{}, 1), st: &stats{}}
	rctx := ctx
	if phase == "done" {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
	}
	ans := c.do(rctx, "report", http.MethodPost, e.Endpoints.Internal+"/internal/sim/runs/"+r.a.RunID+"/report",
		map[string]string{"Authorization": "Bearer " + e.Token}, map[string]any{"organiserId": r.a.OrganiserID, "report": rep})
	var reply Reply
	if ans.ok() {
		_ = json.Unmarshal(ans.body, &reply)
	} else if ans.status == http.StatusNotFound {
		reply.Stop = true
	}
	return reply, ans.ok()
}

func copyMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

// registerSimGauges exposes fans and bots by state (sim_fans{state}, sim_bots{state}).
func registerSimGauges() {
	for _, st := range []string{"arriving", "joining", "waiting", "admitted", "holding", "paying", "paid", "abandoned", "sold_out", "refused", "lost", "payment_failed", "expired", "unknown"} {
		st := st
		metrics.NewGauge(`sim_fans{state="`+st+`"}`, func() float64 { return gaugeOf(false, st) })
		metrics.NewGauge(`sim_bots{state="`+st+`"}`, func() float64 { return gaugeOf(true, st) })
	}
}

func gaugeOf(bot bool, st string) float64 {
	gaugeMu.Lock()
	t := gaugeT
	gaugeMu.Unlock()
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if bot {
		return float64(t.bots[st])
	}
	return float64(t.fans[st])
}
