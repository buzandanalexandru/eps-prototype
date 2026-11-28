// Package simtest builds the whole on-sale system in one test process for the step 6 tests: the
// core's contract handler (checkout, payments, sale settings, rehearsals, runs), the gateway (the
// waiting room with its admitter, and the live stream), both simulated providers, the core's
// internal simulator API and the simulator engine, each behind a real HTTP server, over
// testcontainers Postgres and Redis. Used by tests only.
package simtest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/live"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsale"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// LogBuffer is a concurrency-safe log sink.
type LogBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *LogBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

// String is everything logged so far.
func (l *LogBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// Issuer of the test organiser tokens.
const Issuer = "urn:eps:web:test"

type staticKeys map[string]ed25519.PublicKey

func (k staticKeys) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) { return k, nil }

// Stack is the running system.
type Stack struct {
	*salestest.Env
	Sales    *sales.Service
	Sim      *simulation.Service
	Payments *payments.Service
	Gate     *onsale.Gate
	Queue    *queue.Engine
	Agg      *live.Aggregator
	Verifier *auth.Verifier
	Breaker  *payments.Breaker
	Knobs    map[string]*payments.Knobs

	Core, Gateway, Internal, PSPA, PSPB *httptest.Server
	Engine                              *simulation.Engine
	// GatewayHits counts requests that reached the gateway (the Stop test).
	GatewayHits atomic.Int64

	// Logs holds every JSON log line of the stack (the personal-data test reads it).
	Logs *LogBuffer

	orgKey  ed25519.PrivateKey
	simKey  ed25519.PrivateKey
	Log     *slog.Logger
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// Start builds and starts the stack; everything stops when the test ends.
func Start(t *testing.T) *Stack {
	t.Helper()
	base := salestest.Setup(t)
	logs := &LogBuffer{}
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := snapshot.NewStore(snapshot.RehearsalSource{Inner: snapshot.DBSource{Pool: base.Pool}, Pool: base.Pool}, base.Redis, log,
		snapshot.Options{MemTTL: time.Millisecond, RedisTTL: time.Millisecond, MissingTTL: time.Millisecond, MACKey: []byte("test-key")})
	base.Store = store
	base.Svc.Snaps = store
	inv := &inventory.Sharded{Redis: base.Redis}
	base.Svc.Inv = inv
	ctx, cancel := context.WithCancel(context.Background())
	s := &Stack{Env: base, Log: log, Logs: logs, cancel: cancel}

	// Providers A and B.
	var coreMux http.Handler
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { coreMux.ServeHTTP(w, r) }))
	providers := map[string]*fakepsp.Provider{}
	for _, id := range []string{"a", "b"} {
		p := &fakepsp.Provider{ID: id, APIKey: "api-key", AdminToken: "admin-token", WebhookSecret: []byte("whsec-" + id), Redis: base.Redis, Log: log, RetryBase: 20 * time.Millisecond}
		mux := http.NewServeMux()
		p.Routes(mux)
		srv := httptest.NewServer(mux)
		p.PublicURL = srv.URL + "/" + id
		providers[id] = p
		if id == "a" {
			s.PSPA = srv
		} else {
			s.PSPB = srv
		}
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ps := &payments.Service{Pool: base.Pool, Checkout: base.Svc, Providers: map[string]payments.Provider{}, Preferred: "a", Log: log}
	ps.Breaker = &payments.Breaker{Redis: base.Redis, OnChange: ps.ProviderStateEvent}
	knobs := map[string]*payments.Knobs{}
	for id, srv := range map[string]*httptest.Server{"a": s.PSPA, "b": s.PSPB} {
		ps.Providers[id] = &payments.HTTPProvider{ID: id, BaseURL: srv.URL, APIKey: "api-key", Timeout: 3 * time.Second, Client: client}
		knobs[id] = &payments.Knobs{BaseURL: srv.URL, AdminToken: "admin-token", Client: client}
	}
	ps.WebhookURL = func(p string) string { return core.URL + "/v1/webhooks/psp/" + p }
	hooks := &payments.Webhooks{Svc: ps, Log: log, Secrets: map[string][]byte{"a": []byte("whsec-a"), "b": []byte("whsec-b")}}
	s.Payments, s.Breaker, s.Knobs = ps, ps.Breaker, knobs

	// Sales, the gate and the waiting room (real clock: the admitter runs for real).
	sv := &sales.Service{Pool: base.Pool, Redis: base.Redis, MACKey: []byte("test-key"),
		Published: func(ctx context.Context, event string) (string, error) {
			x, err := store.Load(ctx, event)
			if err != nil {
				return "", nil
			}
			return x.OrganiserID, nil
		}}
	_, passKey, _ := ed25519.GenerateKey(rand.Reader)
	scorer := &botdefence.Scorer{Redis: base.Redis}
	caps := &botdefence.Caps{Redis: base.Redis}
	eng := &queue.Engine{Redis: base.Redis, Sales: sv, Signer: queue.NewSigner(passKey), Scorer: scorer, JoinsPerMinute: 100000, NewGuestsPerMinute: 100000, Secret: []byte("test-secret")}
	gate := &onsale.Gate{Sales: sv, Redis: base.Redis, Pool: base.Pool, Passes: &queue.PassCheck{Verifier: queue.NewVerifier(passKey.Public().(ed25519.PublicKey)), Redis: base.Redis},
		Caps: caps, Scorer: scorer, OTP: &botdefence.OTP{Redis: base.Redis}, Checkout: base.Svc, Secret: []byte("test-secret")}
	base.Svc.Gate = gate
	life := &onsale.Lifecycle{Checkout: base.Svc, Engine: eng, Caps: caps}
	base.Svc.Ended = func(ctx context.Context, o *checkout.Order, how string) { _ = life.End(ctx, o, how) }
	s.Sales, s.Gate, s.Queue = sv, gate, eng

	// Tokens: organisers from a test key, simulated fans from the simulator's own key.
	_, s.orgKey, _ = ed25519.GenerateKey(rand.Reader)
	simPub, simKey, _ := ed25519.GenerateKey(rand.Reader)
	s.simKey = simKey
	v := auth.NewVerifier(staticKeys{"t1": s.orgKey.Public().(ed25519.PublicKey)}, Issuer)
	v.Sim = &auth.SimIssuer{Issuer: auth.SimIssuerForEnv("test"), Key: simPub}
	s.Verifier = v

	sim := &simulation.Service{Pool: base.Pool, Redis: base.Redis, Sales: sv, Snaps: store, Knobs: knobs, Limits: simulation.Limits{MaxFans: 2000, MaxDurationSeconds: 600}, Log: log}
	s.Sim = sim
	shop := &coreapi.Shop{Checkout: base.Svc, Payments: ps, Knobs: knobs, Log: log, Redis: base.Redis}
	avail := &snapshot.AvailabilityService{Store: store, Usage: checkout.Usage{Svc: base.Svc, HeldBy: inventory.HeldOf(inv)}, Redis: base.Redis}
	avail.Degraded = func(ctx context.Context, org, event string) bool {
		list, err := sv.Live(ctx, org, event)
		if err != nil {
			return false
		}
		cur, ok := sales.Current(list, time.Now())
		return ok && cur != nil && cur.Degraded
	}
	srv := &coreapi.Server{Service: "core", Pool: base.Pool, Snapshots: store, Availability: avail, Shop: shop,
		OnSale: &coreapi.OnSale{Sales: sv, Gate: gate, Redis: base.Redis}, Sim: sim}
	h, err := coreapi.NewHandler(srv, coreapi.Options{Log: log, Verifier: v, CORS: httpx.NewCORS("https://studio.test"),
		Extra: func(mux *http.ServeMux) { mux.HandleFunc("POST /v1/webhooks/psp/{provider}", hooks.Handle) }})
	if err != nil {
		t.Fatal(err)
	}
	coreMux = h
	s.Core = core

	// Gateway: the waiting room and the live stream.
	hub := &live.Hub{Redis: base.Redis, Sales: sv, Verifier: v, Log: log}
	qh := &queue.HTTP{Engine: eng, Verifier: v, Log: log}
	gw := service.SimpleHandler("gateway", log, nil, httpx.NewCORS("https://studio.test"), func(mux *http.ServeMux) { qh.Routes(mux); hub.Routes(mux) })
	gwh := httpx.WithWriteDeadline(30*time.Second, httpx.IsEventStream)(gw)
	s.Gateway = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.GatewayHits.Add(1)
		gwh.ServeHTTP(w, r)
	}))
	s.goRun(func() { (&queue.Admitter{Engine: eng, Log: log}).Run(ctx) })

	// The core's internal simulator API and the simulator.
	imux := http.NewServeMux()
	(&simulation.InternalHTTP{Svc: sim, Token: "control-token"}).Routes(imux)
	s.Internal = httptest.NewServer(imux)
	s.Engine = &simulation.Engine{
		Endpoints: simulation.Endpoints{Gateway: s.Gateway.URL, Core: core.URL, Internal: s.Internal.URL,
			PSP: map[string]string{"a": s.PSPA.URL, "b": s.PSPB.URL}, Return: "http://localhost:3000/sim-return"},
		Token: "control-token", Minter: &simulation.Minter{Key: simKey, Issuer: auth.SimIssuerForEnv("test")}, MaxInflight: 100, Log: log,
		Poll: 200 * time.Millisecond, Finish: 20 * time.Second,
	}
	s.Agg = &live.Aggregator{Redis: base.Redis, Pool: base.Pool, Sales: sv, Snaps: store, Inv: inv, Breaker: ps.Breaker, Log: log}
	t.Cleanup(func() {
		cancel()
		s.running.Wait()
		s.Gateway.CloseClientConnections()
		s.Gateway.Close()
		s.Internal.Close()
		core.Close()
		for _, p := range providers {
			p.Wait()
		}
		s.PSPA.Close()
		s.PSPB.Close()
	})
	return s
}

func (s *Stack) goRun(f func()) {
	s.running.Add(1)
	go func() { defer s.running.Done(); f() }()
}

// RunSimulator starts the simulator's claim loop until the test ends.
func (s *Stack) RunSimulator(ctx context.Context) {
	s.goRun(func() { s.Engine.Run(ctx) })
}

// RunAggregator frames watched sales every second until ctx ends.
func (s *Stack) RunAggregator(ctx context.Context) {
	s.goRun(func() { s.Agg.Run(ctx, nil) })
}

// Context ends with the test.
func (s *Stack) Context(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// OrganiserToken is a core token of the organiser (role owner).
func (s *Stack) OrganiserToken(t *testing.T, org string) string {
	t.Helper()
	now := time.Now()
	c := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: Issuer, Subject: uuid.NewString(), Audience: jwt.ClaimStrings{auth.Audience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))}, Kind: auth.KindOrganiser, Org: org, Role: "owner", Sid: "s1"}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header["kid"] = "t1"
	out, err := tok.SignedString(s.orgKey)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// FanToken is a web-issued guest token (a real fan).
func (s *Stack) FanToken(t *testing.T, ip string) string {
	t.Helper()
	now := time.Now()
	c := auth.Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: Issuer, Subject: "guest:" + uuid.NewString(), Audience: jwt.ClaimStrings{auth.Audience},
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))}, Kind: auth.KindGuest, Sid: "s1", ClientIP: ip}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header["kid"] = "t1"
	out, _ := tok.SignedString(s.orgKey)
	return out
}

// SimToken is a simulator token for a rehearsal event and run.
func (s *Stack) SimToken(t *testing.T, rehearsal, run, ip string) string {
	t.Helper()
	m := &simulation.Minter{Key: s.simKey, Issuer: auth.SimIssuerForEnv("test")}
	tok, err := m.Mint("guest:sim-"+strings.ReplaceAll(run, "-", "")[:12]+"-1", rehearsal, run, ip, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// RealSale publishes an event with a sale moment open since a minute ago (bot defence on).
func (s *Stack) RealSale(t *testing.T, arena, plus int64) (*salestest.Sale, *sales.Sale) {
	t.Helper()
	ev := s.NewSale(t, arena, plus)
	st := sales.Defaults(time.Now().Add(-time.Minute).Truncate(time.Second))
	st.RatePerMinute, st.ActiveLimit, st.CapPerOrder, st.CapPerPerson = 60000, 100000, 4, 6
	x, err := s.Sales.Create(context.Background(), ev.Org, ev.Event, "General sale", "test", st)
	if err != nil {
		t.Fatal(err)
	}
	return ev, x
}
