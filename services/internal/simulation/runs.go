package simulation

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/live"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Run states (design D-2).
const (
	Pending   = "pending"
	Starting  = "starting"
	Running   = "running"
	Finishing = "finishing"
	Done      = "done"
	Failed    = "failed"
	Stopped   = "stopped"
)

// ActiveStates are the states of a run that is not over.
var ActiveStates = []string{Pending, Starting, Running, Finishing}

// LostAfter ends a run whose simulator stopped reporting; ClaimTimeout fails a run no simulator
// took.
const (
	LostAfter    = 30 * time.Second
	ClaimTimeout = 60 * time.Second
)

// Run is a simulation run as Studio reads it.
type Run struct {
	ID            string          `json:"id"`
	OrganiserID   string          `json:"-"`
	SaleID        string          `json:"saleId"`
	EventID       string          `json:"eventId"`
	State         string          `json:"state"`
	Knobs         Knobs           `json:"knobs"`
	CreatedAt     time.Time       `json:"createdAt"`
	ClaimedAt     *time.Time      `json:"claimedAt,omitempty"`
	OpensAt       *time.Time      `json:"opensAt,omitempty"`
	EndedAt       *time.Time      `json:"endedAt,omitempty"`
	LastReportAt  *time.Time      `json:"lastReportAt,omitempty"`
	StopRequested bool            `json:"stopRequested"`
	OutageOnAt    *time.Time      `json:"outageOnAt,omitempty"`
	OutageOffAt   *time.Time      `json:"outageOffAt,omitempty"`
	Report        json.RawMessage `json:"report,omitempty"`
	Summary       json.RawMessage `json:"summary,omitempty"`
	Error         *string         `json:"error,omitempty"`
}

// Active reports whether the run is not over.
func (r *Run) Active() bool {
	for _, s := range ActiveStates {
		if r.State == s {
			return true
		}
	}
	return false
}

// Service holds rehearsals and runs in the core (design D-1, D-2).
type Service struct {
	Pool  *pgxpool.Pool
	Redis *redis.Client
	Sales *sales.Service
	Snaps interface {
		Load(ctx context.Context, eventID string) (*snapshot.Snapshot, error)
	}
	// Knobs are the providers' admin APIs by provider id ("a", "b").
	Knobs  map[string]*payments.Knobs
	Limits Limits
	// Production applies the shared sample's rules (EPS_ENV=prod).
	Production bool
	Log        *slog.Logger
	Now        func() time.Time
	// Usage reads a rehearsal's paid orders and attempts for the summary (nil: none).
	Facts func(ctx context.Context, org, event, sale string) (Facts, error)
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

var (
	runsStarted = metrics.NewCounter("eps_sim_runs_total", "Simulation runs started.")
	reports     = metrics.NewCounter("eps_sim_reports_total", "Simulator reports received.")
	runsLost    = metrics.NewCounter("eps_sim_runs_lost_total", "Simulation runs ended because the simulator stopped reporting.")
)

const runCols = `id::text, organiser_id::text, sale_id::text, event_id::text, state, knobs, created_at, claimed_at, opens_at, ended_at, last_report_at,
	stop_requested, outage_on_at, outage_off_at, report, summary, error`

func scanRun(row pgx.Row) (*Run, error) {
	var r Run
	var knobs, report, summary []byte
	err := row.Scan(&r.ID, &r.OrganiserID, &r.SaleID, &r.EventID, &r.State, &knobs, &r.CreatedAt, &r.ClaimedAt, &r.OpensAt, &r.EndedAt, &r.LastReportAt,
		&r.StopRequested, &r.OutageOnAt, &r.OutageOffAt, &report, &summary, &r.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, runNotFound()
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(knobs, &r.Knobs)
	if len(report) > 0 {
		r.Report = report
	}
	if len(summary) > 0 {
		r.Summary = summary
	}
	return &r, nil
}

func runNotFound() *apperr.Problem {
	return apperr.New(http.StatusNotFound, "EPS-SIM-NOT-FOUND", "Run not found.", false)
}

// Get reads a run of the organiser.
func (s *Service) Get(ctx context.Context, org, id string) (*Run, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, runNotFound()
	}
	var r *Run
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		r, err = scanRun(tx.QueryRow(ctx, `select `+runCols+` from sales.simulation_runs where id = $1`, id))
		return err
	})
	return r, err
}

// List returns the latest runs of a sale (and of the rehearsals it replaced), newest first.
func (s *Service) List(ctx context.Context, org, saleID string, limit int) ([]Run, error) {
	var out []Run
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select `+runCols+` from sales.simulation_runs where sale_id = $1 order by created_at desc limit $2`, saleID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRun(rows)
			if err != nil {
				return err
			}
			out = append(out, *r)
		}
		return rows.Err()
	})
	return out, err
}

// liveVersion is the source event's live snapshot version (0 when it is not on sale).
func (s *Service) liveVersion(ctx context.Context, event string) (int, error) {
	if s.Snaps == nil {
		return 0, nil
	}
	snap, err := s.Snaps.Load(ctx, event)
	if errors.Is(err, snapshot.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return snap.Version, nil
}

// CreateRehearsal copies a real sale moment into a rehearsal (design D-1).
func (s *Service) CreateRehearsal(ctx context.Context, org, saleID, by string) (*sales.Sale, error) {
	src, _, err := s.Sales.Get(ctx, org, saleID)
	if err != nil {
		return nil, err
	}
	v, err := s.liveVersion(ctx, src.EventID)
	if err != nil {
		return nil, err
	}
	return s.Sales.CreateRehearsal(ctx, org, saleID, by, v)
}

// ResetRehearsal replaces a rehearsal with a fresh one; refused while a run is active on it.
func (s *Service) ResetRehearsal(ctx context.Context, org, rehearsalID, by string) (*sales.Sale, error) {
	cur, _, err := s.Sales.Get(ctx, org, rehearsalID)
	if err != nil {
		return nil, err
	}
	if cur.Rehearsal == nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "Only a rehearsal can be reset.", false).With("field", "sale")
	}
	runs, err := s.List(ctx, org, rehearsalID, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) > 0 && runs[0].Active() {
		return nil, apperr.New(http.StatusConflict, apperr.SimActive, "Stop the run before resetting its rehearsal.", false).With("runId", runs[0].ID)
	}
	v, err := s.liveVersion(ctx, cur.Rehearsal.SourceEventID)
	if err != nil {
		return nil, err
	}
	return s.Sales.ResetRehearsal(ctx, org, rehearsalID, by, v)
}

// activeAnywhere lists the active runs of every organiser (the one-per-environment rule).
func activeAnywhere(ctx context.Context, tx pgx.Tx) ([]Run, error) {
	rows, err := tx.Query(ctx, `select id::text, organiser_id::text, sale_id::text, event_id::text, state, last_report_at, created_at
		from sales.simulation_runs_in($1, 100)`, ActiveStates)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.OrganiserID, &r.SaleID, &r.EventID, &r.State, &r.LastReportAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Start validates the knobs and creates a pending run on a rehearsal (design D-2). A rehearsal that
// already had a run is replaced by a fresh one first, so every run starts with full stock and an
// empty queue; the run is then on the fresh rehearsal (its sale id is in the answer).
func (s *Service) Start(ctx context.Context, org, saleID, by string, k Knobs) (*Run, error) {
	k = k.Defaults()
	if p := k.Validate(s.Limits); p != nil {
		return nil, p
	}
	sale, _, err := s.Sales.Get(ctx, org, saleID)
	if err != nil {
		return nil, err
	}
	if sale.Rehearsal == nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "A run needs a rehearsal: create one from this sale moment first.", false).With("field", "sale")
	}
	// Every rule is checked before the rehearsal is touched (review D-e).
	if p := s.sampleRule(ctx, sale, k); p != nil {
		return nil, p
	}
	if err := s.preflight(ctx, org); err != nil {
		return nil, err
	}
	prev, err := s.List(ctx, org, sale.ID, 1)
	if err != nil {
		return nil, err
	}
	if len(prev) > 0 {
		if prev[0].Active() {
			return nil, apperr.New(http.StatusConflict, apperr.SimActive, "A run is already active on this rehearsal.", false).With("runId", prev[0].ID)
		}
		if sale, err = s.ResetRehearsal(ctx, org, sale.ID, by); err != nil {
			return nil, err
		}
	}
	if sale.Degraded {
		return nil, apperr.New(http.StatusConflict, apperr.SimInvalid, "Turn degraded mode off on the rehearsal first.", false).With("field", "degraded")
	}
	id := uuid.NewString()
	kb, _ := json.Marshal(k)
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		// One active run in the whole environment (a shared cluster), decided under one lock.
		if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext('eps-simulation-runs'))`); err != nil {
			return err
		}
		active, err := activeAnywhere(ctx, tx)
		if err != nil {
			return err
		}
		for _, a := range active {
			if a.OrganiserID == org {
				return apperr.New(http.StatusConflict, apperr.SimActive, "One of your rehearsals is already running.", false).With("runId", a.ID).With("saleId", a.SaleID)
			}
		}
		if len(active) > 0 {
			return apperr.New(http.StatusConflict, apperr.SimBusy, "Another rehearsal is running on this platform. Try again in a few minutes.", true)
		}
		if _, err := tx.Exec(ctx, `insert into sales.simulation_runs (id, organiser_id, sale_id, event_id, state, knobs, created_by) values ($1, $2, $3, $4, 'pending', $5, $6)`,
			id, org, sale.ID, sale.EventID, kb, by); err != nil {
			return err
		}
		_, err = outbox.Write(ctx, tx, outbox.Event{Topic: "simulations", Type: "simulation.created", OrganiserID: org, EventID: sale.EventID, Payload: map[string]any{"runId": id, "saleId": sale.ID}})
		return err
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.ConstraintName == "simulation_runs_one_active" {
		return nil, apperr.New(http.StatusConflict, apperr.SimActive, "One of your rehearsals is already running.", false)
	}
	if err != nil {
		return nil, err
	}
	runsStarted.Inc()
	s.log().Info("simulation run requested", "organiser", org, "by", by, "run", id, "sale", sale.ID, "fans", k.Fans, "botShare", *k.BotShare, "providerA", k.ProviderA.Mode)
	s.watch(ctx, sale.ID)
	return s.Get(ctx, org, id)
}

// Stop asks the simulator to end a run; a run not yet claimed stops at once.
func (s *Service) Stop(ctx context.Context, org, id string, by ...string) (*Run, error) {
	who := ""
	if len(by) > 0 {
		who = by[0]
	}
	s.log().Info("simulation run stop requested", "organiser", org, "by", who, "run", id)
	r, err := s.Get(ctx, org, id)
	if err != nil {
		return nil, err
	}
	if !r.Active() {
		return r, nil
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update sales.simulation_runs set stop_requested = true,
			state = case when state = 'pending' then 'stopped' else state end,
			ended_at = case when state = 'pending' then now() else ended_at end
			where id = $1`, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, org, id)
}

// DegradeAllowed: in production the shared sample's real sale is never switched to degraded mode
// (anyone can sign in to it); its rehearsals can be (review D-b).
func (s *Service) DegradeAllowed(ctx context.Context, sale *sales.Sale) *apperr.Problem {
	if !s.Production || sale.Rehearsal != nil || s.Snaps == nil {
		return nil
	}
	if snap, err := s.Snaps.Load(ctx, sale.EventID); err == nil && snap.IsSample {
		return apperr.New(http.StatusForbidden, apperr.Forbidden, "The shared sample's real sale cannot be switched to degraded mode. Use a rehearsal.", false)
	}
	return nil
}

// RunsPerHour bounds the runs one organiser starts in an hour (review D-b).
const RunsPerHour = 3

// sampleRule: in production the shared sample organiser runs only at the default size (review D-b).
func (s *Service) sampleRule(ctx context.Context, sale *sales.Sale, k Knobs) *apperr.Problem {
	if !s.Production || s.Snaps == nil {
		return nil
	}
	snap, err := s.Snaps.Load(ctx, sale.EventID)
	if err != nil || !snap.IsSample {
		return nil
	}
	d := Knobs{}.Defaults()
	if k.Fans > d.Fans || k.DurationSeconds > d.DurationSeconds || *k.BotShare > *d.BotShare {
		return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "The shared sample runs at the default size only (1,000 fans, 10% bots, 5 minutes).", false).
			With("field", "fans").With("min", 10).With("max", d.Fans)
	}
	return nil
}

// preflight refuses a run while the organiser started too many this hour, while any run is active
// in the environment, or while a real sale here has fans in its waiting room (review D-b).
func (s *Service) preflight(ctx context.Context, org string) error {
	var recent int
	if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select count(*) from sales.simulation_runs where created_at > now() - interval '1 hour'`).Scan(&recent)
	}); err != nil {
		return err
	}
	if recent >= RunsPerHour {
		return apperr.New(http.StatusTooManyRequests, apperr.RateLimited, "At most 3 rehearsal runs an hour per organiser.", true).With("max", RunsPerHour)
	}
	var near []Run
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		active, err := activeAnywhere(ctx, tx)
		if err != nil {
			return err
		}
		for _, a := range active {
			if a.OrganiserID == org {
				return apperr.New(http.StatusConflict, apperr.SimActive, "One of your rehearsals is already running.", false).With("runId", a.ID).With("saleId", a.SaleID)
			}
		}
		if len(active) > 0 {
			return apperr.New(http.StatusConflict, apperr.SimBusy, "Another rehearsal is running on this platform. Try again in a few minutes.", true)
		}
		rows, err := tx.Query(ctx, `select organiser_id::text, event_id::text, sale_id::text, opens_at from sales.real_sales_near(now(), 200)`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Run
			var at time.Time
			if err := rows.Scan(&r.OrganiserID, &r.EventID, &r.SaleID, &at); err != nil {
				return err
			}
			r.OpensAt = &at
			near = append(near, r)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	now := s.now()
	for _, r := range near {
		busy := now.Before(*r.OpensAt) // its pre-queue is open
		if !busy && s.Redis != nil {
			if c, err := queue.CountsOf(ctx, s.Redis, r.OrganiserID, r.EventID, r.SaleID, now); err == nil {
				busy = c.State == "shuffling" || c.Prequeue+c.Line+c.Slow+c.Active > 0
			}
		}
		if busy {
			return apperr.New(http.StatusConflict, apperr.SimBusy, "A real sale on this platform has fans in its waiting room. Rehearse when it is over.", true).With("reason", "real_sale_live")
		}
	}
	return nil
}

// watch adds a sale to the aggregator's watched set for a while (design D-6).
func (s *Service) watch(ctx context.Context, saleID string) {
	if s.Redis != nil {
		_ = live.Watch(ctx, s.Redis, saleID, 2*time.Minute)
	}
}

// merchantKnobs sets (or, with nil, resets) a rehearsal's knobs at one provider.
func (s *Service) merchantKnobs(ctx context.Context, provider, org, event string, k map[string]any) {
	kc := s.Knobs[provider]
	if kc == nil {
		return
	}
	body := map[string]any{"latency_ms": 150, "latency_jitter_ms": 100, "outage_mode": "503"}
	for key, v := range k {
		body[key] = v
	}
	b, _ := json.Marshal(body)
	if _, err := kc.Put(ctx, payments.MerchantOf(payments.Scope(org, event)), b); err != nil {
		s.log().Warn("rehearsal knobs not set", "provider", provider, "event", event, "error", err.Error())
	}
}

// restoreKnobs puts both providers back to the defaults for the rehearsal's merchant.
func (s *Service) restoreKnobs(ctx context.Context, org, event string) {
	for _, p := range []string{"a", "b"} {
		s.merchantKnobs(ctx, p, org, event, nil)
	}
}
