// Command gateway is the EPS gateway: the waiting rooms (step 5) - join and status for fans, the
// draw at opening and the admitter that lets fans in at each sale's rate and signs their passes -
// plus health, readiness and version. Its readiness depends only on Redis: the gateway keeps
// admitting fans from Redis state when the database is slow (graceful degradation, step 6). It
// reads Postgres only for the core-token keys and a sale's settings when Redis has no copy.
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
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wire"
)

func main() {
	log := service.Logger("gateway")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rdb := service.Redis(log)
	pool := service.Postgres(ctx, log)
	checks := []health.Checker{health.Redis(rdb)}
	cors := httpx.NewCORS(service.Env("CORS_ALLOWED_ORIGINS", ""))
	priv, _, err := queue.KeysFromEnv(true)
	if err != nil {
		log.Error("pass keys", "error", err.Error())
		os.Exit(1)
	}
	var extra func(*http.ServeMux)
	if rdb != nil {
		var keys auth.KeySource = auth.DBKeySource{Pool: pool}
		if pool == nil {
			keys = auth.StaticKeys{}
		}
		verifier := auth.NewVerifier(keys, auth.IssuersFromEnv(os.Getenv)...)
		verifier.Start(ctx)
		perMin, _ := strconv.Atoi(os.Getenv("QUEUE_JOINS_PER_MINUTE"))
		engine := &queue.Engine{Redis: rdb, Sales: wire.SalesService(pool, rdb, nil), Signer: queue.NewSigner(priv), Scorer: &botdefence.Scorer{Redis: rdb}, JoinsPerMinute: perMin}
		go (&queue.Admitter{Engine: engine, Log: log}).Run(ctx)
		h := &queue.HTTP{Engine: engine, Verifier: verifier, Log: log}
		extra = h.Routes
		log.Info("waiting rooms on", "admitter", queue.Tick.String())
	}
	service.Serve(log, service.SimpleHandler("gateway", log, checks, cors, extra))
}
