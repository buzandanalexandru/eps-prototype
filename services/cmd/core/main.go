// Command core is the EPS core API: the contract's health, identity and snapshot operations (steps
// 0 to 3) and checkout, payments and the wallet (step 4), plus the providers' webhook endpoint.
package main

import (
	"context"
	"net/http"
	"os"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func main() {
	log := service.Logger("core")
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
	if pool != nil {
		// Copies in Redis are signed with SNAPSHOT_CACHE_KEY (a secret CI generates per environment).
		snaps = snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, log, snapshot.Options{MACKey: []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))})
		if rdb != nil {
			shop, hooks = wire.Shop(pool, rdb, snaps, log)
			usage = checkout.Usage{Svc: shop.Checkout}
		}
		avail = &snapshot.AvailabilityService{Store: snaps, Usage: usage, Redis: rdb}
	}
	srv := &coreapi.Server{
		Snapshots:    snaps,
		Availability: avail,
		Usage:        usage,
		Shop:         shop,
		Service:      "core",
		Pool:         pool,
		Checks: []health.Checker{
			health.Postgres(pool),
			health.Redis(rdb),
			health.SchemaAtLeast(pool, db.Sales, salesVersion),
			health.SchemaAtLeast(pool, db.Control, db.MinControlVersion),
		},
	}
	var keys auth.KeySource = auth.DBKeySource{Pool: pool}
	if pool == nil {
		keys = noKeys{}
	}
	issuers := auth.IssuersFromEnv(os.Getenv)
	verifier := auth.NewVerifier(keys, issuers...)
	verifier.Start(ctx)
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
	service.Serve(log, h)
}
