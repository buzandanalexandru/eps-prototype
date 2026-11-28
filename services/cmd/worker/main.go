// Command worker runs the background work of the sales side (step 4): the hold sweeper, the payment
// reconciler, the instalment runner, the outbox relay to Redis Streams and the stub issuer that
// marks paid orders issued until step 7 issues real tickets.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func main() {
	log := service.Logger("worker")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool := service.Postgres(ctx, log)
	rdb := service.Redis(log)
	checks := []health.Checker{health.Postgres(pool), health.Redis(rdb)}
	if pool != nil && rdb != nil {
		snaps := snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, log, snapshot.Options{MACKey: []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))})
		svc, _ := wire.Build(pool, rdb, snaps, log)
		host, _ := os.Hostname()
		go (&checkout.Sweeper{Svc: svc.Checkout, Log: log}).Run(ctx, time.Second)
		go (&payments.Reconciler{Svc: svc.Payments, Log: log}).Run(ctx, 5*time.Second)
		go (&checkout.InstalmentRunner{Svc: svc.Checkout, Charger: svc.Payments, Log: log}).Run(ctx, 5*time.Second)
		go (&outbox.Relay{Pool: pool, Redis: rdb, Log: log}).Run(ctx, 200*time.Millisecond)
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
		log.Info("worker loops started", "sweeper", "1s", "reconciler", "5s", "instalments", "5s", "relay", "200ms")
	}
	service.Serve(log, service.SimpleHandler("worker", log, checks, nil, nil))
}
