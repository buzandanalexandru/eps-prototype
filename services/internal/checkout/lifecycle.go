package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// AttemptLimit is how long a payment attempt may stay unresolved (D-1); while an attempt is open
// the hold lasts at least this long plus HoldGrace from the payment's start, and never more than
// the hold's own deadline plus both (review M1: extensions are capped).
const (
	AttemptLimit = 120 * time.Second
	HoldGrace    = 30 * time.Second
)

var soldOuts = metrics.NewCounter("eps_holds_sold_out_total", "Holds refused because an instance had no room.")

// Cancel cancels a held or payment-failed order and frees its stock at once.
func (s *Service) Cancel(ctx context.Context, o *Order) (*Order, error) {
	if o.State != Held && o.State != PaymentFailed {
		return nil, stateProblem(o.State)
	}
	var ended *ended
	err := db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		list, err := endTx(ctx, tx, []string{o.ID}, Cancelled, "cancelled by the fan", false)
		if err == nil && len(list) == 1 {
			ended = &list[0]
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if ended == nil {
		cur, _ := s.GetIn(ctx, o.OrganiserID, o.ID)
		if cur != nil {
			return nil, stateProblem(cur.State)
		}
		return nil, notFound()
	}
	// Postgres first, then the stock (review M2): a payment that locked the order first wins.
	if _, err := s.Inv.Release(ctx, o.OrganiserID, ended.EventID, ended.HoldRef); err != nil {
		s.log().Warn("release after cancel failed", "order", o.ID, "error", err.Error())
	}
	return s.GetIn(ctx, o.OrganiserID, o.ID)
}

// Expire ends one hold whose deadline passed (the sweeper works in batches, see Sweeper).
func (s *Service) Expire(ctx context.Context, org, orderID string) (*Order, error) {
	if _, err := s.ExpireBatch(ctx, org, []string{orderID}); err != nil {
		return nil, err
	}
	return s.GetIn(ctx, org, orderID)
}

type ended struct {
	ID, EventID, HoldRef string
}

// endTx moves orders to `to` in one statement: held or payment-failed ones, and for expiry also
// paying ones whose deadline passed with no open attempt. Expiry checks the deadline in the same
// statement, so an order whose payment started meanwhile is left alone. It writes the history,
// the events and cancels pending plans; it returns what it moved.
func endTx(ctx context.Context, tx pgx.Tx, ids []string, to, reason string, deadlineOnly bool) ([]ended, error) {
	cond := "o.state in ('held', 'payment_failed')"
	if deadlineOnly {
		cond = `o.hold_expires_at <= now() and (o.state in ('held', 'payment_failed') or (o.state = 'paying' and not exists
			(select 1 from sales.payment_attempts a where a.order_id = o.id and a.state in ('created', 'submitted', 'challenge_required', 'unknown'))))`
	}
	rows, err := tx.Query(ctx, `with old as (select id, state from sales.orders o where id = any($1::uuid[]) and `+cond+` for update)
		update sales.orders o set state = $2, updated_at = now() from old where o.id = old.id
		returning o.id::text, o.event_id::text, o.hold_ref, old.state, o.snapshot_version, o.total_minor, o.currency`, ids, to)
	if err != nil {
		return nil, err
	}
	var out []ended
	type ev struct {
		id, event, prev, cur string
		v                    int
		total                int64
	}
	var evs []ev
	for rows.Next() {
		var e ended
		var x ev
		if err := rows.Scan(&e.ID, &e.EventID, &e.HoldRef, &x.prev, &x.v, &x.total, &x.cur); err != nil {
			rows.Close()
			return nil, err
		}
		x.id, x.event = e.ID, e.EventID
		out = append(out, e)
		evs = append(evs, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	moved := make([]string, len(out))
	prev := make([]string, len(out))
	payloads := make([]string, len(out))
	events := make([]string, len(out))
	for i, x := range evs {
		moved[i], prev[i], events[i] = x.id, x.prev, x.event
		b, _ := json.Marshal(map[string]any{"orderId": x.id, "eventId": x.event, "snapshotVersion": x.v, "totalMinor": x.total, "currency": x.cur})
		payloads[i] = string(b)
	}
	if _, err := tx.Exec(ctx, `insert into sales.order_history (organiser_id, order_id, from_state, to_state, reason)
		select o.organiser_id, o.id, p.prev, $3, $4 from unnest($1::uuid[], $2::text[]) as p(id, prev) join sales.orders o on o.id = p.id`, moved, prev, to, reason); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `insert into sales.outbox (organiser_id, event_id, topic, type, payload)
		select o.organiser_id, p.event::uuid, 'orders', $4, p.payload::jsonb from unnest($1::uuid[], $2::text[], $3::text[]) as p(id, event, payload) join sales.orders o on o.id = p.id`,
		moved, events, payloads, "order."+to); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `update sales.instalment_plans set state = 'cancelled' where order_id = any($1::uuid[]) and state = 'pending'`, moved); err != nil {
		return nil, err
	}
	return out, nil
}

// ExpireBatch expires the organiser's due orders among ids in one transaction, then releases
// their stock in one inventory step per event. It returns how many expired.
func (s *Service) ExpireBatch(ctx context.Context, org string, ids []string) (int, error) {
	var list []ended
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		list, err = endTx(ctx, tx, ids, Expired, "hold deadline passed", true)
		return err
	})
	if err != nil {
		return 0, err
	}
	byEvent := map[string][]string{}
	for _, e := range list {
		byEvent[e.EventID] = append(byEvent[e.EventID], e.HoldRef)
	}
	for ev, refs := range byEvent {
		if err := s.Inv.ReleaseMany(ctx, org, ev, refs); err != nil {
			s.log().Warn("release after expiry failed", "event", ev, "error", err.Error())
		}
	}
	return len(list), nil
}

// BeginPayingTx moves a held or payment-failed order to paying inside the caller's transaction
// (which also records the payment attempt). The hold lasts at least the attempt's limit plus
// HoldGrace, capped at the hold's own deadline plus both. A payment in full resets the order's
// plan and cancels a pending plan (review D-b). It returns the order as locked and the new hold
// deadline; the caller extends the inventory hold after commit.
func (s *Service) BeginPayingTx(ctx context.Context, tx pgx.Tx, orderID, plan string) (*Order, time.Time, error) {
	o, err := LoadTx(ctx, tx, orderID, true)
	if err != nil {
		return nil, time.Time{}, err
	}
	now := s.now()
	if o.State != Held && o.State != PaymentFailed {
		return o, time.Time{}, stateProblem(o.State)
	}
	if !o.HoldExpiresAt.After(now) {
		return o, time.Time{}, apperr.New(http.StatusConflict, apperr.HoldExpired, "The hold ended and the tickets were released.", false)
	}
	deadline := o.HoldExpiresAt
	if min := now.Add(AttemptLimit + HoldGrace); deadline.Before(min) {
		deadline = min
	}
	if max := o.HoldBaseExpiresAt.Add(AttemptLimit + HoldGrace); deadline.After(max) {
		deadline = max
	}
	deadline = deadline.Truncate(time.Millisecond)
	_, moved, err := Transition(ctx, tx, orderID, Paying, "payment started", ", hold_expires_at = $4, last_error_code = null", deadline)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !moved {
		return o, time.Time{}, stateProblem(o.State)
	}
	if plan != "instalments" {
		if _, err := tx.Exec(ctx, `update sales.orders set pay_plan = 'full' where id = $1`, orderID); err != nil {
			return nil, time.Time{}, err
		}
		if _, err := tx.Exec(ctx, `update sales.instalment_plans set state = 'cancelled' where order_id = $1 and state = 'pending'`, orderID); err != nil {
			return nil, time.Time{}, err
		}
		o.PayPlan = "full"
	}
	o.State = Paying
	o.HoldExpiresAt = deadline
	return o, deadline, nil
}

// ExtendHold moves the inventory hold to the order's deadline (after BeginPayingTx commits).
func (s *Service) ExtendHold(ctx context.Context, o *Order, deadline time.Time) {
	if err := s.Inv.Extend(ctx, o.OrganiserID, o.EventID, o.HoldRef, deadline); err != nil && !errors.Is(err, ErrNoHold) {
		s.log().Warn("hold extend failed", "order", o.ID, "error", err.Error())
	}
}

// FailPayingTx moves a paying order to payment_failed inside tx. Its hold goes back to its own
// deadline, but leaves at least HoldGrace to try again, within the extension cap (review M1).
func (s *Service) FailPayingTx(ctx context.Context, tx pgx.Tx, orderID, code string) error {
	_, _, err := Transition(ctx, tx, orderID, PaymentFailed, "payment failed: "+code,
		`, last_error_code = $4, hold_expires_at = least(o.hold_base_expires_at + $5::interval, greatest(o.hold_base_expires_at, now() + $6::interval))`,
		code, secs(AttemptLimit+HoldGrace), secs(HoldGrace))
	return err
}

// Outcome is what happened to an order when its payment succeeded.
type Outcome struct {
	Paid        bool // the order is paid (now or already)
	AlreadyPaid bool // it was paid before this success (a duplicate)
	Refund      bool // its stock could not be kept: the payment must be refunded (order refunded)
	Order       *Order
}

// capsFor is the live snapshot's capacity for each instance an order draws on. An instance the
// live version no longer has (or an event off sale) is not capped (review L2): an in-flight
// payment is still completed.
func (s *Service) capsFor(ctx context.Context, eventID string, draws map[string]int64) (map[string]int64, error) {
	out := map[string]int64{}
	live, err := s.Snaps.Load(ctx, eventID)
	if err != nil && !errors.Is(err, snapshot.ErrNotFound) {
		return nil, err
	}
	var caps map[string]int64
	if live != nil {
		caps = Capacities(live)
	}
	for k := range draws {
		if c, ok := caps[k]; ok {
			out[k] = c
		} else {
			out[k] = 1 << 50
		}
	}
	return out, nil
}

// takeStock commits an order's stock: its own hold, or (when that hold ended) the same stock taken
// again under the deterministic re-hold <order>_r<ref>. It returns the hold that now carries it.
func (s *Service) takeStock(ctx context.Context, o *Order, ref string) (string, error) {
	caps, err := s.capsFor(ctx, o.EventID, o.Draws)
	if err != nil {
		return "", err
	}
	err = s.Inv.Commit(ctx, o.OrganiserID, o.EventID, o.HoldRef, caps)
	if !errors.Is(err, ErrNoHold) {
		return o.HoldRef, err
	}
	re := o.ID + "_r" + shortRef(ref)
	h := Hold{OrganiserID: o.OrganiserID, EventID: o.EventID, ID: re, Draws: o.Draws, Deadline: s.now().Add(time.Minute)}
	if err := s.reserve(ctx, h, caps); err != nil {
		return "", err
	}
	if err := s.Inv.Commit(ctx, o.OrganiserID, o.EventID, re, caps); err != nil {
		_, _ = s.Inv.Release(ctx, o.OrganiserID, o.EventID, re)
		return "", err
	}
	return re, nil
}

func secs(d time.Duration) string { return strconv.Itoa(int(d.Seconds())) + " seconds" }

func shortRef(ref string) string {
	out := make([]byte, 0, len(ref))
	for i := 0; i < len(ref) && len(out) < 36; i++ {
		c := ref[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "x"
	}
	return string(out)
}

// Complete is the success path of a provider payment: under the order's row lock (review H1) it
// commits the stock and marks the order paid, running also(tx) in the same transaction (the
// attempt's own state, the plan). A late success on an expired or cancelled order takes the same
// stock again under the deterministic re-hold <order>_r<ref> (ref: the attempt id) if it is still
// free; otherwise the order is marked refunded and Outcome.Refund asks the caller to refund.
func (s *Service) Complete(ctx context.Context, org, orderID, ref, reason string, also func(tx pgx.Tx, o *Order) error) (Outcome, error) {
	var out Outcome
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		o, err := LoadTx(ctx, tx, orderID, true)
		if err != nil {
			return err
		}
		switch o.State {
		case Paid, Issued:
			out = Outcome{Paid: true, AlreadyPaid: true}
			return nil
		case Refunded:
			out = Outcome{Refund: true}
			return nil
		}
		holdRef, err := s.takeStock(ctx, o, ref)
		var short *ErrShort
		if errors.As(err, &short) || errors.Is(err, ErrNoHold) {
			soldOuts.Inc()
			_, moved, err := Transition(ctx, tx, orderID, Refunded, "paid after the hold ended; stock gone", ", last_error_code = $4", apperr.HoldExpired)
			if err != nil {
				return err
			}
			if moved {
				if err := emit(ctx, tx, o, "order.refunded", map[string]any{"reason": apperr.HoldExpired}); err != nil {
					return err
				}
			}
			out = Outcome{Refund: true}
			return nil
		}
		if err != nil {
			return err
		}
		if o.State == Held || o.State == PaymentFailed {
			if _, _, err := Transition(ctx, tx, orderID, Paying, reason, ""); err != nil {
				return err
			}
		}
		_, moved, err := Transition(ctx, tx, orderID, Paid, reason, ", paid_at = now(), hold_ref = $4, last_error_code = null", holdRef)
		if err != nil {
			return err
		}
		if !moved {
			return errors.New("checkout: order changed state during payment")
		}
		o.HoldRef = holdRef
		if also != nil {
			if err := also(tx, o); err != nil {
				return err
			}
		}
		out = Outcome{Paid: true}
		return emit(ctx, tx, o, "order.paid", map[string]any{"reason": reason})
	})
	if err != nil {
		return Outcome{}, err
	}
	out.Order, err = s.GetIn(ctx, org, orderID)
	return out, err
}

// MarkPaidTx marks a held or payment-failed order paid inside the caller's transaction without
// touching the stock (the wallet pays this way; CommitStock follows after commit, review M4).
func (s *Service) MarkPaidTx(ctx context.Context, tx pgx.Tx, orderID, reason string) (*Order, error) {
	o, err := LoadTx(ctx, tx, orderID, true)
	if err != nil {
		return nil, err
	}
	if o.State != Held && o.State != PaymentFailed {
		return o, stateProblem(o.State)
	}
	if !o.HoldExpiresAt.After(s.now()) {
		return o, apperr.New(http.StatusConflict, apperr.HoldExpired, "The hold ended and the tickets were released.", false)
	}
	if _, _, err := Transition(ctx, tx, orderID, Paying, reason, ""); err != nil {
		return nil, err
	}
	if _, moved, err := Transition(ctx, tx, orderID, Paid, reason, ", paid_at = now(), last_error_code = null"); err != nil || !moved {
		if err == nil {
			err = errors.New("checkout: order changed state during payment")
		}
		return nil, err
	}
	if err := emit(ctx, tx, o, "order.paid", map[string]any{"reason": reason}); err != nil {
		return nil, err
	}
	o.State = Paid
	return o, nil
}

// CommitStock commits a paid order's stock (after MarkPaidTx committed). The stock may be taken
// again under a re-hold if the order's hold ended meanwhile; the order then records it.
func (s *Service) CommitStock(ctx context.Context, o *Order, ref string) error {
	holdRef, err := s.takeStock(ctx, o, ref)
	if err != nil {
		var short *ErrShort
		if errors.As(err, &short) || errors.Is(err, ErrNoHold) {
			return apperr.New(http.StatusConflict, apperr.HoldSoldOut, "Not enough tickets left.", false)
		}
		return err
	}
	if holdRef == o.HoldRef {
		return nil
	}
	return db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update sales.orders set hold_ref = $2 where id = $1", o.ID, holdRef)
		return err
	})
}

// RefundPaidTx undoes a paid order whose stock could not be committed (the wallet's compensation).
func RefundPaidTx(ctx context.Context, tx pgx.Tx, o *Order, code string) error {
	_, moved, err := Transition(ctx, tx, o.ID, Refunded, "stock could not be committed", ", last_error_code = $4", code)
	if err != nil || !moved {
		return err
	}
	return emit(ctx, tx, o, "order.refunded", map[string]any{"reason": code})
}

// Issue marks a paid order issued (the stub issuer, driven by order.paid, until step 7).
func (s *Service) Issue(ctx context.Context, org, orderID string) error {
	return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		o, err := LoadTx(ctx, tx, orderID, true)
		if err != nil {
			return err
		}
		_, moved, err := Transition(ctx, tx, orderID, Issued, "tickets issued (step 4 stub)", "")
		if err != nil || !moved {
			return err
		}
		return emit(ctx, tx, o, "order.issued", nil)
	})
}
