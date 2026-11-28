package onsale

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Lifecycle ends passes and cap entries from the order events (the worker's "onsale" consumer): a
// paid order uses its pass (a shopping place frees at once) and its tickets now count from the cap
// ledger; a cancelled, expired or refunded order revokes its pass and gives its cap back.
type Lifecycle struct {
	Checkout *checkout.Service
	Engine   *queue.Engine
	Caps     *botdefence.Caps
}

// Handle applies one order event.
func (l *Lifecycle) Handle(ctx context.Context, e outbox.Envelope) error {
	how := ""
	switch e.Type {
	case "order.paid":
		how = "used"
	case "order.cancelled", "order.expired", "order.refunded":
		how = "revoked"
	default:
		return nil
	}
	var p struct {
		OrderID string `json:"orderId"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.OrderID == "" {
		return nil
	}
	o, err := l.Checkout.GetIn(ctx, e.OrganiserID, p.OrderID)
	if errors.Is(err, checkout.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return l.End(ctx, o, how)
}

// End ends an order's pass and cap entry.
func (l *Lifecycle) End(ctx context.Context, o *checkout.Order, how string) error {
	if o.SaleID != nil && o.PassQID != nil {
		if err := l.Engine.Ended(ctx, o.OrganiserID, o.EventID, *o.SaleID, *o.PassQID, how); err != nil {
			return err
		}
	}
	if o.CapIdentity != nil && l.Caps != nil {
		return l.Caps.Drop(ctx, o.OrganiserID, o.EventID, *o.CapIdentity, o.ID)
	}
	return nil
}

// Audit copies the waiting rooms' audit trail (joins, drawn positions, admissions, the draw's seed)
// from Redis to Postgres in batches: the hot path writes nothing to Postgres per fan.
type Audit struct {
	Redis *redis.Client
	Pool  *pgxpool.Pool
	Log   *slog.Logger
}

func auditIndex() string {
	k, _ := redisx.PlatformKey("queue-sales")
	return k
}

// RunOnce moves up to 5,000 audit entries per sale.
func (a *Audit) RunOnce(ctx context.Context) (int, error) {
	list, err := a.Redis.SMembers(ctx, auditIndex()).Result()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, m := range list {
		p := strings.Split(m, "/")
		if len(p) != 3 {
			continue
		}
		n, err := a.sale(ctx, p[0], p[1], p[2])
		if err != nil && a.Log != nil {
			a.Log.Warn("queue audit failed", "sale", p[2], "error", err.Error())
		}
		total += n
	}
	return total, nil
}

func (a *Audit) sale(ctx context.Context, org, event, sale string) (int, error) {
	k, err := queue.KeysOf(org, event, sale)
	if err != nil {
		return 0, err
	}
	items, err := a.Redis.LRange(ctx, k.Audit(), 0, 4999).Result()
	if err != nil || len(items) == 0 {
		return 0, err
	}
	type entry struct {
		T   string `json:"t"`
		QID string `json:"qid"`
		Who string `json:"who"`
		At  int64  `json:"at"`
		Pos int64  `json:"pos"`
	}
	var joinQ, joinWho []string
	var joinAt []time.Time
	var posQ []string
	var posN []int64
	var admQ []string
	var admAt []time.Time
	for _, it := range items {
		var e entry
		if json.Unmarshal([]byte(it), &e) != nil {
			continue
		}
		switch e.T {
		case "join":
			joinQ, joinWho, joinAt = append(joinQ, e.QID), append(joinWho, e.Who), append(joinAt, time.UnixMilli(e.At))
		case "pos":
			posQ, posN = append(posQ, e.QID), append(posN, e.Pos)
		case "admit":
			admQ, admAt = append(admQ, e.QID), append(admAt, time.UnixMilli(e.At))
		}
	}
	seed, _ := a.Redis.HGetAll(ctx, k.State()).Result()
	err = db.WithTx(ctx, a.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		if len(joinQ) > 0 {
			if _, err := tx.Exec(ctx, `insert into sales.queue_entries (organiser_id, sale_id, qid, identity_hash, joined_at)
				select $1, $2, q, w, t from unnest($3::text[], $4::text[], $5::timestamptz[]) as x(q, w, t) on conflict do nothing`, org, sale, joinQ, joinWho, joinAt); err != nil {
				return err
			}
		}
		if len(posQ) > 0 {
			if _, err := tx.Exec(ctx, `update sales.queue_entries e set position = x.p from unnest($2::text[], $3::bigint[]) as x(q, p)
				where e.sale_id = $1 and e.qid = x.q`, sale, posQ, posN); err != nil {
				return err
			}
		}
		if len(admQ) > 0 {
			if _, err := tx.Exec(ctx, `update sales.queue_entries e set admitted_at = x.t from unnest($2::text[], $3::timestamptz[]) as x(q, t)
				where e.sale_id = $1 and e.qid = x.q`, sale, admQ, admAt); err != nil {
				return err
			}
		}
		if s := seed["seed"]; s != "" {
			n, _ := strconv.Atoi(seed["n"])
			at, _ := strconv.ParseInt(seed["at"], 10, 64)
			if _, err := tx.Exec(ctx, `update sales.sales set shuffle_seed = $2, shuffled_count = $3, shuffled_at = $4
				where id = $1 and shuffle_seed is null`, sale, s, n, time.UnixMilli(at)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	// Only what was written is removed (entries pushed meanwhile stay for the next round).
	return len(items), a.Redis.LTrim(ctx, k.Audit(), int64(len(items)), -1).Err()
}

// Run moves the audit every interval.
func (a *Audit) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = a.RunOnce(ctx)
		}
	}
}
