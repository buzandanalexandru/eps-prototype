package payments

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
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
	// Parallel bounds the provider calls in flight (default 8, review H3).
	Parallel int
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
	// Attempts failed as provider_unavailable are asked about once more by key (review H3).
	var recheck []ref
	_ = db.WithTx(ctx, r.Svc.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, attempt_id::text from sales.recheck_attempts(200)")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x ref
			if err := rows.Scan(&x.org, &x.id); err != nil {
				return err
			}
			recheck = append(recheck, x)
		}
		return rows.Err()
	})
	par := r.Parallel
	if par <= 0 {
		par = 8
	}
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	var changed atomic.Int64
	run := func(f func() (bool, error), id string) {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ok, err := f()
			if err != nil && r.Log != nil {
				r.Log.Warn("reconcile failed", "attempt", id, "error", err.Error())
			}
			if ok {
				changed.Add(1)
			}
		}()
	}
	for _, x := range list {
		run(func() (bool, error) { return r.one(ctx, x.org, x.id) }, x.id)
	}
	for _, x := range recheck {
		run(func() (bool, error) { return r.recheck(ctx, x.org, x.id) }, x.id)
	}
	wg.Wait()
	return int(changed.Load()), nil
}

// recheck asks the provider once about an attempt that failed before acceptance: a charge found
// after all is applied (it pays the order, or is refunded as a duplicate).
func (r *Reconciler) recheck(ctx context.Context, org, id string) (bool, error) {
	s := r.Svc
	a, err := s.GetAttempt(ctx, org, id)
	if err != nil {
		return false, err
	}
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update sales.payment_attempts set rechecked = true where id = $1", id)
		return err
	})
	pv, ok := s.Providers[a.Provider]
	if !ok {
		return false, nil
	}
	ch, err := pv.FindByKey(ctx, a.Merchant(), a.ProviderKey)
	if err != nil {
		return false, nil // no such charge (as expected) or no answer
	}
	return true, s.Apply(ctx, org, id, ch)
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
	// Past its limit while the provider's breaker is open: no call, the attempt is failed (a
	// late success still pays or is refunded).
	if s.now().After(a.DeadlineAt) && s.Breaker.Read(ctx, a.Scope(), a.Provider).State == Open {
		err := s.expireAttempt(ctx, a, "timeout")
		return err == nil, err
	}
	if s.Breaker.Read(ctx, a.Scope(), a.Provider).State == Open {
		return false, nil // asked again once the provider is back
	}
	var ch Charge
	if a.ChargeID != nil {
		ch, err = pv.Get(ctx, *a.ChargeID)
	} else {
		ch, err = pv.FindByKey(ctx, a.Merchant(), a.ProviderKey)
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
		moved, err := moveAttempt(ctx, tx, a.ID, Failed, []string{Created, Submitted, ChallengeRequired, Unknown}, ", error_code = $4, failure_reason = $5", apperr.PayTimeout, reason)
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
