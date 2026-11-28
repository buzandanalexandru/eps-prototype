package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// StreamMaxLen trims every stream (approximately) to this many entries.
const StreamMaxLen = 1_000_000

// RelayLockID is the advisory lock that keeps relays from running side by side, so each topic's
// events reach the stream in the order they were recorded.
const RelayLockID = 7_340_001

var relayed = metrics.NewCounter("eps_outbox_relayed_total", "Outbox events published to Redis Streams.")

// Relay moves recorded events to Redis Streams (at least once: a crash between publishing and
// marking resends, and consumers drop the repeat by event id).
type Relay struct {
	Pool  *pgxpool.Pool
	Redis *redis.Client
	Log   *slog.Logger
	Batch int // per organiser per round (default 500)
	// AfterPublish, when set, runs after the events are in the stream and before they are marked
	// sent (tests use it to simulate a crash).
	AfterPublish func() error
}

// RunOnce publishes every pending event once and returns how many it published.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	batch := r.Batch
	if batch <= 0 {
		batch = 500
	}
	var orgs []string
	err := db.WithTx(ctx, r.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text from sales.outbox_organisers(1000)")
		if err != nil {
			return err
		}
		orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, org := range orgs {
		for {
			n, err := r.relayOrganiser(ctx, org, batch)
			total += n
			if err != nil {
				return total, err
			}
			if n < batch {
				break
			}
		}
	}
	return total, nil
}

func (r *Relay) relayOrganiser(ctx context.Context, org string, batch int) (int, error) {
	n := 0
	err := db.WithTx(ctx, r.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "select pg_try_advisory_xact_lock($1)", RelayLockID).Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return nil // another relay is publishing; it will take these too
		}
		rows, err := tx.Query(ctx, `select seq, id::text, topic, type, version, organiser_id::text, coalesce(event_id::text, ''), created_at, payload
			from sales.outbox where relayed_at is null order by seq limit $1 for update skip locked`, batch)
		if err != nil {
			return err
		}
		type item struct {
			seq   int64
			topic string
			env   Envelope
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.seq, &it.env.ID, &it.topic, &it.env.Type, &it.env.Version, &it.env.OrganiserID, &it.env.EventID, &it.env.OccurredAt, &it.env.Payload); err != nil {
				rows.Close()
				return err
			}
			it.env.SaleID = it.env.EventID
			items = append(items, it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		pipe := r.Redis.Pipeline()
		seqs := make([]int64, 0, len(items))
		for _, it := range items {
			b, _ := json.Marshal(it.env)
			pipe.XAdd(ctx, &redis.XAddArgs{Stream: StreamKey(it.topic), MaxLen: StreamMaxLen, Approx: true, Values: map[string]any{"id": it.env.ID, "env": string(b)}})
			seqs = append(seqs, it.seq)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
		if r.AfterPublish != nil {
			if err := r.AfterPublish(); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "update sales.outbox set relayed_at = clock_timestamp() where seq = any($1)", seqs); err != nil {
			return err
		}
		// Relayed rows are kept 7 days (D-12).
		if _, err := tx.Exec(ctx, `delete from sales.outbox where seq in (select seq from sales.outbox where relayed_at < now() - interval '7 days' limit 1000)`); err != nil {
			return err
		}
		n = len(items)
		relayed.Add(int64(n))
		return nil
	})
	return n, err
}

// Run relays until ctx ends: on every LISTEN outbox notification and at least every interval.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	wake := make(chan struct{}, 1)
	go r.listen(ctx, wake)
	t := time.NewTicker(interval)
	defer t.Stop()
	for ctx.Err() == nil {
		if _, err := r.RunOnce(ctx); err != nil && ctx.Err() == nil {
			r.logErr("relay: run", err)
		}
		select {
		case <-ctx.Done():
		case <-wake:
		case <-t.C:
		}
	}
}

// listen forwards LISTEN outbox notifications to wake (reconnecting after errors).
func (r *Relay) listen(ctx context.Context, wake chan<- struct{}) {
	for ctx.Err() == nil {
		func() {
			conn, err := r.Pool.Acquire(ctx)
			if err != nil {
				return
			}
			defer conn.Release()
			if _, err := conn.Exec(ctx, "listen outbox"); err != nil {
				return
			}
			for {
				if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
					// The connection may be unusable after an error: drop it from the pool.
					_ = conn.Conn().Close(context.WithoutCancel(ctx))
					return
				}
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func (r *Relay) logErr(msg string, err error) {
	if r.Log != nil {
		r.Log.Warn(msg, "error", err.Error())
	}
}
