// Command gateway is the EPS gateway (waiting room, passes and live streams in later steps).
// Step 0 serves health, readiness and version, with CORS for the platform's web origins. It holds
// no Postgres pool and its readiness depends only on Redis: the gateway must keep admitting fans
// from Redis state when the database is slow (graceful degradation, step 6).
package main

import (
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
)

func main() {
	log := service.Logger("gateway")
	rdb := service.Redis(log)
	checks := []health.Checker{health.Redis(rdb)}
	cors := httpx.NewCORS(service.Env("CORS_ALLOWED_ORIGINS", ""))
	service.Serve(log, service.SimpleHandler("gateway", log, checks, cors, nil))
}
