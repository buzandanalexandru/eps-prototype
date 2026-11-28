// Command gateway is the EPS gateway: the waiting rooms (step 5) - join and status for fans, the
// draw at opening and the admitter that lets fans in at each sale's rate and signs their passes -
// plus health, readiness and version. It holds no Postgres pool and its readiness depends only on
// Redis: the gateway keeps admitting fans from Redis state when the database is slow (graceful
// degradation, step 6). The core publishes the core-token keys and each sale's settings to Redis,
// signed with SNAPSHOT_CACHE_KEY.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/live"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func main() {
	log := service.Logger("gateway")
	wire.RequireMACKey(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rdb := service.Redis(log)
	checks := []health.Checker{health.Redis(rdb)}
	cors := httpx.NewCORS(service.Env("CORS_ALLOWED_ORIGINS", ""))
	priv, _, err := queue.KeysFromEnv(true)
	if err != nil {
		log.Error("pass keys", "error", err.Error())
		os.Exit(1)
	}
	var extra func(*http.ServeMux)
	if rdb != nil {
		mac := []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))
		verifier := auth.NewVerifier(auth.RedisKeySource{Redis: rdb, MACKey: mac}, auth.IssuersFromEnv(os.Getenv)...)
		// The simulator's rehearsal-only issuer (step 6): required outside EPS_ENV=dev.
		simIss, err := auth.SimIssuerFromEnv(os.Getenv)
		if err != nil {
			log.Error("simulator token key", "error", err.Error())
			os.Exit(1)
		}
		verifier.Sim = simIss
		verifier.Start(ctx)
		perMin, _ := strconv.Atoi(os.Getenv("QUEUE_JOINS_PER_MINUTE"))
		guests, _ := strconv.Atoi(os.Getenv("QUEUE_NEW_GUESTS_PER_MINUTE"))
		engine := &queue.Engine{Redis: rdb, Sales: wire.SalesService(nil, rdb, nil), Signer: queue.NewSigner(priv), Scorer: &botdefence.Scorer{Redis: rdb}, JoinsPerMinute: perMin, NewGuestsPerMinute: guests, Secret: wire.IdentitySecret()}
		go (&queue.Admitter{Engine: engine, Log: log}).Run(ctx)
		h := &queue.HTTP{Engine: engine, Verifier: verifier, Log: log}
		// Step 6: the live dashboard's server-sent events, straight to the organiser's browser.
		hub := &live.Hub{Redis: rdb, Sales: engine.Sales, Verifier: verifier, Log: log}
		extra = func(mux *http.ServeMux) {
			h.Routes(mux)
			hub.Routes(mux)
		}
		log.Info("waiting rooms on", "admitter", queue.Tick.String())
	}
	service.Serve(log, service.SimpleHandler("gateway", log, checks, cors, extra))
}
