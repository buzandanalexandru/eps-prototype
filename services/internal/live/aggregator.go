package live

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Frame is one second of a sale (design D-6). Every number is read from the system: queue and
// stock from Redis, sales and payments from Postgres, rates from the change of those totals.
type Frame struct {
	TS        int64           `json:"ts"`
	Sale      string          `json:"sale"`
	Event     string          `json:"event"`
	Name      string          `json:"name"`
	Rehearsal bool            `json:"rehearsal"`
	Phase     string          `json:"phase"`
	Queue     QueueFrame      `json:"queue"`
	Sales     SalesFrame      `json:"sales"`
	Payments  PaymentsFrame   `json:"payments"`
	Bots      BotsFrame       `json:"bots"`
	Stock     []StockLine     `json:"stock"`
	StockOK   bool            `json:"stockReady"`
	System    *System         `json:"system"`
	Degraded  bool            `json:"degraded"`
	Suggest   bool            `json:"suggestDegrade"`
	Run       json.RawMessage `json:"run,omitempty"`
	Timeline  []Event         `json:"timeline"`
}

// QueueFrame is the waiting room.
type QueueFrame struct {
	State          string  `json:"state"`
	Waiting        int64   `json:"waiting"`
	Prequeue       int64   `json:"prequeue"`
	Slow           int64   `json:"slow"`
	AdmittedTotal  int64   `json:"admittedTotal"`
	AdmittedPerMin float64 `json:"admittedPerMin"`
	Active         int64   `json:"active"`
	Rate           int     `json:"rate"`
	ActiveLimit    int     `json:"activeLimit"`
	Paused         bool    `json:"paused"`
}

// SalesFrame is what was sold (paid or issued orders of the sale's event).
type SalesFrame struct {
	Sold         int64   `json:"sold"`
	Capacity     int64   `json:"capacity"`
	PaidOrders   int64   `json:"paidOrders"`
	RevenueMinor int64   `json:"revenueMinor"`
	Currency     string  `json:"currency"`
	SoldPerMin   float64 `json:"soldPerMin"`
}

// ProviderFrame is one provider for the sale's merchant.
type ProviderFrame struct {
	State     string `json:"state"`
	Attempts  int64  `json:"attempts"`  // last minute
	Succeeded int64  `json:"succeeded"` // last minute
	Failures  int64  `json:"failures"`  // last minute (failed or declined)
}

// PaymentsFrame is the last minute of payments plus the failovers so far.
type PaymentsFrame struct {
	AttemptsPerMin float64                  `json:"attemptsPerMin"`
	FailuresPerMin float64                  `json:"failuresPerMin"`
	SuccessRate    *float64                 `json:"successRate"`
	FailoversTotal int64                    `json:"failoversTotal"`
	Providers      map[string]ProviderFrame `json:"providers"`
}

// BotsFrame is what the defences did.
type BotsFrame struct {
	Flagged        int64 `json:"flagged"`
	SentBack       int64 `json:"sentBack"` // suspects behind the line or in the slow lane now
	PassesRejected int64 `json:"passesRejected"`
	OTPChallenged  int64 `json:"otpChallenged"`
	OTPFailed      int64 `json:"otpFailed"`
}

// StockLine is one pool instance.
type StockLine struct {
	Instance string `json:"instance"`
	Pool     string `json:"pool"`
	Capacity int64  `json:"capacity"`
	Left     int64  `json:"left"`
	Held     int64  `json:"held"`
	Sold     int64  `json:"sold"`
}

// System is the cluster's view from Prometheus (null without it).
type System struct {
	GatewayRPS     *float64       `json:"gatewayRps"`
	CoreP95Ms      *float64       `json:"coreP95Ms"`
	CoreErrorShare *float64       `json:"coreErrorShare"`
	Pods           map[string]int `json:"pods"`
}

// Event is a timeline entry.
type Event struct {
	TS   int64  `json:"ts"`
	Kind string `json:"kind"` // breaker, failover, sold_out, degraded, run, opened
	// What changed, as a short machine value (the screen words it).
	Provider string `json:"provider,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	N        int64  `json:"n,omitempty"`
}

// Suggestion rule (D-9): core p95 over 2 s or 5xx over 5% in each of the last 30 frames.
const (
	SuggestFrames  = 30
	SuggestP95Ms   = 2000
	SuggestErrRate = 0.05
	maxStockLines  = 30
	timelineLen    = 20
)

// Aggregator builds the frames on the worker's lease holder.
type Aggregator struct {
	Redis *redis.Client
	Pool  *pgxpool.Pool
	Sales *sales.Service
	Snaps interface {
		Load(context.Context, string) (*snapshot.Snapshot, error)
	}
	Inv     *inventory.Sharded
	Breaker *payments.Breaker
	Prom    *Prom
	Log     *slog.Logger
	Now     func() time.Time

	// Parallel is how many frames are built at once (at most the aggregator's pool size).
	Parallel int

	mu       sync.Mutex
	state    map[string]*saleState
	rotation int
	warnedAt time.Time
}

type saleState struct {
	last     *Frame
	totals   []Point // the last 60 s of totals, oldest first
	timeline []Event
	high     int // consecutive frames over the suggestion limits
	seen     time.Time
}

var (
	frameErrors = metrics.NewCounter("eps_live_frame_errors_total", "Live frames that could not be built.")
	framesBuilt = metrics.NewCounter("eps_live_frames_total", "Live dashboard frames built.")
	frameTime   = metrics.NewHistogram("eps_live_frame_seconds", "Time to build one tick of live frames.", metrics.DefaultLatencyBuckets)
)

func (a *Aggregator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Run ticks every second while active() (the worker lease) until ctx ends.
func (a *Aggregator) Run(ctx context.Context, active func() bool) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if active == nil || active() {
				a.Tick(ctx)
			}
		}
	}
}

// Tick frames every watched sale once (at most MaxWatched, 4 at a time).
func (a *Aggregator) Tick(ctx context.Context) {
	start := time.Now()
	defer func() { frameTime.Observe(time.Since(start).Seconds()) }()
	now := a.now()
	all, err := Watched(ctx, a.Redis, now, WatchedScan)
	if err != nil || len(all) == 0 {
		return
	}
	ids := a.fair(ctx, all)
	var sys *System
	if a.Prom != nil {
		sys = a.Prom.System(ctx)
	}
	par := a.Parallel
	if par <= 0 {
		par = 2
	}
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
			defer cancel()
			f, err := a.Build(cctx, id, now, sys)
			if err != nil {
				a.failed(id, err)
				return
			}
			if f == nil {
				Unwatch(ctx, a.Redis, id)
				return
			}
			b, _ := json.Marshal(f)
			_ = Publish(ctx, a.Redis, id, b, Point{TS: f.TS, Waiting: f.Queue.Waiting, Admitted: f.Queue.AdmittedPerMin, Sold: f.Sales.SoldPerMin,
				Failovers: f.Payments.FailoversTotal, AdmittedTotal: f.Queue.AdmittedTotal, SoldTotal: f.Sales.Sold})
			framesBuilt.Inc()
		}(id)
	}
	wg.Wait()
	a.forget(now)
}

// fair picks at most MaxWatched sales, at most MaxWatchedPerOrg of one organiser, starting at a
// rotating offset so every watched sale gets its turn (review D-g).
func (a *Aggregator) fair(ctx context.Context, all []string) []string {
	a.mu.Lock()
	start := a.rotation % len(all)
	a.rotation++
	a.mu.Unlock()
	per := map[string]int{}
	var out []string
	for i := 0; i < len(all) && len(out) < MaxWatched; i++ {
		id := all[(start+i)%len(all)]
		org := ""
		if a.Sales != nil {
			if x, err := a.Sales.Lookup(ctx, id); err == nil {
				org = x.OrganiserID
			}
		}
		if per[org] >= MaxWatchedPerOrg {
			continue
		}
		per[org]++
		out = append(out, id)
	}
	return out
}

// failed counts a frame that could not be built and logs it at most once a minute.
func (a *Aggregator) failed(sale string, err error) {
	frameErrors.Inc()
	a.mu.Lock()
	warn := time.Since(a.warnedAt) > time.Minute
	if warn {
		a.warnedAt = time.Now()
	}
	a.mu.Unlock()
	if warn && a.Log != nil {
		a.Log.Warn("live frame not built", "sale", sale, "error", err.Error())
	}
}

// forget drops the state of sales not framed for 5 minutes.
func (a *Aggregator) forget(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, st := range a.state {
		if now.Sub(st.seen) > 5*time.Minute {
			delete(a.state, id)
		}
	}
}

func (a *Aggregator) stateOf(ctx context.Context, sale string) *saleState {
	a.mu.Lock()
	if a.state == nil {
		a.state = map[string]*saleState{}
	}
	st, ok := a.state[sale]
	a.mu.Unlock()
	if ok {
		return st
	}
	st = &saleState{}
	// A new lease holder picks the rates up from the shared history.
	if pts, err := History(ctx, a.Redis, sale, 60); err == nil {
		st.totals = pts
	}
	a.mu.Lock()
	a.state[sale] = st
	a.mu.Unlock()
	return st
}

// perMinute is the change of a total over the last minute of points (or what there is of it,
// once at least 5 seconds).
func perMinute(pts []Point, now int64, cur int64, pick func(Point) int64) float64 {
	for _, p := range pts {
		if now-p.TS <= 61_000 {
			dt := float64(now-p.TS) / 1000
			if dt < 5 {
				return 0
			}
			d := float64(cur - pick(p))
			if d < 0 {
				return 0
			}
			return d / dt * 60
		}
	}
	return 0
}

// Build makes one frame for a sale (nil, nil for a sale that no longer exists).
func (a *Aggregator) Build(ctx context.Context, saleID string, now time.Time, sys *System) (*Frame, error) {
	sale, err := a.Sales.Lookup(ctx, saleID)
	if err != nil {
		var p *apperr.Problem
		if errors.As(err, &p) && p.Code == apperr.SaleNotFound {
			return nil, nil
		}
		return nil, err
	}
	org, event := sale.OrganiserID, sale.EventID
	f := &Frame{TS: now.UnixMilli(), Sale: sale.ID, Event: event, Name: sale.Name, Rehearsal: sale.Rehearsal != nil,
		Phase: string(sale.Settings.PhaseAt(now)), Degraded: sale.Degraded, System: sys, Stock: []StockLine{}}
	f.Queue.Rate, f.Queue.ActiveLimit, f.Queue.Paused = sale.Settings.RatePerMinute, sale.Settings.ActiveLimit, sale.Settings.Paused
	if c, err := queue.CountsOf(ctx, a.Redis, org, event, sale.ID, now); err == nil {
		f.Queue.State, f.Queue.Waiting, f.Queue.Prequeue, f.Queue.Slow = c.State, c.Line+c.Slow, c.Prequeue, c.Slow
		f.Queue.AdmittedTotal, f.Queue.Active = c.Admitted, c.Active
		f.Bots.Flagged, f.Bots.PassesRejected, f.Bots.SentBack = c.Flagged, c.PassesRejected, c.Slow
	}
	// Stock: every pool instance of the live snapshot, summed over its shards.
	if a.Snaps != nil {
		if snap, err := a.Snaps.Load(ctx, event); err == nil {
			f.Sales.Currency = snap.Currency
			a.stock(ctx, f, snap, org, event)
		}
	}
	// Providers: the breaker of the sale's merchant (a rehearsal has its own).
	scope := org
	if sale.Rehearsal != nil {
		scope = payments.Scope(org, event)
	}
	f.Payments.Providers = map[string]ProviderFrame{}
	for _, p := range []string{"a", "b"} {
		pf := ProviderFrame{State: payments.Closed}
		if a.Breaker != nil {
			pf.State = a.Breaker.Read(ctx, scope, p).State
		}
		f.Payments.Providers[p] = pf
	}
	if a.Pool != nil {
		if err := a.facts(ctx, f, org, event); err != nil {
			return nil, err
		}
	}
	if a.Redis != nil {
		f.Run = LoadReport(ctx, a.Redis, sale.ID)
	}
	st := a.stateOf(ctx, sale.ID)
	a.mu.Lock()
	defer a.mu.Unlock()
	st.seen = now
	f.Queue.AdmittedPerMin = perMinute(st.totals, f.TS, f.Queue.AdmittedTotal, func(p Point) int64 { return p.AdmittedTotal })
	f.Sales.SoldPerMin = perMinute(st.totals, f.TS, f.Sales.Sold, func(p Point) int64 { return p.SoldTotal })
	st.totals = append(st.totals, Point{TS: f.TS, AdmittedTotal: f.Queue.AdmittedTotal, SoldTotal: f.Sales.Sold})
	for len(st.totals) > 0 && f.TS-st.totals[0].TS > 61_000 {
		st.totals = st.totals[1:]
	}
	// The suggestion counts consecutive frames over the limits; it never flips the switch.
	// While degraded the count starts again: the load it measures is the one the switch limits.
	if over(sys) && !f.Degraded {
		st.high++
	} else {
		st.high = 0
	}
	f.Suggest = !f.Degraded && st.high >= SuggestFrames
	st.timeline = append(st.timeline, transitions(st.last, f)...)
	if len(st.timeline) > timelineLen {
		st.timeline = st.timeline[len(st.timeline)-timelineLen:]
	}
	f.Timeline = append([]Event{}, st.timeline...)
	st.last = f
	return f, nil
}

func over(sys *System) bool {
	if sys == nil {
		return false
	}
	return (sys.CoreP95Ms != nil && *sys.CoreP95Ms > SuggestP95Ms) || (sys.CoreErrorShare != nil && *sys.CoreErrorShare > SuggestErrRate)
}

func (a *Aggregator) stock(ctx context.Context, f *Frame, snap *snapshot.Snapshot, org, event string) {
	type inst struct {
		id, pool string
		cap      int64
	}
	var list []inst
	for _, p := range snap.Pools {
		for _, in := range p.Instances {
			list = append(list, inst{in.InstanceID, p.Name, in.Capacity})
			f.Sales.Capacity += in.Capacity
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].cap > list[j].cap })
	if len(list) > 200 {
		list = list[:200]
	}
	ids := make([]string, len(list))
	for i, in := range list {
		ids[i] = in.id
	}
	levels := map[string]inventory.Level{}
	if a.Inv != nil {
		lv, err := a.Inv.Stock(ctx, org, event, ids)
		switch {
		case err == nil:
			levels, f.StockOK = lv, true
		case errors.Is(err, checkout.ErrNotReady):
			// Nothing was taken since the counters were (re)built: every unit is still free.
			f.StockOK = false
		}
	}
	for i, in := range list {
		if i >= maxStockLines {
			break
		}
		l, ok := levels[in.id]
		line := StockLine{Instance: in.id, Pool: in.pool, Capacity: in.cap, Left: in.cap}
		if ok {
			line.Left, line.Held, line.Sold = l.Free, l.Held, l.Sold
		}
		f.Stock = append(f.Stock, line)
	}
}

// facts reads the sale event's orders and the last minute of payments in one transaction.
func (a *Aggregator) facts(ctx context.Context, f *Frame, org, event string) error {
	return db.WithTx(ctx, a.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `select count(*), coalesce(sum(o.total_minor), 0),
			coalesce((select sum(l.qty) from sales.order_lines l join sales.orders x on x.organiser_id = l.organiser_id and x.id = l.order_id
			          where x.event_id = $1 and x.state in ('paid', 'issued')), 0)
			from sales.orders o where o.event_id = $1 and o.state in ('paid', 'issued')`, event).Scan(&f.Sales.PaidOrders, &f.Sales.RevenueMinor, &f.Sales.Sold); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `select a.provider, count(*), count(*) filter (where a.state = 'succeeded'),
			count(*) filter (where a.state in ('failed', 'declined'))
			from sales.payment_attempts a join sales.orders o on o.organiser_id = a.organiser_id and o.id = a.order_id
			where a.purpose = 'order' and a.created_at > now() - interval '60 seconds' and o.event_id = $1 group by a.provider`, event)
		if err != nil {
			return err
		}
		var att, ok, bad int64
		for rows.Next() {
			var p string
			var n, s, fl int64
			if err := rows.Scan(&p, &n, &s, &fl); err != nil {
				rows.Close()
				return err
			}
			pf := f.Payments.Providers[p]
			pf.Attempts, pf.Succeeded, pf.Failures = n, s, fl
			f.Payments.Providers[p] = pf
			att, ok, bad = att+n, ok+s, bad+fl
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		f.Payments.AttemptsPerMin, f.Payments.FailuresPerMin = float64(att), float64(bad)
		if ok+bad > 0 {
			r := float64(ok) / float64(ok+bad)
			f.Payments.SuccessRate = &r
		}
		if err := tx.QueryRow(ctx, `select count(*) from sales.payment_attempts a join sales.orders o on o.organiser_id = a.organiser_id and o.id = a.order_id
			where a.purpose = 'order' and a.failover_of is not null and o.event_id = $1`, event).Scan(&f.Payments.FailoversTotal); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `select count(distinct c.order_id) filter (where c.outcome = 'sent'), count(*) filter (where c.outcome in ('wrong', 'locked'))
			from sales.otp_challenges c join sales.orders o on o.organiser_id = c.organiser_id and o.id = c.order_id where o.event_id = $1`, event).Scan(&f.Bots.OTPChallenged, &f.Bots.OTPFailed)
	})
}

// transitions are the timeline entries between two frames.
func transitions(prev, cur *Frame) []Event {
	if prev == nil {
		return nil
	}
	var out []Event
	for _, p := range []string{"a", "b"} {
		if a, b := prev.Payments.Providers[p].State, cur.Payments.Providers[p].State; a != b {
			out = append(out, Event{TS: cur.TS, Kind: "breaker", Provider: p, From: a, To: b})
		}
	}
	if d := cur.Payments.FailoversTotal - prev.Payments.FailoversTotal; d > 0 {
		out = append(out, Event{TS: cur.TS, Kind: "failover", From: "a", To: "b", N: d})
	}
	if prev.Degraded != cur.Degraded {
		to := "off"
		if cur.Degraded {
			to = "on"
		}
		out = append(out, Event{TS: cur.TS, Kind: "degraded", To: to})
	}
	if prev.Queue.State != cur.Queue.State && cur.Queue.State != "" {
		out = append(out, Event{TS: cur.TS, Kind: "queue", From: prev.Queue.State, To: cur.Queue.State})
	}
	soldOut := func(f *Frame) bool {
		if !f.StockOK || len(f.Stock) == 0 {
			return false
		}
		for _, l := range f.Stock {
			if l.Left > 0 || l.Held > 0 {
				return false
			}
		}
		return true
	}
	if !soldOut(prev) && soldOut(cur) {
		out = append(out, Event{TS: cur.TS, Kind: "sold_out"})
	}
	if rs, rc := runState(prev.Run), runState(cur.Run); rc != "" && rs != rc {
		out = append(out, Event{TS: cur.TS, Kind: "run", From: rs, To: rc})
	}
	return out
}

func runState(b json.RawMessage) string {
	if len(b) == 0 {
		return ""
	}
	var x struct {
		State string `json:"state"`
	}
	_ = json.Unmarshal(b, &x)
	return x.State
}
