package checkout

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

var expiredHolds = metrics.NewCounter("eps_holds_expired_total", "Holds released by the sweeper.")

// Sweeper releases holds whose deadline passed (D-7): every second it expires held and
// payment-failed orders past their deadline (paying orders are never expired while their attempt
// is open; the reconciler resolves them), then releases inventory holds no live order owns.
type Sweeper struct {
	Svc *Service
	Log *slog.Logger
}

// RunOnce expires every due order and returns how many it expired.
func (w *Sweeper) RunOnce(ctx context.Context) (int, error) {
	type due struct{ org, id string }
	var list []due
	err := db.WithTx(ctx, w.Svc.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, order_id::text from sales.due_holds(500)")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.org, &d.id); err != nil {
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
		o, err := w.Svc.Expire(ctx, d.org, d.id)
		if err != nil {
			if w.Log != nil {
				w.Log.Debug("sweeper: expire skipped", "order", d.id, "error", err.Error())
			}
			continue
		}
		if o != nil && o.State == Expired {
			n++
			expiredHolds.Inc()
		}
	}
	return n, nil
}

// SweepOrphans releases inventory holds older than `grace` past their deadline whose order is not
// live (a crash between taking stock and recording the order, or a re-hold left behind).
func (w *Sweeper) SweepOrphans(ctx context.Context, grace time.Duration) (int, error) {
	ri, ok := w.Svc.Inv.(*RedisInventory)
	if !ok {
		return 0, nil
	}
	events, err := ri.Events(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ev := range events {
		org, event := ev[0], ev[1]
		ids, err := ri.Due(ctx, org, event, w.Svc.now().Add(-grace), 200)
		if err != nil {
			continue
		}
		for _, hid := range ids {
			orderID, _, _ := strings.Cut(hid, "_")
			o, err := w.Svc.GetIn(ctx, org, orderID)
			live := err == nil && o.HoldRef == hid && (o.State == Held || o.State == Paying || o.State == PaymentFailed)
			if live {
				// The order still holds it: keep the index in step with the order's deadline.
				_ = ri.Extend(ctx, org, event, hid, o.HoldExpiresAt)
				continue
			}
			if err != nil && err != ErrNotFound {
				continue
			}
			if ok, _ := ri.Release(ctx, org, event, hid); ok {
				n++
			} else {
				// Committed or already released: drop it from the index.
				k, _ := ri.key(org, event, "holds-exp")
				_ = ri.Redis.ZRem(ctx, k, hid).Err()
			}
		}
	}
	return n, nil
}

// Run sweeps every interval until ctx ends.
func (w *Sweeper) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if _, err := w.RunOnce(ctx); err != nil && w.Log != nil && ctx.Err() == nil {
			w.Log.Warn("sweeper failed", "error", err.Error())
		}
		tick++
		if tick%30 == 0 {
			_, _ = w.SweepOrphans(ctx, time.Minute)
		}
	}
}
