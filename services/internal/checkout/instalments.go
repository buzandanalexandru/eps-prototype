package checkout

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/pricing"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Offer is the instalment plan a buyer can choose for an order (D-10): a deposit now and the
// monthly payments that still fit before the event, capped per buyer from today.
type Offer struct {
	DepositMinor int64             `json:"depositMinor"`
	Payments     []pricing.Payment `json:"payments"`
	TotalMinor   int64             `json:"totalMinor"`
}

// GraceDays is how long an overdue part may be paid late.
const GraceDays = 7

// Offer computes the plan for an order, or nil when its products do not all allow instalments
// (or no monthly payment fits before the event any more).
func (s *Service) Offer(ctx context.Context, o *Order) (*Offer, error) {
	snap, err := s.Snaps.LoadVersion(ctx, o.EventID, o.SnapshotVersion)
	if err != nil {
		return nil, err
	}
	return OfferFor(snap, o, s.now()), nil
}

// OfferFor is Offer on a known snapshot and time.
func OfferFor(snap *snapshot.Snapshot, o *Order, now time.Time) *Offer {
	if len(o.Lines) == 0 {
		return nil
	}
	var deposit int64
	count, day := 0, 0
	for _, l := range o.Lines {
		p := snap.Product(l.ProductID)
		if p == nil || p.Instalments == nil {
			return nil
		}
		pl := p.Instalments
		deposit += pricing.DepositFor(pl.DepositMinor, pl.DepositBps, l.UnitAllInMinor) * int64(l.Qty)
		if count == 0 || pl.MonthlyCount < count {
			count = pl.MonthlyCount
		}
		if day == 0 {
			day = pl.DayOfMonth
		}
	}
	starts, err := time.Parse(time.RFC3339Nano, snap.Event.StartsAt)
	if err != nil {
		return nil
	}
	tz := snap.Timezone
	if tz == "" {
		tz = snap.Event.Timezone
	}
	sch, _, err := pricing.InstalmentSchedule(pricing.ScheduleInput{AllInMinor: o.TotalMinor, DepositMinor: deposit, MonthlyCount: count, DayOfMonth: day,
		From: now, EventStartsAt: starts, TimeZone: tz, CapToFit: true})
	if err != nil {
		return nil
	}
	return &Offer{DepositMinor: sch.DepositMinor, Payments: sch.Payments, TotalMinor: sch.TotalMinor}
}

// Part is one scheduled payment of a plan.
type Part struct {
	N           int        `json:"n"`
	DueOn       string     `json:"dueOn"`
	AmountMinor int64      `json:"amountMinor"`
	State       string     `json:"state"`
	GraceUntil  *string    `json:"graceUntil,omitempty"`
	PaidAt      *time.Time `json:"paidAt,omitempty"`
}

// Plan is an order's instalment plan.
type Plan struct {
	ID              string  `json:"id"`
	OrganiserID     string  `json:"-"`
	OrderID         string  `json:"orderId"`
	State           string  `json:"state"`
	DepositMinor    int64   `json:"depositMinor"`
	TotalMinor      int64   `json:"totalMinor"`
	SavedMethodID   *string `json:"-"`
	ClockOffsetDays int     `json:"clockOffsetDays"`
	Parts           []Part  `json:"parts"`
}

// CreatePlanTx records a pending plan for an order paying by instalments (its parts are charged
// only once the deposit succeeds and the plan is active).
func CreatePlanTx(ctx context.Context, tx pgx.Tx, o *Order, offer *Offer) (string, error) {
	var id string
	if _, err := tx.Exec(ctx, `delete from sales.instalment_parts where plan_id in (select id from sales.instalment_plans where order_id = $1 and state = 'pending')`, o.ID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `delete from sales.instalment_plans where order_id = $1 and state = 'pending'`, o.ID); err != nil {
		return "", err
	}
	err := tx.QueryRow(ctx, `insert into sales.instalment_plans (organiser_id, order_id, state, deposit_minor, total_minor) values ($1, $2, 'pending', $3, $4) returning id::text`,
		o.OrganiserID, o.ID, offer.DepositMinor, offer.TotalMinor).Scan(&id)
	if err != nil {
		return "", err
	}
	for _, p := range offer.Payments {
		if _, err := tx.Exec(ctx, `insert into sales.instalment_parts (organiser_id, plan_id, n, due_on, amount_minor, state) values ($1, $2, $3, $4, $5, 'scheduled')`,
			o.OrganiserID, id, p.N, p.DueOn, p.AmountMinor); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `update sales.orders set pay_plan = 'instalments' where id = $1`, o.ID); err != nil {
		return "", err
	}
	return id, nil
}

// ActivatePlanTx makes a pending plan active with the card the provider saved for it.
func ActivatePlanTx(ctx context.Context, tx pgx.Tx, orderID, savedMethodID string) error {
	var sm *string
	if savedMethodID != "" {
		sm = &savedMethodID
	}
	_, err := tx.Exec(ctx, `update sales.instalment_plans set state = 'active', saved_method_id = coalesce($2, saved_method_id) where order_id = $1 and state = 'pending'`, orderID, sm)
	return err
}

// PlanOf reads an order's plan (nil when it pays in full).
func (s *Service) PlanOf(ctx context.Context, org, orderID string) (*Plan, error) {
	var p *Plan
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		p, err = planTx(ctx, tx, "order_id = $1", orderID)
		return err
	})
	return p, err
}

func planTx(ctx context.Context, tx pgx.Tx, where string, arg any) (*Plan, error) {
	var p Plan
	err := tx.QueryRow(ctx, `select id::text, organiser_id::text, order_id::text, state, deposit_minor, total_minor, saved_method_id::text, clock_offset_days
		from sales.instalment_plans where `+where+` order by created_at desc limit 1`, arg).
		Scan(&p.ID, &p.OrganiserID, &p.OrderID, &p.State, &p.DepositMinor, &p.TotalMinor, &p.SavedMethodID, &p.ClockOffsetDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `select n, due_on::text, amount_minor, state, grace_until::text, paid_at from sales.instalment_parts where plan_id = $1 order by n`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pt Part
		if err := rows.Scan(&pt.N, &pt.DueOn, &pt.AmountMinor, &pt.State, &pt.GraceUntil, &pt.PaidAt); err != nil {
			return nil, err
		}
		p.Parts = append(p.Parts, pt)
	}
	return &p, rows.Err()
}

// PlanByID reads a plan of a known organiser.
func (s *Service) PlanByID(ctx context.Context, org, planID string) (*Plan, error) {
	var p *Plan
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		p, err = planTx(ctx, tx, "id = $1", planID)
		return err
	})
	return p, err
}

// Today is the local date the instalment clock reads (Europe/Bucharest).
func (s *Service) Today() string { return pricing.LocalDate(s.now(), pricing.DefaultTimeZone) }

// SimulateNextDue moves a plan's clock so its next scheduled part is due today (demo action).
func (s *Service) SimulateNextDue(ctx context.Context, org, orderID string) (*Plan, error) {
	today, _ := time.Parse(time.DateOnly, s.Today())
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		p, err := planTx(ctx, tx, "order_id = $1", orderID)
		if err != nil {
			return err
		}
		if p == nil || p.State != "active" {
			return apperr.New(http.StatusConflict, apperr.OrderState, "This order has no active instalment plan.", false)
		}
		for _, pt := range p.Parts {
			if pt.State != "scheduled" {
				continue
			}
			due, _ := time.Parse(time.DateOnly, pt.DueOn)
			off := int(due.Sub(today).Hours() / 24)
			if off < p.ClockOffsetDays {
				off = p.ClockOffsetDays
			}
			_, err := tx.Exec(ctx, `update sales.instalment_plans set clock_offset_days = $2 where id = $1`, p.ID, off)
			return err
		}
		return apperr.New(http.StatusConflict, apperr.OrderState, "No scheduled payment is left.", false)
	})
	if err != nil {
		return nil, err
	}
	return s.PlanOf(ctx, org, orderID)
}

// PartCharger charges one instalment part with the plan's saved card (the payments service).
type PartCharger interface {
	ChargePart(ctx context.Context, p *Plan, n int) error
}

// InstalmentRunner charges due parts (D-10): parts due on or before the plan's clock date are
// charged with the saved card under the key inst:<plan>:<part>:<try>.
type InstalmentRunner struct {
	Svc     *Service
	Charger PartCharger
	Log     *slog.Logger
}

// RunOnce charges every due part once and returns how many it started.
func (r *InstalmentRunner) RunOnce(ctx context.Context) (int, error) {
	type due struct {
		org, plan string
		n         int
	}
	var list []due
	err := db.WithTx(ctx, r.Svc.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, plan_id::text, n from sales.due_instalment_parts($1::date, 200)", r.Svc.Today())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.org, &d.plan, &d.n); err != nil {
				return err
			}
			list = append(list, d)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range list {
		p, err := r.Svc.PlanByID(ctx, d.org, d.plan)
		if err != nil || p == nil {
			continue
		}
		if err := r.Charger.ChargePart(ctx, p, d.n); err != nil {
			if r.Log != nil {
				r.Log.Warn("instalment charge failed", "plan", d.plan, "part", d.n, "error", err.Error())
			}
			continue
		}
		n++
	}
	return n, nil
}

// Run charges due parts every interval until ctx ends.
func (r *InstalmentRunner) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = r.RunOnce(ctx)
		}
	}
}

// PartResultTx records the outcome of a part's charge: paid, or overdue with a 7-day grace period
// (on the plan's clock). A plan whose parts are all paid is completed.
func PartResultTx(ctx context.Context, tx pgx.Tx, org, planID string, n int, ok bool, today string) error {
	if ok {
		if _, err := tx.Exec(ctx, `update sales.instalment_parts set state = 'paid', paid_at = now(), grace_until = null where plan_id = $1 and n = $2`, planID, n); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update sales.instalment_plans set state = 'completed' where id = $1 and not exists
			(select 1 from sales.instalment_parts where plan_id = $1 and state <> 'paid')`, planID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `update sales.instalment_parts p set state = 'overdue',
			grace_until = coalesce(p.grace_until, ($3::date + l.clock_offset_days + $4::int))
			from sales.instalment_plans l where l.id = p.plan_id and p.plan_id = $1 and p.n = $2`, planID, n, today, GraceDays); err != nil {
			return err
		}
	}
	typ := "order.instalment_paid"
	if !ok {
		typ = "order.instalment_overdue"
	}
	_, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: typ, OrganiserID: org, Payload: map[string]any{"planId": planID, "part": n}})
	return err
}
