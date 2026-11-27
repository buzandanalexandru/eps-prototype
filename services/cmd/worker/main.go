// Command worker runs background work (issuing, webhooks, reconciliation in later steps).
// Step 0 serves health and readiness only.
package main

import (
	"context"

	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
)

func main() {
	log := service.Logger("worker")
	pool := service.Postgres(context.Background(), log)
	rdb := service.Redis(log)
	checks := []health.Checker{health.Postgres(pool), health.Redis(rdb)}
	service.Serve(log, service.SimpleHandler("worker", log, checks, nil, nil))
}
