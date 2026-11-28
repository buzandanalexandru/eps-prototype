package inventory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// TopicInventory carries the reconciler's events (inventory.drift_corrected, inventory.oversell_risk).
const TopicInventory = "inventory"

// ShardState is one shard of one instance as the reconciler reads it.
type ShardState struct {
	Shard                   int
	Alloc, Used, Sold, Held int64
	Halted                  bool
}

// Shards reads every shard of an instance, each with one script (its live entries summed inside).
func (s *Sharded) Shards(ctx context.Context, org, event, instance string) ([]ShardState, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return nil, err
	}
	p := part(instance)
	n, err := s.Redis.HGet(ctx, k.meta(p), "n").Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make([]ShardState, 0, n)
	for i := 0; i < n; i++ {
		v, err := shardSnapshot.Run(ctx, s.Redis, []string{k.ctr(i, p), k.shardExp(i)}, k.entryPrefix(i), p).Int64Slice()
		if err != nil {
			return nil, err
		}
		out = append(out, ShardState{Shard: i, Alloc: v[0], Used: v[1], Sold: v[2], Held: v[3], Halted: v[4] == 1})
	}
	return out, nil
}

// Instances lists the instances of an event that were ever split, with their capacity.
func (s *Sharded) Instances(ctx context.Context, org, event string) (map[string]int64, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return nil, err
	}
	parts, err := s.Redis.SMembers(ctx, k.insts()).Result()
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, p := range parts {
		v, err := s.Redis.HMGet(ctx, k.meta(p), "inst", "cap").Result()
		if err != nil || v[0] == nil {
			continue
		}
		out[fmt.Sprint(v[0])] = num(v[1])
	}
	return out, nil
}

func (s *Sharded) repair(ctx context.Context, org, event, instance string, shard int, mode string, delta int64) (int64, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return 0, err
	}
	return shardRepair.Run(ctx, s.Redis, []string{k.ctr(shard, part(instance))}, mode, delta).Int64()
}

// Halt stops (or allows again) new holds on every shard of an instance.
func (s *Sharded) Halt(ctx context.Context, org, event, instance string, on bool) error {
	sh, err := s.Shards(ctx, org, event, instance)
	if err != nil {
		return err
	}
	v := int64(0)
	if on {
		v = 1
	}
	for _, x := range sh {
		if _, err := s.repair(ctx, org, event, instance, x.Shard, "halt", v); err != nil {
			return err
		}
	}
	return nil
}

// Committed lists holds committed before `before` that the reconciler has not yet seen paid.
func (s *Sharded) Committed(ctx context.Context, org, event string, before time.Time, limit int64) ([]string, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return nil, err
	}
	var out []string
	for h := 0; h < HomeTags && int64(len(out)) < limit; h++ {
		ids, err := s.Redis.ZRangeArgs(ctx, redis.ZRangeArgs{Key: k.homeCommitted("h" + strconv.Itoa(h)), Start: "-inf", Stop: strconv.FormatInt(before.UnixMilli(), 10), ByScore: true, Count: limit - int64(len(out))}).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, ids...)
	}
	return out, nil
}

// Verified drops a committed hold from the reconciler's list (its order is paid).
func (s *Sharded) Verified(ctx context.Context, org, event, holdID string) error {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return err
	}
	return s.Redis.ZRem(ctx, k.homeCommitted(homeOf(holdID)), holdID).Err()
}

var (
	corrections = map[string]*metrics.Counter{}
	reconRounds = metrics.NewCounter("eps_inventory_reconcile_rounds_total", "Reconciler rounds over the active events.")
	reconSkips  = metrics.NewCounter("eps_inventory_reconcile_skipped_total", "Pools skipped because Postgres changed during the read.")
)

func init() {
	for _, k := range []string{"shard_invariant", "allocation", "sold_raised", "oversell_risk", "unpaid_commit"} {
		corrections[k] = metrics.NewCounter("eps_inventory_corrections_total", "Reconciler corrections by kind.", "kind", k)
	}
}

// Reconciler compares the shards with Postgres (D-10) and repairs drift seen in two rounds in a
// row: a broken shard invariant (a lost update), allocations that no longer add up to the
// capacity, Postgres paid sales the counters do not show (a Redis loss: sold is raised from free
// stock, and when free stock cannot cover it the pool stops taking holds and an alert is raised),
// and commits whose order is still unpaid 2 minutes later (returned to free stock). Every
// correction is an inventory_adjustments row and an event.
type Reconciler struct {
	Inv  *Sharded
	Pool *pgxpool.Pool
	Log  *slog.Logger
	// UnpaidAfter is how long a commit may wait for its order to be paid (default 2 minutes).
	UnpaidAfter time.Duration

	mu   sync.Mutex
	seen map[string]int64
	now  map[string]int64
}

// Correction is one repair the reconciler applied.
type Correction struct {
	OrganiserID, EventID, Instance string
	Shard                          int
	Kind                           string
	Delta                          int64
}

// confirm reports whether the same drift was seen in the previous round.
func (r *Reconciler) confirm(key string, drift int64) bool {
	r.now[key] = drift
	prev, ok := r.seen[key]
	return ok && prev == drift
}

// RunOnce reconciles every event with holds once.
func (r *Reconciler) RunOnce(ctx context.Context) ([]Correction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = map[string]int64{}
	}
	r.now = map[string]int64{}
	defer func() { r.seen = r.now }()
	evs, err := r.Inv.Events(ctx)
	if err != nil {
		return nil, err
	}
	var out []Correction
	for _, e := range evs {
		c, err := r.event(ctx, e[0], e[1])
		if err != nil && r.Log != nil && ctx.Err() == nil {
			r.Log.Warn("reconcile failed", "event", e[1], "error", err.Error())
		}
		out = append(out, c...)
	}
	reconRounds.Inc()
	return out, nil
}

func (r *Reconciler) paid(ctx context.Context, org, event string) (map[string]int64, error) {
	out := map[string]int64{}
	err := db.WithTx(ctx, r.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select d.key, sum(d.value::bigint) from sales.orders o, jsonb_each_text(o.draws) d
			where o.event_id = $1 and o.state in ('paid', 'issued') group by d.key`, event)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			out[k] = n
		}
		return rows.Err()
	})
	return out, err
}

func (r *Reconciler) event(ctx context.Context, org, event string) ([]Correction, error) {
	var out []Correction
	// Commits still unpaid after the grace: their stock goes back.
	after := r.UnpaidAfter
	if after <= 0 {
		after = 2 * time.Minute
	}
	ids, err := r.Inv.Committed(ctx, org, event, r.Inv.now().Add(-after), 200)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		orderID, _, _ := strings.Cut(id, "_")
		var state, ref string
		err := db.WithTx(ctx, r.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select state, hold_ref from sales.orders where id::text = $1`, orderID).Scan(&state, &ref)
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		if (state == "paid" || state == "issued") && ref == id {
			_ = r.Inv.Verified(ctx, org, event, id)
			continue
		}
		if err := r.Inv.Unsell(ctx, org, event, id); err != nil {
			return out, err
		}
		c := Correction{org, event, "", -1, "unpaid_commit", 0}
		out = append(out, c)
		r.record(ctx, c, "commit "+id+" without a paid order")
	}
	insts, err := r.Inv.Instances(ctx, org, event)
	if err != nil {
		return out, err
	}
	paid1, err := r.paid(ctx, org, event)
	if err != nil {
		return out, err
	}
	snaps := map[string][]ShardState{}
	for in := range insts {
		sh, err := r.Inv.Shards(ctx, org, event, in)
		if err != nil {
			return out, err
		}
		snaps[in] = sh
	}
	paid2, err := r.paid(ctx, org, event)
	if err != nil {
		return out, err
	}
	for in := range insts {
		stockGauges(org, event, in, snaps[in])
	}
	for in, capacity := range insts {
		if paid1[in] != paid2[in] {
			reconSkips.Inc()
			continue
		}
		sh := snaps[in]
		key := org + "/" + event + "/" + in
		var alloc, sold int64
		for _, x := range sh {
			alloc += x.Alloc
			sold += x.Sold
			// 1. The shard invariant: used = sold + held entries.
			if d := x.Used - x.Sold - x.Held; d != 0 && r.confirm(key+"/u/"+strconv.Itoa(x.Shard), d) {
				if _, err := r.Inv.repair(ctx, org, event, in, x.Shard, "used", -d); err != nil {
					return out, err
				}
				c := Correction{org, event, in, x.Shard, "shard_invariant", -d}
				out = append(out, c)
				r.record(ctx, c, "used counter repaired")
			}
		}
		// 2. Allocations add up to the capacity.
		if capacity >= 0 && len(sh) > 0 {
			if d := capacity - alloc; d != 0 && r.confirm(key+"/a", d) {
				got := int64(0)
				for _, x := range sh {
					if got == d {
						break
					}
					v, err := r.Inv.repair(ctx, org, event, in, x.Shard, "alloc", d-got)
					if err != nil {
						return out, err
					}
					got += v
				}
				c := Correction{org, event, in, -1, "allocation", got}
				out = append(out, c)
				r.record(ctx, c, "allocations set to the capacity")
			}
		}
		// 3. Postgres ahead of the counters: raise sold from free stock, else stop new holds.
		if d := paid1[in] - sold; d > 0 && r.confirm(key+"/s", d) {
			got := int64(0)
			for _, x := range sh {
				if got == d {
					break
				}
				v, err := r.Inv.repair(ctx, org, event, in, x.Shard, "absorb", d-got)
				if err != nil {
					return out, err
				}
				got += v
			}
			if got > 0 {
				c := Correction{org, event, in, -1, "sold_raised", got}
				out = append(out, c)
				r.record(ctx, c, "sold raised to the paid orders")
			}
			halted := len(sh) > 0
			for _, x := range sh {
				halted = halted && x.Halted
			}
			if got < d && !halted {
				if err := r.Inv.Halt(ctx, org, event, in, true); err != nil {
					return out, err
				}
				c := Correction{org, event, in, -1, "oversell_risk", d - got}
				out = append(out, c)
				r.record(ctx, c, "paid orders exceed what the counters can cover: new holds stopped")
			}
		}
	}
	return out, nil
}

func (r *Reconciler) record(ctx context.Context, c Correction, detail string) {
	if m := corrections[c.Kind]; m != nil {
		m.Inc()
	}
	if r.Log != nil {
		r.Log.Warn("inventory corrected", "event", c.EventID, "instance", c.Instance, "shard", c.Shard, "kind", c.Kind, "delta", c.Delta)
	}
	typ := "inventory.drift_corrected"
	if c.Kind == "oversell_risk" {
		typ = "inventory.oversell_risk"
	}
	var shard *int
	if c.Shard >= 0 {
		v := c.Shard
		shard = &v
	}
	_ = db.WithTx(ctx, r.Pool, db.RoleCore, c.OrganiserID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `insert into sales.inventory_adjustments (organiser_id, event_id, instance_id, shard, kind, delta, detail) values ($1, $2, $3, $4, $5, $6, $7)`,
			c.OrganiserID, c.EventID, c.Instance, shard, c.Kind, c.Delta, detail); err != nil {
			return err
		}
		_, err := outbox.Write(ctx, tx, outbox.Event{Topic: TopicInventory, Type: typ, OrganiserID: c.OrganiserID, EventID: c.EventID,
			Payload: map[string]any{"instanceId": c.Instance, "shard": c.Shard, "kind": c.Kind, "delta": c.Delta}})
		return err
	})
}

// Run reconciles every interval while it holds the reconciler lease (one worker at a time).
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	lease, _ := redisx.PlatformKey("inv-reconciler")
	me := strconv.FormatInt(time.Now().UnixNano(), 36)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ok, err := r.Inv.Redis.SetNX(ctx, lease, me, 3*interval).Result()
		if err == nil && !ok {
			if cur, _ := r.Inv.Redis.Get(ctx, lease).Result(); cur == me {
				_ = r.Inv.Redis.Expire(ctx, lease, 3*interval).Err()
				ok = true
			}
		}
		if ok {
			_, _ = r.RunOnce(ctx)
		}
	}
}

// Stock gauges per instance (step 6's dashboard reads them): free, held and sold units as of the
// reconciler's last round.
var stock sync.Map // key -> *[3]atomic.Int64

func stockGauges(org, event, instance string, sh []ShardState) {
	key := event + "/" + instance
	v, loaded := stock.LoadOrStore(key, &[3]atomic.Int64{})
	vals := v.(*[3]atomic.Int64)
	var free, held, sold int64
	for _, x := range sh {
		free += max(0, x.Alloc-x.Used)
		held += x.Held
		sold += x.Sold
	}
	vals[0].Store(free)
	vals[1].Store(held)
	vals[2].Store(sold)
	if loaded {
		return
	}
	l := `{organiser="` + org + `",event="` + event + `",instance="` + strings.ReplaceAll(instance, `"`, "") + `"}`
	metrics.NewGauge("eps_inventory_free"+l, func() float64 { return float64(vals[0].Load()) })
	metrics.NewGauge("eps_inventory_held"+l, func() float64 { return float64(vals[1].Load()) })
	metrics.NewGauge("eps_inventory_sold"+l, func() float64 { return float64(vals[2].Load()) })
}
