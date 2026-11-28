package simulation

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// CleanAfter is how long after its last run a live rehearsal keeps its Redis data (D-1).
const CleanAfter = 2 * time.Hour

var keysDeleted = metrics.NewCounter("eps_rehearsal_keys_deleted_total", "Redis keys of ended rehearsals deleted by the clean-up.")

// Cleaner deletes the Redis data of rehearsals that were reset or deleted, or whose last run ended
// CleanAfter ago (the worker's lease holder, every 30 s). Postgres keeps their orders as a record.
type Cleaner struct {
	Pool  *pgxpool.Pool
	Redis *redis.Client
	Sales *sales.Service
	Log   *slog.Logger
	Now   func() time.Time
	// MaxScanSteps bounds one rehearsal's SCAN (1,000 keys a step).
	MaxScanSteps int
}

// RunOnce cleans up to 10 rehearsals and returns how many keys it deleted.
func (c *Cleaner) RunOnce(ctx context.Context) (int, error) {
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	type rh struct{ org, event, sale string }
	var list []rh
	err := db.WithTx(ctx, c.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select organiser_id::text, event_id::text, sale_id::text from sales.rehearsals_to_clean($1, 10)`, now.Add(-CleanAfter))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x rh
			if err := rows.Scan(&x.org, &x.event, &x.sale); err != nil {
				return err
			}
			list = append(list, x)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, x := range list {
		n, err := DeleteRehearsalKeys(ctx, c.Redis, x.org, x.event, c.MaxScanSteps)
		total += n
		if err != nil {
			return total, err
		}
		if err := c.Sales.MarkCleaned(ctx, x.org, x.sale); err != nil {
			return total, err
		}
		if c.Log != nil {
			c.Log.Info("rehearsal data deleted", "organiser", x.org, "event", x.event, "keys", n)
		}
	}
	return total, nil
}

// DeleteRehearsalKeys deletes every key of a rehearsal event: its hot keys (queue, stock shards,
// holds: tag {org:event:...}) and its organiser keys (sales copy, caps, OTP, breaker:
// eps:{org}:event:...). Only ever called with a rehearsal's own event id.
func DeleteRehearsalKeys(ctx context.Context, rdb *redis.Client, org, event string, maxSteps int) (int, error) {
	if maxSteps <= 0 {
		maxSteps = 5000
	}
	n := 0
	for _, pattern := range []string{"eps:{" + org + ":" + event + ":*", "eps:{" + org + "}:" + event + ":*"} {
		var cursor uint64
		for step := 0; step < maxSteps; step++ {
			keys, next, err := rdb.Scan(ctx, cursor, pattern, 1000).Result()
			if err != nil {
				return n, err
			}
			if len(keys) > 0 {
				d, err := rdb.Unlink(ctx, keys...).Result()
				if err != nil {
					return n, err
				}
				n += int(d)
				keysDeleted.Add(d)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return n, nil
}
