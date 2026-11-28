// Command worker runs the background work of the sales side (step 4): the hold sweeper, the payment
// reconciler, the instalment runner and the plan defaults, the outbox relay to Redis Streams, the
// purge of old idempotency keys and webhook ids, the stub issuer that marks paid orders issued
// until step 7 issues real tickets, and step 5's shard sweeper, inventory reconciler, queue audit
// and pass/cap ends. Each loop has its own small connection pool (review M1), so a
// slow provider never starves the sweeper.
package main

import (
	"context"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsale"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func poolFor(ctx context.Context, name string, def int32) *pgxpool.Pool {
	n := def
	if v, err := strconv.Atoi(os.Getenv("WORKER_POOL_" + name)); err == nil && v > 0 && v <= 20 {
		n = int32(v)
	}
	p, err := db.Open(ctx, os.Getenv("DATABASE_URL"), n)
	if err != nil {
		return nil
	}
	return p
}

func main() {
	log := service.Logger("worker")
	wire.RequireMACKey(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool := service.Postgres(ctx, log)
	rdb := service.Redis(log)
	checks := []health.Checker{health.Postgres(pool), health.Redis(rdb)}
	if pool != nil && rdb != nil {
		snaps := snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, log, snapshot.Options{MACKey: []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))})
		svc, _ := wire.Build(pool, rdb, snaps, log)
		// Loops with their own pools (falling back to the shared one).
		with := func(name string, def int32) (*wire.Services, *pgxpool.Pool) {
			p := poolFor(ctx, name, def)
			if p == nil {
				return svc, pool
			}
			s2, _ := wire.Build(p, rdb, snaps, log)
			return s2, p
		}
		sweep, _ := with("SWEEPER", 2)
		recon, _ := with("RECONCILER", 2)
		inst, _ := with("INSTALMENTS", 1)
		_, relayPool := with("RELAY", 1)
		host, _ := os.Hostname()
		// Every loop below is a singleton: with two worker replicas (review spec M6) only the holder
		// of the worker lease runs them, the other is a hot standby that takes over within seconds.
		// The order-event consumers run on both (consumer groups share the work).
		inv := &inventory.Sharded{Redis: rdb, LiveOrders: sweep.Checkout.LiveHoldRefs}
		go leader(ctx, rdb, host, log, func(lctx context.Context) {
			go (&checkout.Sweeper{Svc: sweep.Checkout, Log: log, Index: inv}).Run(lctx, time.Second)
			go (&payments.Reconciler{Svc: recon.Payments, Log: log}).Run(lctx, 5*time.Second)
			go (&checkout.InstalmentRunner{Svc: inst.Checkout, Charger: inst.Payments, Log: log}).Run(lctx, 5*time.Second)
			go every(lctx, time.Minute, func() { _, _ = (&checkout.DefaultRunner{Svc: inst.Checkout, Log: log}).RunOnce(lctx) })
			go (&outbox.Relay{Pool: relayPool, Redis: rdb, Log: log}).Run(lctx, 200*time.Millisecond)
			go every(lctx, 10*time.Minute, func() {
				_ = db.WithTx(lctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
					var a, b int64
					if err := tx.QueryRow(lctx, "select idempotency_keys, webhook_events from sales.purge_expired()").Scan(&a, &b); err != nil {
						return err
					}
					log.Info("purged", "idempotency_keys", a, "webhook_events", b)
					return nil
				})
			})
			// Step 5: the shard sweeper (lapsed shard entries within a second or two, even when the core
			// that took them stopped), the reconciler (every 15 s, one worker at a time), the waiting
			// rooms' audit copy to Postgres, and the end of passes and cap entries from order events.
			go (&inventory.ShardSweeper{Inv: inv, Log: log}).Run(lctx, time.Second)
			go (&inventory.Reconciler{Inv: inv, Pool: recon.Checkout.Pool, Log: log}).Run(lctx, 15*time.Second)
			go (&onsale.Audit{Redis: rdb, Pool: pool, Log: log}).Run(lctx, 2*time.Second)
			salesSvc := wire.SalesService(pool, rdb, snaps)
			go every(lctx, time.Minute, func() { _, _ = salesSvc.RefreshActive(lctx) })
		})
		registerGauges(ctx, pool)
		// Stub issuer (step 7 replaces it): order.paid → issued.
		issuer := &outbox.Consumer{Redis: rdb, Topic: outbox.TopicOrders, Group: "issuer", Name: host, Log: log,
			Handle: func(ctx context.Context, e outbox.Envelope) error {
				if e.Type != "order.paid" {
					return nil
				}
				var p struct {
					OrderID string `json:"orderId"`
				}
				if err := jsonUnmarshal(e.Payload, &p); err != nil || p.OrderID == "" {
					return nil
				}
				return svc.Checkout.Issue(ctx, e.OrganiserID, p.OrderID)
			}}
		go issuer.Run(ctx)
		life := &onsale.Lifecycle{Checkout: svc.Checkout, Engine: &queue.Engine{Redis: rdb}, Caps: &botdefence.Caps{Redis: rdb}}
		go (&outbox.Consumer{Redis: rdb, Topic: outbox.TopicOrders, Group: "onsale", Name: host, Log: log, Handle: life.Handle}).Run(ctx)
		log.Info("worker loops started", "sweeper", "1s", "reconciler", "5s", "instalments", "5s", "relay", "200ms")
	}
	service.Serve(log, service.SimpleHandler("worker", log, checks, nil, nil))
}

func every(ctx context.Context, d time.Duration, f func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f()
		}
	}
}

// registerGauges samples the outbox backlog and unresolved attempts every 10 s (metrics).
func registerGauges(ctx context.Context, pool *pgxpool.Pool) {
	var pending, oldest, unresolved atomic.Uint64
	load := func(v *atomic.Uint64) func() float64 { return func() float64 { return math.Float64frombits(v.Load()) } }
	metrics.NewGauge("eps_outbox_pending", load(&pending))
	metrics.NewGauge("eps_outbox_oldest_seconds", load(&oldest))
	metrics.NewGauge("eps_payment_attempts_unresolved", load(&unresolved))
	go every(ctx, 10*time.Second, func() {
		_ = db.WithTx(ctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
			var p, u int64
			var o float64
			if err := tx.QueryRow(ctx, "select pending, oldest_seconds from sales.outbox_backlog()").Scan(&p, &o); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, "select sales.unresolved_count()").Scan(&u); err != nil {
				return err
			}
			pending.Store(math.Float64bits(float64(p)))
			oldest.Store(math.Float64bits(o))
			unresolved.Store(math.Float64bits(float64(u)))
			return nil
		})
	})
}

// leader runs f while this worker holds the worker lease (renewed every 3 s, 10 s life); f's
// context ends when the lease is lost.
func leader(ctx context.Context, rdb *redis.Client, host string, log *slog.Logger, f func(context.Context)) {
	key, _ := redisx.PlatformKey("worker-lease")
	me := host + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	renew := redis.NewScript(`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE', KEYS[1], ARGV[2]) end return 0`)
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	var cancel context.CancelFunc
	for {
		ok, err := rdb.SetNX(ctx, key, me, 10*time.Second).Result()
		if err == nil && !ok {
			n, _ := renew.Run(ctx, rdb, []string{key}, me, 10000).Int()
			ok = n == 1
		}
		switch {
		case ok && cancel == nil:
			var lctx context.Context
			lctx, cancel = context.WithCancel(ctx)
			log.Info("worker lease taken: running the loops")
			f(lctx)
		case !ok && cancel != nil && err == nil:
			log.Warn("worker lease lost: loops stopped")
			cancel()
			cancel = nil
		}
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
			}
			return
		case <-t.C:
		}
	}
}
