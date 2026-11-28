package checkout

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// AttemptLimit is how long a payment attempt may stay unresolved (D-1); the hold of a paying order
// lasts at least this long plus HoldGrace from the payment's start.
const (
	AttemptLimit = 120 * time.Second
	HoldGrace    = 30 * time.Second
)

// Cancel cancels a held or payment-failed order and frees its stock at once.
func (s *Service) Cancel(ctx context.Context, o *Order) (*Order, error) {
	if o.State != Held && o.State != PaymentFailed {
		return nil, stateProblem(o.State)
	}
	return s.end(ctx, o.OrganiserID, o.ID, Cancelled, "cancelled by the fan")
}

// Expire ends a hold whose deadline passed (the sweeper). The inventory decides a race with a
// payment: a hold that was already committed by a success is left alone.
func (s *Service) Expire(ctx context.Context, org, orderID string) (*Order, error) {
	return s.end(ctx, org, orderID, Expired, "hold deadline passed")
}

func (s *Service) end(ctx context.Context, org, orderID, to, reason string) (*Order, error) {
	o, err := s.GetIn(ctx, org, orderID)
	if err != nil {
		return nil, err
	}
	if !CanMove(o.State, to) {
		return o, stateProblem(o.State)
	}
	if to == Expired && o.HoldExpiresAt.After(s.now()) {
		return o, nil // extended meanwhile (a payment started)
	}
	if _, err := s.Inv.Release(ctx, org, o.EventID, o.HoldRef); err != nil {
		return nil, err
	}
	if committed, _ := s.holdCommitted(ctx, o); committed {
		// A payment won the race: the stock is sold; the payment path marks the order paid.
		return s.GetIn(ctx, org, orderID)
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, moved, err := Transition(ctx, tx, orderID, to, reason, "")
		if err != nil || !moved {
			return err
		}
		if _, err := tx.Exec(ctx, `update sales.instalment_plans set state = 'cancelled' where order_id = $1 and state = 'pending'`, orderID); err != nil {
			return err
		}
		return emit(ctx, tx, o, "order."+to, nil)
	})
	if err != nil {
		return nil, err
	}
	return s.GetIn(ctx, org, orderID)
}

// holdCommitted reports whether the order's inventory hold was turned into sold units.
func (s *Service) holdCommitted(ctx context.Context, o *Order) (bool, error) {
	ri, ok := s.Inv.(*RedisInventory)
	if !ok {
		return false, nil
	}
	k, err := ri.key(o.OrganiserID, o.EventID, "hold", o.HoldRef)
	if err != nil {
		return false, err
	}
	st, err := ri.Redis.HGet(ctx, k, "state").Result()
	if err != nil {
		return false, nil
	}
	return st == "committed", nil
}

// BeginPayingTx moves a held or payment-failed order to paying inside the caller's transaction
// (which also records the payment attempt), extending its hold to end no earlier than the
// attempt's limit plus HoldGrace. It returns the order as locked and the new hold deadline; the
// caller extends the inventory hold after commit.
func (s *Service) BeginPayingTx(ctx context.Context, tx pgx.Tx, orderID string) (*Order, time.Time, error) {
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
		deadline = min.Truncate(time.Millisecond)
	}
	_, moved, err := Transition(ctx, tx, orderID, Paying, "payment started", ", hold_expires_at = $4, last_error_code = null", deadline)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !moved {
		return o, time.Time{}, stateProblem(o.State)
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

// FailPayingTx moves a paying order to payment_failed (its hold keeps running) inside tx.
func (s *Service) FailPayingTx(ctx context.Context, tx pgx.Tx, orderID, code string) error {
	_, _, err := Transition(ctx, tx, orderID, PaymentFailed, "payment failed: "+code, ", last_error_code = $4", code)
	return err
}

// Outcome is what happened to an order when its payment succeeded.
type Outcome struct {
	Paid        bool // the order is paid (now or already)
	AlreadyPaid bool // it was paid before this success (a duplicate)
	Refund      bool // its stock could not be kept: the payment must be refunded (order refunded)
	Order       *Order
}

// capacitiesFor is the live snapshot's capacities, or nil when the event went off sale meanwhile
// (an in-flight payment is still completed then).
func (s *Service) capacitiesFor(ctx context.Context, eventID string) (map[string]int64, error) {
	live, err := s.Snaps.Load(ctx, eventID)
	if errors.Is(err, snapshot.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Capacities(live), nil
}

func unbounded(draws map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for k := range draws {
		out[k] = 1 << 50
	}
	return out
}

// Complete is the success path of a payment (provider confirmed, or the wallet paid): it commits
// the stock and marks the order paid, running also(tx) in the same transaction (the attempt's own
// state, the plan, ledger lines). A late success on an expired or cancelled order re-takes the
// stock if it is still free, otherwise the order is marked refunded and Outcome.Refund asks the
// caller to refund the payment (D-1).
func (s *Service) Complete(ctx context.Context, org, orderID, reason string, also func(tx pgx.Tx, o *Order) error) (Outcome, error) {
	o, err := s.GetIn(ctx, org, orderID)
	if err != nil {
		return Outcome{}, err
	}
	if o.State == Paid || o.State == Issued {
		return Outcome{Paid: true, AlreadyPaid: true, Order: o}, nil
	}
	if o.State == Refunded {
		return Outcome{Refund: true, Order: o}, nil
	}
	caps, err := s.capacitiesFor(ctx, o.EventID)
	if err != nil {
		return Outcome{}, err
	}
	if caps == nil {
		caps = unbounded(o.Draws)
	}
	holdRef := o.HoldRef
	err = s.Inv.Commit(ctx, org, o.EventID, holdRef, caps)
	if errors.Is(err, ErrNoHold) {
		// The hold ended (expired or cancelled): take the same stock again if it is still free.
		holdRef = o.ID + "_r" + strconv.FormatInt(time.Now().UnixNano(), 36)
		h := Hold{OrganiserID: org, EventID: o.EventID, ID: holdRef, Draws: o.Draws, Deadline: s.now().Add(time.Minute)}
		if rerr := s.reserve(ctx, h, caps); rerr != nil {
			err = rerr
		} else {
			err = s.Inv.Commit(ctx, org, o.EventID, holdRef, caps)
			if err != nil {
				_, _ = s.Inv.Release(ctx, org, o.EventID, holdRef)
			}
		}
	}
	var short *ErrShort
	if errors.As(err, &short) || errors.Is(err, ErrNoHold) {
		// The stock is gone: refund (the provider refund is the caller's).
		err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			_, moved, err := Transition(ctx, tx, orderID, Refunded, "paid after the hold ended; stock gone", ", last_error_code = $4", apperr.HoldExpired)
			if err != nil {
				return err
			}
			if moved {
				return emit(ctx, tx, o, "order.refunded", map[string]any{"reason": apperr.HoldExpired})
			}
			return nil
		})
		if err != nil {
			return Outcome{}, err
		}
		o, _ = s.GetIn(ctx, org, orderID)
		return Outcome{Refund: true, Order: o}, nil
	}
	if err != nil {
		return Outcome{}, err
	}
	var already bool
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		cur, err := LoadTx(ctx, tx, orderID, true)
		if err != nil {
			return err
		}
		if cur.State == Paid || cur.State == Issued {
			already = true
			return nil
		}
		if cur.State == Held || cur.State == PaymentFailed {
			// Wallet payments and late successes pass through paying for an honest history.
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
		if also != nil {
			if err := also(tx, cur); err != nil {
				return err
			}
		}
		return emit(ctx, tx, cur, "order.paid", map[string]any{"reason": reason})
	})
	if err != nil {
		return Outcome{}, err
	}
	o, err = s.GetIn(ctx, org, orderID)
	return Outcome{Paid: true, AlreadyPaid: already, Order: o}, err
}

// CommitTx commits an order's stock and marks it paid inside the caller's transaction (the wallet
// pays this way: ledger entry, stock and order in one step). The Redis commit runs before the
// caller's transaction commits; a commit that fails rolls the whole payment back.
func (s *Service) CommitTx(ctx context.Context, tx pgx.Tx, orderID, reason string) (*Order, error) {
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
	caps, err := s.capacitiesFor(ctx, o.EventID)
	if err != nil {
		return nil, err
	}
	if caps == nil {
		caps = unbounded(o.Draws)
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
	if err := s.Inv.Commit(ctx, o.OrganiserID, o.EventID, o.HoldRef, caps); err != nil {
		var short *ErrShort
		if errors.As(err, &short) {
			return nil, apperr.New(http.StatusConflict, apperr.HoldSoldOut, "Not enough tickets left.", false)
		}
		if errors.Is(err, ErrNoHold) {
			return nil, apperr.New(http.StatusConflict, apperr.HoldExpired, "The hold ended and the tickets were released.", false)
		}
		return nil, err
	}
	o.State = Paid
	return o, nil
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
