// Package wire builds the step 4 services (checkout, payments, wallet) from the environment, for
// the core and the worker alike.
package wire

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Services are the step 4 services of one process.
type Services struct {
	Checkout *checkout.Service
	Payments *payments.Service
	Wallet   *wallet.Service
	Knobs    map[string]*payments.Knobs
}

// Build wires checkout, payments and wallet. Secrets come from the per-environment eps-runtime
// secret: ORDER_LINK_KEY signs resume and order links, PSP_API_KEY is the core's key at the
// simulated providers, PSP_ADMIN_TOKEN their knobs, PSP_WEBHOOK_SECRET_A/B their webhook keys.
func Build(pool *pgxpool.Pool, rdb *redis.Client, snaps *snapshot.Store, log *slog.Logger) (*Services, *payments.Webhooks) {
	linkKey := []byte(os.Getenv("ORDER_LINK_KEY"))
	if len(linkKey) == 0 {
		log.Warn("ORDER_LINK_KEY not set: order links use a per-process key")
		linkKey = []byte(time.Now().String())
	}
	co := &checkout.Service{Pool: pool, Inv: &checkout.RedisInventory{Redis: rdb}, Snaps: snaps, Links: &checkout.Links{Key: linkKey}, Log: log}
	client := &http.Client{Timeout: 10 * time.Second}
	apiKey := os.Getenv("PSP_API_KEY")
	urls := map[string]string{"a": env("PSP_A_URL", "http://localhost:8091"), "b": env("PSP_B_URL", "http://localhost:8092")}
	ps := &payments.Service{Pool: pool, Checkout: co, Providers: map[string]payments.Provider{}, Preferred: env("PSP_PREFERRED", "a"), Log: log}
	ps.Breaker = &payments.Breaker{Redis: rdb, OnChange: ps.ProviderStateEvent}
	knobs := map[string]*payments.Knobs{}
	for id, u := range urls {
		ps.Providers[id] = &payments.HTTPProvider{ID: id, BaseURL: strings.TrimRight(u, "/"), APIKey: apiKey, Timeout: 3 * time.Second, Client: client}
		knobs[id] = &payments.Knobs{BaseURL: strings.TrimRight(u, "/"), AdminToken: os.Getenv("PSP_ADMIN_TOKEN"), Client: client}
	}
	hookBase := strings.TrimRight(env("CORE_INTERNAL_URL", "http://localhost:8080"), "/")
	ps.WebhookURL = func(p string) string { return hookBase + "/v1/webhooks/psp/" + p }
	w := &wallet.Service{Pool: pool, Checkout: co, Payments: ps}
	ps.Topups = w
	hooks := &payments.Webhooks{Svc: ps, Log: log, Secrets: map[string][]byte{
		"a": []byte(os.Getenv("PSP_WEBHOOK_SECRET_A")),
		"b": []byte(os.Getenv("PSP_WEBHOOK_SECRET_B")),
	}}
	return &Services{Checkout: co, Payments: ps, Wallet: w, Knobs: knobs}, hooks
}

// Shop is Build for the core's API.
func Shop(pool *pgxpool.Pool, rdb *redis.Client, snaps *snapshot.Store, log *slog.Logger) (*coreapi.Shop, *payments.Webhooks) {
	s, hooks := Build(pool, rdb, snaps, log)
	var origins []string
	for _, o := range strings.Split(env("RETURN_URL_ORIGINS", os.Getenv("CORS_ALLOWED_ORIGINS")), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, strings.TrimRight(o, "/"))
		}
	}
	return &coreapi.Shop{Checkout: s.Checkout, Payments: s.Payments, Wallet: s.Wallet, Knobs: s.Knobs, ReturnOrigins: origins, Log: log}, hooks
}
