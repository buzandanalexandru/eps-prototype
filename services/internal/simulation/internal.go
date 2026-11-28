package simulation

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/live"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Offer is something a simulated fan may buy: a visible product (and its day or slot), weighted by
// the capacity behind it.
type Offer struct {
	ProductID   string `json:"productId"`
	Instance    string `json:"instance,omitempty"`
	MaxPerOrder int    `json:"maxPerOrder"`
	Weight      int64  `json:"weight"`
}

// Assignment is a claimed run as the simulator receives it.
type Assignment struct {
	RunID       string    `json:"runId"`
	OrganiserID string    `json:"organiserId"`
	SaleID      string    `json:"saleId"`
	EventID     string    `json:"eventId"`
	Version     int       `json:"version"`
	OpensAt     time.Time `json:"opensAt"`
	CapPerOrder int       `json:"capPerOrder"`
	Knobs       Knobs     `json:"knobs"`
	Offers      []Offer   `json:"offers"`
}

// Report is what the simulator sends every second (counts and latencies only: no personal data).
type Report struct {
	Phase    string               `json:"phase"` // running | finishing | done | failed
	Fans     map[string]int       `json:"fans"`  // by state
	Bots     map[string]int       `json:"bots"`  // by state
	Truth    Truth                `json:"truth"`
	Requests map[string]StepStats `json:"requests"` // by step (p95 over the run, sampled)
	RPS      float64              `json:"rps"`      // average over the run
	PeakRPS  float64              `json:"peakRps"`  // the busiest second
	// BotPlaces are the queue places of the bots (sent only in the final report): the core counts
	// which of them the system flagged for the summary, after the run.
	BotPlaces []string `json:"botPlaces,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Truth is the simulator's ground truth.
type Truth struct {
	// Planned at the start; Sent once a client made its first request (review D-f).
	FansPlanned    int    `json:"fansPlanned"`
	BotsPlanned    int    `json:"botsPlanned"`
	FansSent       int    `json:"fansSent"`
	BotsSent       int    `json:"botsSent"`
	FansAdmitted   int    `json:"fansAdmitted"`
	BotsAdmitted   int    `json:"botsAdmitted"`
	FanTickets     int    `json:"fanTickets"`
	BotTickets     int    `json:"botTickets"`
	FansAskedOTP   int    `json:"fansAskedOtp"`
	BotsAskedOTP   int    `json:"botsAskedOtp"`
	SkipsRefused   int    `json:"skipsRefused"`
	CapsRefused    int    `json:"capsRefused"`
	BusyAnswers    int    `json:"busyAnswers"`
	SoldOutAt      string `json:"soldOutAt,omitempty"`
	FansAbandoned  int    `json:"fansAbandoned"`
	FansDeclined   int    `json:"fansDeclined"`
	JoinsRateLimit int    `json:"joinsRateLimited"`
}

// StepStats are one step's requests.
type StepStats struct {
	Count  int     `json:"count"`
	Errors int     `json:"errors"`
	P50Ms  float64 `json:"p50Ms"`
	P95Ms  float64 `json:"p95Ms"`
}

// Reply tells the simulator whether to stop.
type Reply struct {
	Stop  bool   `json:"stop"`
	State string `json:"state"`
}

// Grace is how long past its duration a run may finish (fans in flight) before the core stops it.
const Grace = 90 * time.Second

// Offers lists what simulated fans buy from a snapshot: visible products that need no other
// product, on event pools or on one day or slot pool (one offer per instance).
func Offers(s *snapshot.Snapshot) []Offer {
	pools := map[string]snapshot.Pool{}
	for _, p := range s.Pools {
		pools[p.PoolID] = p
	}
	var out []Offer
	for _, p := range s.Products {
		if !p.Visible || len(p.Requires) > 0 || len(p.PriceSteps) == 0 || len(p.PoolIDs) == 0 {
			continue
		}
		maxN := p.MaxPerOrder
		if maxN <= 0 {
			maxN = 10
		}
		var dated *snapshot.Pool
		var cap int64 = -1
		ok := true
		for _, id := range p.PoolIDs {
			pl, found := pools[id]
			if !found {
				ok = false
				break
			}
			if pl.Scope == "event" && len(pl.Instances) == 1 {
				if c := pl.Instances[0].Capacity; cap < 0 || c < cap {
					cap = c
				}
				continue
			}
			if dated != nil {
				ok = false // two dated pools: skip (the simulator keeps to simple tickets)
				break
			}
			dated = &pl
		}
		if !ok {
			continue
		}
		if dated == nil {
			if cap > 0 {
				out = append(out, Offer{ProductID: p.ProductID, MaxPerOrder: maxN, Weight: cap})
			}
			continue
		}
		for _, in := range dated.Instances {
			_, inst, found := strings.Cut(in.InstanceID, "@")
			if !found || in.Capacity <= 0 {
				continue
			}
			w := in.Capacity
			if cap >= 0 && cap < w {
				w = cap
			}
			out = append(out, Offer{ProductID: p.ProductID, Instance: inst, MaxPerOrder: maxN, Weight: w})
		}
	}
	return out
}

// Claim takes the oldest pending run (design D-2): moves it to starting, opens its rehearsal at
// now + opensIn with the pre-queue open at once, and sets the decline rate on both providers for
// the rehearsal's merchant. Nil when there is nothing to do.
func (s *Service) Claim(ctx context.Context) (*Assignment, error) {
	var cands []Run
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id::text, organiser_id::text, sale_id::text, event_id::text, state, last_report_at, created_at
			from sales.simulation_runs_in($1, 5)`, []string{Pending})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Run
			if err := rows.Scan(&r.ID, &r.OrganiserID, &r.SaleID, &r.EventID, &r.State, &r.LastReportAt, &r.CreatedAt); err != nil {
				return err
			}
			cands = append(cands, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		var r *Run
		err := db.WithTx(ctx, s.Pool, db.RoleCore, c.OrganiserID, func(tx pgx.Tx) error {
			var err error
			r, err = scanRun(tx.QueryRow(ctx, `update sales.simulation_runs set state = 'starting', claimed_at = now()
				where id = $1 and state = 'pending' and not stop_requested returning `+runCols, c.ID))
			return err
		})
		if err != nil {
			continue // taken by another simulator, or stopped meanwhile
		}
		a, err := s.prepare(ctx, r)
		if err != nil {
			s.fail(ctx, r, "The rehearsal could not be opened.")
			s.log().Warn("simulation run not started", "run", r.ID, "error", err.Error())
			continue
		}
		return a, nil
	}
	return nil, nil
}

func (s *Service) prepare(ctx context.Context, r *Run) (*Assignment, error) {
	opens := s.now().Add(time.Duration(r.Knobs.OpensInSeconds) * time.Second)
	sale, err := s.Sales.OpenRehearsal(ctx, r.OrganiserID, r.SaleID, "simulator", opens)
	if err != nil {
		return nil, err
	}
	snap, err := s.Snaps.Load(ctx, r.EventID)
	if err != nil {
		return nil, err
	}
	offers := Offers(snap)
	if len(offers) == 0 {
		return nil, errors.New("nothing a simulated fan can buy")
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, r.OrganiserID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update sales.simulation_runs set opens_at = $2 where id = $1`, r.ID, sale.Settings.OpensAt)
		return err
	})
	if err != nil {
		return nil, err
	}
	if d := *r.Knobs.DeclineRate; d > 0 {
		for _, p := range []string{"a", "b"} {
			s.merchantKnobs(ctx, p, r.OrganiserID, r.EventID, map[string]any{"decline_rate": d})
		}
	}
	s.watch(ctx, r.SaleID)
	return &Assignment{RunID: r.ID, OrganiserID: r.OrganiserID, SaleID: r.SaleID, EventID: r.EventID, Version: snap.Version,
		OpensAt: sale.Settings.OpensAt, CapPerOrder: sale.Settings.CapPerOrder, Knobs: r.Knobs, Offers: offers}, nil
}

// PendingCount is how many runs wait for a simulator.
func (s *Service) PendingCount(ctx context.Context) (int, error) {
	var n int
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select count(*) from sales.simulation_runs_in($1, 100)`, []string{Pending}).Scan(&n)
	})
	return n, err
}

// RefusePending ends every pending run with why (the simulator cannot take them now).
func (s *Service) RefusePending(ctx context.Context, why string) {
	var list []Run
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id::text, organiser_id::text, sale_id::text, event_id::text, state, last_report_at, created_at
			from sales.simulation_runs_in($1, 20)`, []string{Pending})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Run
			if err := rows.Scan(&r.ID, &r.OrganiserID, &r.SaleID, &r.EventID, &r.State, &r.LastReportAt, &r.CreatedAt); err != nil {
				return err
			}
			list = append(list, r)
		}
		return rows.Err()
	})
	for i := range list {
		s.fail(ctx, &list[i], why)
	}
}

// fail ends a run as failed and restores its rehearsal's providers.
func (s *Service) fail(ctx context.Context, r *Run, why string) {
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, r.OrganiserID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update sales.simulation_runs set state = 'failed', ended_at = now(), error = $2
			where id = $1 and state = any($3)`, r.ID, why, ActiveStates)
		return err
	})
	s.restoreKnobs(context.WithoutCancel(ctx), r.OrganiserID, r.EventID)
	s.closeRehearsal(context.WithoutCancel(ctx), r)
}

// closeRehearsal shuts the run's rehearsal again (review D-c).
func (s *Service) closeRehearsal(ctx context.Context, r *Run) {
	if err := s.Sales.CloseRehearsal(ctx, r.OrganiserID, r.SaleID, "simulator"); err != nil {
		s.log().Warn("rehearsal not closed after its run", "run", r.ID, "error", err.Error())
	}
}

// ApplySchedule changes provider A on the plan's times, once each (design D-2).
func (s *Service) applySchedule(ctx context.Context, tx pgx.Tx, r *Run, now time.Time) error {
	a := r.Knobs.ProviderA
	if a == nil || a.Mode == "none" || r.OpensAt == nil {
		return nil
	}
	on := r.OpensAt.Add(time.Duration(a.AtSeconds) * time.Second)
	off := on.Add(time.Duration(a.ForSeconds) * time.Second)
	base := map[string]any{"decline_rate": *r.Knobs.DeclineRate}
	if r.OutageOnAt == nil && !now.Before(on) && now.Before(off) {
		k := map[string]any{"decline_rate": *r.Knobs.DeclineRate}
		if a.Mode == "outage" {
			k["outage"] = true
		} else {
			k["failure_rate"] = a.FailureRate
		}
		s.merchantKnobs(ctx, "a", r.OrganiserID, r.EventID, k)
		if _, err := tx.Exec(ctx, `update sales.simulation_runs set outage_on_at = $2 where id = $1`, r.ID, now); err != nil {
			return err
		}
		t := now
		r.OutageOnAt = &t
	}
	if r.OutageOnAt != nil && r.OutageOffAt == nil && !now.Before(off) {
		s.merchantKnobs(ctx, "a", r.OrganiserID, r.EventID, base)
		if _, err := tx.Exec(ctx, `update sales.simulation_runs set outage_off_at = $2 where id = $1`, r.ID, now); err != nil {
			return err
		}
		t := now
		r.OutageOffAt = &t
	}
	return nil
}

// ReceiveReport records a report and answers whether to stop (design D-2).
func (s *Service) ReceiveReport(ctx context.Context, org, runID string, rep Report) (Reply, error) {
	reports.Inc()
	now := s.now()
	var reply Reply
	var ended *Run
	saleID := ""
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		r, err := scanRun(tx.QueryRow(ctx, `select `+runCols+` from sales.simulation_runs where id = $1 for update`, runID))
		if err != nil {
			return err
		}
		saleID = r.SaleID
		if !r.Active() {
			reply = Reply{Stop: true, State: r.State}
			return nil
		}
		rb, _ := json.Marshal(rep)
		next := r.State
		switch rep.Phase {
		case "running":
			if r.State == Starting {
				next = Running
			}
		case "finishing":
			next = Finishing
		case "done":
			next = Done
			if r.StopRequested {
				next = Stopped
			}
		case "failed":
			next = Failed
		}
		if _, err := tx.Exec(ctx, `update sales.simulation_runs set report = $2, last_report_at = $3, state = $4 where id = $1`, r.ID, rb, now, next); err != nil {
			return err
		}
		r.State = next
		if r.Active() {
			if err := s.applySchedule(ctx, tx, r, now); err != nil {
				return err
			}
		} else {
			summary := s.summarise(ctx, tx, r, rep)
			sb, _ := json.Marshal(summary)
			var msg *string
			if rep.Error != "" {
				e := rep.Error
				if len(e) > 200 {
					e = e[:200]
				}
				msg = &e
			}
			if _, err := tx.Exec(ctx, `update sales.simulation_runs set summary = $2, ended_at = $3, error = coalesce($4, error) where id = $1`, r.ID, sb, now, msg); err != nil {
				return err
			}
			ended = r
		}
		late := r.OpensAt != nil && now.After(r.OpensAt.Add(time.Duration(r.Knobs.DurationSeconds)*time.Second+Grace))
		reply = Reply{Stop: r.StopRequested || late || !r.Active(), State: r.State}
		return nil
	})
	if err != nil {
		return Reply{}, err
	}
	if ended != nil {
		s.restoreKnobs(context.WithoutCancel(ctx), ended.OrganiserID, ended.EventID)
		s.closeRehearsal(context.WithoutCancel(ctx), ended)
	}
	if s.Redis != nil && saleID != "" {
		s.watch(ctx, saleID)
		rep.BotPlaces = nil // counted once at the end; not kept in Redis
		b, _ := json.Marshal(struct {
			RunID string `json:"runId"`
			State string `json:"state"`
			Report
		}{runID, reply.State, rep})
		_ = live.StoreReport(ctx, s.Redis, saleID, b)
	}
	return reply, nil
}

// Sweep ends runs nobody claimed or whose simulator stopped reporting (every core runs it; the
// conditional update decides once).
func (s *Service) Sweep(ctx context.Context) {
	var list []Run
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var err error
		list, err = activeAnywhere(ctx, tx)
		return err
	})
	now := s.now()
	for i := range list {
		r := list[i]
		full, err := s.Get(ctx, r.OrganiserID, r.ID)
		if err != nil || !full.Active() {
			continue
		}
		switch {
		case full.State == Pending && now.Sub(full.CreatedAt) > ClaimTimeout:
			s.fail(ctx, full, "No simulator took the run. Try again later.")
		case full.State != Pending:
			last := full.ClaimedAt
			if full.LastReportAt != nil {
				last = full.LastReportAt
			}
			if last != nil && now.Sub(*last) > LostAfter {
				runsLost.Inc()
				s.fail(ctx, full, "The simulator stopped reporting.")
			}
		}
	}
}

// Facts are what the system itself recorded for a rehearsal (the summary's sales figures).
type Facts struct {
	PaidOrders    int   `json:"paidOrders"`
	TicketsSold   int   `json:"ticketsSold"`
	RevenueMinor  int64 `json:"revenueMinor"`
	Attempts      int   `json:"attempts"`
	Succeeded     int   `json:"succeeded"`
	Declined      int   `json:"declined"`
	FailedAtA     int   `json:"failedAtA"`
	PaidAtB       int   `json:"paidAtB"`
	Failovers     int   `json:"failovers"`
	OTPChallenged int   `json:"otpChallenged"`
	// OrdersPaying are orders that pressed Pay at least once; OrdersPaid of them succeeded.
	OrdersPaying int `json:"ordersPaying"`
	OrdersPaid   int `json:"ordersPaid"`
	// PaidAtBDuringOutage: payments that succeeded at B while A was down (the failover's effect).
	PaidAtBDuringOutage int `json:"paidAtBDuringOutage"`
}

// FactsOf reads a rehearsal's orders and attempts (organiser scope).
func FactsOf(ctx context.Context, tx pgx.Tx, event string) (Facts, error) {
	var f Facts
	if err := tx.QueryRow(ctx, `select count(*), coalesce(sum(l.n), 0), coalesce(sum(o.total_minor), 0) from sales.orders o
		left join lateral (select sum(qty) n from sales.order_lines where order_id = o.id) l on true
		where o.event_id = $1 and o.state in ('paid', 'issued')`, event).Scan(&f.PaidOrders, &f.TicketsSold, &f.RevenueMinor); err != nil {
		return f, err
	}
	if err := tx.QueryRow(ctx, `select count(*), count(*) filter (where a.state = 'succeeded'), count(*) filter (where a.state = 'declined'),
		count(*) filter (where a.state = 'failed' and a.provider = 'a'), count(*) filter (where a.state = 'succeeded' and a.provider = 'b'),
		count(*) filter (where a.failover_of is not null), count(distinct a.order_id), count(distinct a.order_id) filter (where a.state = 'succeeded')
		from sales.payment_attempts a join sales.orders o on o.organiser_id = a.organiser_id and o.id = a.order_id
		where o.event_id = $1 and a.purpose = 'order'`, event).Scan(&f.Attempts, &f.Succeeded, &f.Declined, &f.FailedAtA, &f.PaidAtB, &f.Failovers, &f.OrdersPaying, &f.OrdersPaid); err != nil {
		return f, err
	}
	err := tx.QueryRow(ctx, `select count(distinct c.order_id) from sales.otp_challenges c join sales.orders o on o.organiser_id = c.organiser_id and o.id = c.order_id
		where o.event_id = $1 and c.outcome = 'sent'`, event).Scan(&f.OTPChallenged)
	return f, err
}

// Summary is a run's summary (design D-8: measured numbers only).
type Summary struct {
	Truth              Truth                `json:"truth"`
	Facts              Facts                `json:"facts"`
	PaymentSuccessRate float64              `json:"paymentSuccessRate"`
	SellOutSeconds     *float64             `json:"sellOutSeconds,omitempty"`
	BotsFlagged        int                  `json:"botsFlagged"`
	FansFlagged        int                  `json:"fansFlagged"`
	PlacesFlagged      int64                `json:"placesFlagged"`
	Requests           map[string]StepStats `json:"requests"`
	RPS                float64              `json:"rps"`
	PeakRPS            float64              `json:"peakRps"`
	Seconds            float64              `json:"seconds"`
}

func (s *Service) summarise(ctx context.Context, tx pgx.Tx, r *Run, rep Report) Summary {
	out := Summary{Truth: rep.Truth, Requests: rep.Requests, RPS: rep.RPS, PeakRPS: rep.PeakRPS}
	if f, err := FactsOf(ctx, tx, r.EventID); err == nil {
		out.Facts = f
		// Per order (a fan whose payment failed over to B and succeeded paid): paid of those that
		// pressed Pay.
		if f.OrdersPaying > 0 {
			out.PaymentSuccessRate = float64(f.OrdersPaid) / float64(f.OrdersPaying)
		}
	}
	if r.OutageOnAt != nil {
		end := s.now()
		if r.OutageOffAt != nil {
			end = *r.OutageOffAt
		}
		_ = tx.QueryRow(ctx, `select count(*) from sales.payment_attempts a join sales.orders o on o.organiser_id = a.organiser_id and o.id = a.order_id
			where o.event_id = $1 and a.provider = 'b' and a.state = 'succeeded' and a.created_at between $2 and $3`, r.EventID, *r.OutageOnAt, end).Scan(&out.Facts.PaidAtBDuringOutage)
	}
	if r.OpensAt != nil {
		out.Seconds = s.now().Sub(*r.OpensAt).Seconds()
		// Sold out by the system's own record (review D-f): every pool instance's paid units reached
		// its capacity; the time is the last paid order's.
		if at := s.soldOutAt(ctx, tx, r.EventID); at != nil && at.After(*r.OpensAt) {
			v := at.Sub(*r.OpensAt).Seconds()
			out.SellOutSeconds = &v
		}
	}
	if s.Redis != nil {
		if c, err := queue.CountsOf(ctx, s.Redis, r.OrganiserID, r.EventID, r.SaleID, s.now()); err == nil {
			out.PlacesFlagged = c.Flagged
		}
		out.BotsFlagged = flaggedAmong(ctx, s.Redis, r, rep.BotPlaces)
		if n := int(out.PlacesFlagged) - out.BotsFlagged; n > 0 {
			out.FansFlagged = n
		}
	}
	return out
}

// soldOutAt is when the rehearsal sold its whole capacity (nil when it did not).
func (s *Service) soldOutAt(ctx context.Context, tx pgx.Tx, event string) *time.Time {
	if s.Snaps == nil {
		return nil
	}
	snap, err := s.Snaps.Load(ctx, event)
	if err != nil {
		return nil
	}
	sold := map[string]int64{}
	rows, err := tx.Query(ctx, `select d.key, sum(d.value::bigint) from sales.orders o, jsonb_each_text(o.draws) d
		where o.event_id = $1 and o.state in ('paid', 'issued') group by d.key`, event)
	if err != nil {
		return nil
	}
	for rows.Next() {
		var k string
		var n int64
		if rows.Scan(&k, &n) == nil {
			sold[k] = n
		}
	}
	rows.Close()
	for _, p := range snap.Pools {
		for _, in := range p.Instances {
			if sold[in.InstanceID] < in.Capacity {
				return nil
			}
		}
	}
	var at *time.Time
	_ = tx.QueryRow(ctx, `select max(paid_at) from sales.orders where event_id = $1 and state in ('paid', 'issued')`, event).Scan(&at)
	return at
}

// flaggedAmong counts the bots' places the system scored as suspects (after the run only).
func flaggedAmong(ctx context.Context, rdb *redis.Client, r *Run, places []string) int {
	k, err := queue.KeysOf(r.OrganiserID, r.EventID, r.SaleID)
	if err != nil || len(places) == 0 {
		return 0
	}
	if len(places) > 50_000 {
		places = places[:50_000]
	}
	n := 0
	for i := 0; i < len(places); i += 500 {
		pipe := rdb.Pipeline()
		end := min(i+500, len(places))
		cmds := make([]*redis.StringCmd, 0, end-i)
		for _, q := range places[i:end] {
			if len(q) > 64 {
				continue
			}
			cmds = append(cmds, pipe.HGet(ctx, k.Bot(q), "score"))
		}
		_, _ = pipe.Exec(ctx)
		for _, c := range cmds {
			if v, err := c.Int(); err == nil && v >= botdefence.SuspectAt {
				n++
			}
		}
	}
	return n
}

// InternalHTTP serves the simulator's API on the core's internal port (not behind the ingress;
// bearer SIM_CONTROL_TOKEN, compared in constant time).
type InternalHTTP struct {
	Svc   *Service
	Token string
}

// Routes mounts the internal operations.
func (h *InternalHTTP) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/sim/claim", h.guard(h.claim))
	mux.HandleFunc("POST /internal/sim/runs/{runId}/report", h.guard(h.report))
}

func (h *InternalHTTP) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if h.Token == "" || len(got) != len(h.Token) || subtle.ConstantTimeCompare([]byte(got), []byte(h.Token)) != 1 {
			httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthenticated", "")
			return
		}
		next(w, r)
	}
}

func (h *InternalHTTP) claim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ClusterBusy bool `json:"clusterBusy"`
		Peek        bool `json:"peek"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	if body.Peek {
		n, err := h.Svc.PendingCount(r.Context())
		if err != nil {
			httpx.WriteProblem(w, r, http.StatusServiceUnavailable, "EPS-UNAVAILABLE", "")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"pending": n > 0})
		return
	}
	if body.ClusterBusy {
		h.Svc.RefusePending(r.Context(), "Another rehearsal is running on this cluster. Try again in a few minutes.")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	a, err := h.Svc.Claim(r.Context())
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusServiceUnavailable, "EPS-UNAVAILABLE", "")
		return
	}
	if a == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

func (h *InternalHTTP) report(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrganiserID string `json:"organiserId"`
		Report      Report `json:"report"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid_request", "")
		return
	}
	rep, err := h.Svc.ReceiveReport(r.Context(), body.OrganiserID, r.PathValue("runId"), body.Report)
	if err != nil {
		status := http.StatusServiceUnavailable
		var p interface{ Error() string }
		if errors.As(err, &p) && strings.Contains(err.Error(), "NOT-FOUND") {
			status = http.StatusNotFound
		}
		httpx.WriteProblem(w, r, status, "EPS-SIM-REPORT", "")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rep)
}
