// Package wire builds the step 4 services (checkout, payments, wallet) from the environment, for
// the core and the worker alike.
package wire

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsale"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n >= 0 {
		return n
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
		if env("EPS_ENV", "dev") != "dev" {
			// Fail closed (review sec L6): order links must verify on every core.
			log.Error("ORDER_LINK_KEY is required outside EPS_ENV=dev")
			os.Exit(1)
		}
		log.Warn("ORDER_LINK_KEY not set (dev): order links use a per-process key")
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		linkKey = b
	}
	co := &checkout.Service{Pool: pool, Inv: &inventory.Sharded{Redis: rdb}, Snaps: snaps, Links: &checkout.Links{Key: linkKey}, Log: log}
	client := &http.Client{Timeout: 10 * time.Second, Transport: payments.SharedTransport}
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
	w := &wallet.Service{Pool: pool, Redis: rdb, Checkout: co, Payments: ps}
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
	return &coreapi.Shop{Checkout: s.Checkout, Payments: s.Payments, Wallet: s.Wallet, Knobs: s.Knobs, ReturnOrigins: origins, Log: log,
		Redis: rdb, HoldsPerMin: envInt("HOLD_RATE_PER_MINUTE", 10), LiveHoldsPer: envInt("HOLD_LIVE_PER_CLIENT", 10)}, hooks
}

// SalesService is the sale settings service of a process (the core, the gateway and the worker).
func SalesService(pool *pgxpool.Pool, rdb *redis.Client, snaps *snapshot.Store) *sales.Service {
	svc := &sales.Service{Redis: rdb, MACKey: []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))}
	if pool != nil {
		svc.Pool = pool // a nil *pgxpool.Pool must not become a non-nil interface
	}
	if snaps != nil {
		svc.Published = func(ctx context.Context, event string) (string, error) {
			s, err := snaps.Load(ctx, event)
			if errors.Is(err, snapshot.ErrNotFound) {
				return "", nil
			}
			if err != nil {
				return "", err
			}
			return s.OrganiserID, nil
		}
	}
	return svc
}

// OnSale wires the step 5 gate into the core's checkout: sale settings, the pass check (the
// gateway's public key, QUEUE_PASS_PUBLIC_KEY; required outside EPS_ENV=dev), caps, scores and OTP.
func OnSale(pool *pgxpool.Pool, rdb *redis.Client, snaps *snapshot.Store, co *checkout.Service, log *slog.Logger) *coreapi.OnSale {
	_, pub, err := queue.KeysFromEnv(false)
	if err != nil {
		log.Error("pass keys", "error", err.Error())
		os.Exit(1)
	}
	sv := SalesService(pool, rdb, snaps)
	g := &onsale.Gate{Sales: sv, Redis: rdb, Pool: pool, Passes: &queue.PassCheck{Verifier: queue.NewVerifier(pub), Redis: rdb},
		Caps: &botdefence.Caps{Redis: rdb}, Scorer: &botdefence.Scorer{Redis: rdb}, OTP: &botdefence.OTP{Redis: rdb}, Checkout: co, Secret: IdentitySecret()}
	co.Gate = g
	life := &onsale.Lifecycle{Checkout: co, Engine: &queue.Engine{Redis: rdb}, Caps: g.Caps}
	co.Ended = func(ctx context.Context, o *checkout.Order, how string) { _ = life.End(ctx, o, how) }
	return &coreapi.OnSale{Sales: sv, Gate: g, Redis: rdb}
}

// IdentitySecret keys the stored identity and address hashes of the waiting rooms and caps
// (review sec L4): derived from SNAPSHOT_CACHE_KEY, so every core and gateway of an environment
// agrees and no new secret is needed.
func IdentitySecret() []byte {
	m := hmac.New(sha256.New, []byte(os.Getenv("SNAPSHOT_CACHE_KEY")))
	m.Write([]byte("eps-identity-hashes-v1"))
	return m.Sum(nil)
}

// RequireMACKey stops a process outside EPS_ENV=dev whose Redis copies would be signed with an
// empty key, and any process without EPS_ENV at all (review sec L6).
func RequireMACKey(log *slog.Logger) {
	envName := os.Getenv("EPS_ENV")
	if envName == "" {
		log.Error("EPS_ENV is required (dev for local development)")
		os.Exit(1)
	}
	if envName != "dev" && os.Getenv("SNAPSHOT_CACHE_KEY") == "" {
		log.Error("SNAPSHOT_CACHE_KEY is required outside EPS_ENV=dev")
		os.Exit(1)
	}
}

// Simulation wires rehearsals and runs (step 6): the providers' admin knobs for the rehearsal
// merchants and the environment's run cap (SIM_MAX_FANS; default 2,000).
func Simulation(pool *pgxpool.Pool, rdb *redis.Client, snaps *snapshot.Store, sv *sales.Service, knobs map[string]*payments.Knobs, log *slog.Logger) *simulation.Service {
	l := simulation.DefaultLimits
	if n := envInt("SIM_MAX_FANS", 0); n >= 10 && n <= 200_000 {
		l.MaxFans = n
	}
	return &simulation.Service{Pool: pool, Redis: rdb, Sales: sv, Snaps: snaps, Knobs: knobs, Limits: l, Log: log}
}

// DegradedOf tells the availability answer whether a current sale of an event is degraded.
func DegradedOf(sv *sales.Service) func(ctx context.Context, org, event string) bool {
	return func(ctx context.Context, org, event string) bool {
		list, err := sv.Live(ctx, org, event)
		if err != nil {
			return false
		}
		cur, ok := sales.Current(list, time.Now())
		return ok && cur != nil && cur.Degraded
	}
}

// SweepRuns ends lost runs every 5 seconds (design D-2).
func SweepRuns(ctx context.Context, sim *simulation.Service) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sim.Sweep(ctx)
		}
	}
}

// SimControlToken is the bearer token of the simulator's internal API (SIM_CONTROL_TOKEN, from
// eps-runtime); required outside EPS_ENV=dev.
func SimControlToken(log *slog.Logger) string {
	t := strings.TrimSpace(os.Getenv("SIM_CONTROL_TOKEN"))
	if t == "" {
		if env("EPS_ENV", "dev") != "dev" {
			log.Error("SIM_CONTROL_TOKEN is required outside EPS_ENV=dev")
			os.Exit(1)
		}
		t = "dev-simulator-control-token"
	}
	return t
}

// ServeInternal serves the simulator's API on INTERNAL_PORT (default 8089; "off" disables), a port
// the ingress never routes to.
func ServeInternal(log *slog.Logger, sim *simulation.Service) {
	port := env("INTERNAL_PORT", "8089")
	if port == "off" {
		return
	}
	mux := http.NewServeMux()
	(&simulation.InternalHTTP{Svc: sim, Token: SimControlToken(log)}).Routes(mux)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second}
	// Bound before the core serves anything: without its internal port no run can start, so a
	// failure stops the process (Kubernetes restarts it) instead of serving without it.
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Error("internal listener failed", "error", err.Error())
		os.Exit(1)
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("internal listener stopped", "error", err.Error())
			os.Exit(1)
		}
	}()
	log.Info("simulator API on", "port", port)
}
