// Command core is the EPS core API: the contract's health, identity and snapshot operations (steps
// 0 to 3) and checkout, payments and the wallet (step 4), plus the providers' webhook endpoint.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/door"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func main() {
	log := service.Logger("core")
	wire.RequireMACKey(log)
	ctx := context.Background()
	pool := service.Postgres(ctx, log)
	rdb := service.Redis(log)

	salesVersion, err := db.LatestVersion(dbfiles.Sales())
	if err != nil {
		log.Error("embedded migrations unreadable", "error", err.Error())
		os.Exit(1)
	}
	var snaps *snapshot.Store
	var avail *snapshot.AvailabilityService
	var usage snapshot.Usage = snapshot.NoUsage{}
	var shop *coreapi.Shop
	var hooks *payments.Webhooks
	var onSale *coreapi.OnSale
	var sim *simulation.Service
	var tix *coreapi.Tickets
	// Step 7: the ticket signing key; outside EPS_ENV=dev the core stops without it.
	ticketKeys := wire.TicketKeys(log)
	if pool != nil {
		// Copies in Redis are signed with SNAPSHOT_CACHE_KEY (a secret CI generates per environment).
		// Rehearsal events (step 6) are served from their source event's pinned version.
		snaps = snapshot.NewStore(snapshot.RehearsalSource{Inner: snapshot.DBSource{Pool: pool}, Pool: pool}, rdb, log, snapshot.Options{MACKey: []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))})
		if rdb != nil {
			shop, hooks = wire.Shop(pool, rdb, snaps, log)
			usage = checkout.Usage{Svc: shop.Checkout, HeldBy: inventory.HeldOf(shop.Checkout.Inv)}
			onSale = wire.OnSale(pool, rdb, snaps, shop.Checkout, log)
			sim = wire.Simulation(pool, rdb, snaps, onSale.Sales, shop.Knobs, log)
			ts := wire.Tickets(pool, rdb, snaps, &wire.Services{Checkout: shop.Checkout, Payments: shop.Payments, Wallet: shop.Wallet}, ticketKeys, log)
			tix = &coreapi.Tickets{Svc: ts, Door: &door.Service{Pool: pool, Snaps: snaps}}
		}
		avail = &snapshot.AvailabilityService{Store: snaps, Usage: usage, Redis: rdb}
		if u, ok := usage.(checkout.Usage); ok {
			avail.Held = u
		}
		if onSale != nil {
			avail.Degraded = wire.DegradedOf(onSale.Sales)
		}
	}
	srv := &coreapi.Server{
		Snapshots:    snaps,
		Availability: avail,
		Usage:        usage,
		Shop:         shop,
		OnSale:       onSale,
		Sim:          sim,
		Tickets:      tix,
		Service:      "core",
		Pool:         pool,
		Checks: []health.Checker{
			health.Postgres(pool),
			health.Redis(rdb),
			health.SchemaAtLeast(pool, db.Sales, salesVersion),
			health.SchemaAtLeast(pool, db.Control, db.MinControlVersion),
			ticketKeys,
		},
	}
	var keys auth.KeySource = auth.DBKeySource{Pool: pool}
	if pool == nil {
		keys = noKeys{}
	}
	issuers := auth.IssuersFromEnv(os.Getenv)
	verifier := auth.NewVerifier(keys, issuers...)
	// The simulator's rehearsal-only issuer (step 6): required outside EPS_ENV=dev.
	simIss, err := auth.SimIssuerFromEnv(os.Getenv)
	if err != nil {
		log.Error("simulator token key", "error", err.Error())
		os.Exit(1)
	}
	verifier.Sim = simIss
	verifier.Start(ctx)
	if pool != nil && rdb != nil {
		// The gateway verifies core tokens with the keys the core publishes to Redis (step 5); every
		// 10 s (step 6), so a fresh environment's first key reaches the dashboard's direct stream soon.
		go auth.RunPublisher(ctx, keys, rdb, []byte(os.Getenv("SNAPSHOT_CACHE_KEY")), 10*time.Second)
	}
	metrics.NewGauge("eps_auth_key_loads", func() float64 { return float64(verifier.Loads()) })
	log.Info("token issuers", "issuers", issuers)
	h, err := coreapi.NewHandler(srv, coreapi.Options{
		Log:      log,
		Verifier: verifier,
		CORS:     httpx.NewCORS(service.Env("CORS_ALLOWED_ORIGINS", "")),
		Extra: func(mux *http.ServeMux) {
			// Provider results (signed; outside the public contract: only the providers call it).
			if hooks != nil {
				mux.HandleFunc("POST /v1/webhooks/psp/{provider}", hooks.Handle)
			}
		},
	})
	if err != nil {
		log.Error("contract unreadable", "error", err.Error())
		os.Exit(1)
	}
	if sim != nil {
		go wire.SweepRuns(ctx, sim)
		wire.ServeInternal(log, sim)
	}
	service.Serve(log, h)
}
