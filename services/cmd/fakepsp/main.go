// Command fakepsp is a simulated payment provider (O4): psp-a and psp-b run this binary with
// PROVIDER_ID a or b. No real payments, ever; no card data, ever.
package main

import (
	"net/http"
	"os"

	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
)

func main() {
	id := service.Env("PROVIDER_ID", "a")
	log := service.Logger("fakepsp-" + id)
	if id != "a" && id != "b" {
		log.Error("PROVIDER_ID must be a or b")
		os.Exit(1)
	}
	rdb := service.Redis(log)
	p := &fakepsp.Provider{
		ID:            id,
		PublicURL:     service.Env("PSP_PUBLIC_URL", "http://localhost:8090/"+id),
		APIKey:        os.Getenv("PSP_API_KEY"),
		AdminToken:    os.Getenv("PSP_ADMIN_TOKEN"),
		WebhookSecret: []byte(os.Getenv("PSP_WEBHOOK_SECRET_" + map[string]string{"a": "A", "b": "B"}[id])),
		Redis:         rdb,
		Log:           log,
	}
	if p.APIKey == "" || p.AdminToken == "" || len(p.WebhookSecret) == 0 {
		log.Warn("PSP_API_KEY, PSP_ADMIN_TOKEN or the webhook secret is not set: the API refuses every call")
	}
	var checks []health.Checker
	if rdb != nil {
		checks = append(checks, health.Redis(rdb))
	}
	service.Serve(log, service.SimpleHandler("fakepsp", log, checks, nil, func(mux *http.ServeMux) { p.Routes(mux) }))
}
