// Command core is the EPS core API (holds, checkout, payments in later steps). Step 0 serves the
// contract's health, version and whoami operations.
package main

import (
	"context"
	"os"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
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
	srv := &coreapi.Server{
		Service: "core",
		Pool:    pool,
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
	})
	if err != nil {
		log.Error("contract unreadable", "error", err.Error())
		os.Exit(1)
	}
	service.Serve(log, h)
}
