package checkout

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

var (
	expiredHolds = metrics.NewCounter("eps_holds_expired_total", "Holds released by the sweeper.")
	sweepLagMs   atomic.Int64
)

func init() {
	metrics.NewGauge("eps_sweep_lag_seconds", func() float64 { return float64(sweepLagMs.Load()) / 1000 })
}

// Sweeper releases holds whose deadline passed (D-7, review H2/M2): every second it expires the due
// orders (held or payment-failed, and paying ones whose attempt is no longer open) in one
// transaction per organiser batch, then releases their stock in one inventory step per event,
// with a bounded number of organisers at a time. Orphaned inventory holds are released by
// SweepOrphans.
type Sweeper struct {
	Svc *Service
	Log *slog.Logger
	// Index finds orphaned holds (nil: no orphan sweep).
	Index HoldIndex
	// Parallel is how many organisers are swept at once (default 4).
	Parallel int
}

// RunOnce expires every due order and returns how many it expired.
func (w *Sweeper) RunOnce(ctx context.Context) (int, error) {
	byOrg := map[string][]string{}
	var oldest time.Time
	err := db.WithTx(ctx, w.Svc.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select d.organiser_id::text, d.order_id::text, d.hold_expires_at from sales.due_holds(5000) d`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var org, id string
			var at time.Time
			if err := rows.Scan(&org, &id, &at); err != nil {
				return err
			}
			if oldest.IsZero() || at.Before(oldest) {
				oldest = at
			}
			byOrg[org] = append(byOrg[org], id)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	par := w.Parallel
	if par <= 0 {
		par = 4
	}
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	var total atomic.Int64
	start := time.Now()
	for org, ids := range byOrg {
		for len(ids) > 0 {
			n := min(500, len(ids))
			batch := ids[:n]
			ids = ids[n:]
			wg.Add(1)
			sem <- struct{}{}
			go func(org string, batch []string) {
				defer wg.Done()
				defer func() { <-sem }()
				k, err := w.Svc.ExpireBatch(ctx, org, batch)
				if err != nil && w.Log != nil {
					w.Log.Warn("sweeper: batch failed", "organiser", org, "error", err.Error())
				}
				total.Add(int64(k))
			}(org, batch)
		}
	}
	wg.Wait()
	// How long the oldest due hold waited past its deadline before it was released.
	if !oldest.IsZero() {
		sweepLagMs.Store(time.Since(oldest).Milliseconds())
	} else {
		sweepLagMs.Store(0)
	}
	_ = start
	expiredHolds.Add(total.Load())
	return int(total.Load()), nil
}

// SweepOrphans settles inventory holds older than `grace` past their deadline whose order no
// longer carries them as a live hold: released when the order is not live (a crash between taking
// stock and recording the order, or a re-hold left behind), committed when the order is paid but
// its stock was never committed (the wallet's reconcile, review M4).
func (w *Sweeper) SweepOrphans(ctx context.Context, grace time.Duration) (int, error) {
	if w.Index == nil {
		return 0, nil
	}
	events, err := w.Index.Events(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, ev := range events {
		org, event := ev[0], ev[1]
		ids, err := w.Index.Due(ctx, org, event, w.Svc.now().Add(-grace), 200)
		if err != nil {
			continue
		}
		for _, hid := range ids {
			orderID, _, _ := strings.Cut(hid, "_")
			o, err := w.Svc.GetIn(ctx, org, orderID)
			if err != nil && err != ErrNotFound {
				continue
			}
			switch {
			case err == nil && o.HoldRef == hid && (o.State == Held || o.State == Paying || o.State == PaymentFailed):
				// The order still holds it: keep the index in step with the order's deadline.
				_ = w.Svc.Inv.Extend(ctx, org, event, hid, o.HoldExpiresAt)
				continue
			case err == nil && o.HoldRef == hid && (o.State == Paid || o.State == Issued):
				if st, _ := w.Svc.Inv.HoldState(ctx, org, event, hid); st == "held" {
					if err := w.Svc.CommitStock(ctx, o, "reconcile"); err == nil {
						n++
					}
				}
			default:
				if r, _ := w.Svc.Inv.Release(ctx, org, event, hid); r == ReleaseDone {
					n++
				}
			}
			_ = w.Index.Forget(ctx, org, event, hid)
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
