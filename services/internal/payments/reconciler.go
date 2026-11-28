package payments

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// ReconcileAge is how old an unresolved attempt must be before the reconciler asks (D-5).
const ReconcileAge = 20 * time.Second

// Reconciler resolves attempts nobody reported on: every attempt in created, submitted,
// challenge_required or unknown for 20 seconds is checked with its provider (so a dropped webhook
// still pays the order), refunds still pending are sent again, and an attempt past its 120-second
// limit with no outcome is failed so its order can be paid again or expire.
type Reconciler struct {
	Svc *Service
	Log *slog.Logger
	Age time.Duration
}

// RunOnce checks every due attempt once and returns how many changed.
func (r *Reconciler) RunOnce(ctx context.Context) (int, error) {
	age := r.Age
	if age <= 0 {
		age = ReconcileAge
	}
	type ref struct{ org, id string }
	var list []ref
	err := db.WithTx(ctx, r.Svc.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, attempt_id::text from sales.unresolved_attempts($1::interval, 200)", age.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x ref
			if err := rows.Scan(&x.org, &x.id); err != nil {
				return err
			}
			list = append(list, x)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, x := range list {
		ok, err := r.one(ctx, x.org, x.id)
		if err != nil && r.Log != nil {
			r.Log.Warn("reconcile failed", "attempt", x.id, "error", err.Error())
		}
		if ok {
			changed++
		}
	}
	return changed, nil
}

func (r *Reconciler) one(ctx context.Context, org, id string) (bool, error) {
	s := r.Svc
	a, err := s.GetAttempt(ctx, org, id)
	if err != nil {
		return false, err
	}
	before := a.State
	if a.State == RefundPending {
		err := s.sendRefund(ctx, org, id)
		return err == nil, err
	}
	pv, ok := s.Providers[a.Provider]
	if !ok {
		return false, nil
	}
	var ch Charge
	if a.ChargeID != nil {
		ch, err = pv.Get(ctx, *a.ChargeID)
	} else {
		ch, err = pv.FindByKey(ctx, a.ProviderKey)
	}
	switch {
	case err == nil:
		if err := s.Apply(ctx, org, id, ch); err != nil {
			return false, err
		}
	case errors.Is(err, ErrNoSuchCharge):
		// The provider never took it: failed (another attempt may now be made).
		if err := s.expireAttempt(ctx, a, "no_such_charge"); err != nil {
			return false, err
		}
	default:
		// Provider unreachable: keep asking until the attempt's limit.
	}
	cur, err := s.GetAttempt(ctx, org, id)
	if err != nil {
		return false, err
	}
	if open(cur.State) && s.now().After(cur.DeadlineAt) {
		if err := s.expireAttempt(ctx, cur, "timeout"); err != nil {
			return false, err
		}
	} else if open(cur.State) {
		// Touch it so the next check comes after another ReconcileAge.
		_ = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "update sales.payment_attempts set updated_at = now() where id = $1", id)
			return err
		})
	}
	cur, _ = s.GetAttempt(ctx, org, id)
	return cur != nil && cur.State != before, nil
}

// expireAttempt fails an attempt that has no outcome (a late success still pays or is refunded).
func (s *Service) expireAttempt(ctx context.Context, a *Attempt, reason string) error {
	return db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
		moved, err := moveAttempt(ctx, tx, a.ID, Failed, []string{Created, Submitted, ChallengeRequired, Unknown}, ", error_code = $4", apperr.PayTimeout)
		if err != nil || !moved {
			return err
		}
		switch a.Purpose {
		case "order":
			return s.Checkout.FailPayingTx(ctx, tx, *a.OrderID, apperr.PayTimeout)
		case "instalment":
			return s.partResult(ctx, tx, a, false)
		}
		return nil
	})
}

// Run reconciles every interval until ctx ends.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.RunOnce(ctx); err != nil && r.Log != nil && ctx.Err() == nil {
				r.Log.Warn("reconciler failed", "error", err.Error())
			}
		}
	}
}
