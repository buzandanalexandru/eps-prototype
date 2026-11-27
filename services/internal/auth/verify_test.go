package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeSource struct {
	mu    sync.Mutex
	keys  map[string]ed25519.PublicKey
	calls int
}

func (f *fakeSource) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeSource) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	out := map[string]ed25519.PublicKey{}
	for k, v := range f.keys {
		out[k] = v
	}
	return out, nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*Verifier, *fakeSource, ed25519.PrivateKey, *clock) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{keys: map[string]ed25519.PublicKey{"k1": pub}}
	c := &clock{t: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)}
	v := NewVerifier(src, "https://web.example")
	v.Now = c.now
	return v, src, priv, c
}

func sign(t *testing.T, priv ed25519.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func organiserClaims(now time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": "https://web.example", "aud": "eps-core", "sub": "user-1",
		"kind": "organiser", "org": "00000000-0000-4000-8000-00000000000a", "role": "owner",
		"sid": "sess-1", "jti": "j1",
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
}

// Scenario "Valid token".
func TestValidOrganiserToken(t *testing.T) {
	v, _, priv, c := setup(t)
	claims, err := v.Verify(context.Background(), sign(t, priv, "k1", organiserClaims(c.t)))
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.Kind != KindOrganiser || claims.Org == "" || claims.Role != "owner" {
		t.Fatalf("claims %+v", claims)
	}
}

// Scenario "Expired, tampered or wrong audience".
func TestRejectsExpiredTamperedWrongAudience(t *testing.T) {
	v, _, priv, c := setup(t)
	ctx := context.Background()

	// Expired 20 s ago: inside the 30 s leeway, accepted.
	m := organiserClaims(c.t.Add(-5*time.Minute - 20*time.Second))
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err != nil {
		t.Fatalf("within leeway should pass: %v", err)
	}
	// Expired 31 s ago: rejected.
	m = organiserClaims(c.t.Add(-5*time.Minute - 31*time.Second))
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err == nil {
		t.Fatal("expired token accepted")
	}
	// Wrong audience.
	m = organiserClaims(c.t)
	m["aud"] = "something-else"
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err == nil {
		t.Fatal("wrong audience accepted")
	}
	// Tampered payload: swap the organiser in the signed token.
	tok := sign(t, priv, "k1", organiserClaims(c.t))
	parts := strings.Split(tok, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var mc map[string]any
	_ = json.Unmarshal(payload, &mc)
	mc["org"] = "00000000-0000-4000-8000-00000000000b"
	b, _ := json.Marshal(mc)
	parts[1] = base64.RawURLEncoding.EncodeToString(b)
	if _, err := v.Verify(ctx, strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered token accepted")
	}
	// alg=none and HMAC are refused.
	none := jwt.NewWithClaims(jwt.SigningMethodNone, organiserClaims(c.t))
	none.Header["kid"] = "k1"
	s, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := v.Verify(ctx, s); err == nil {
		t.Fatal("alg none accepted")
	}
	// A lifetime longer than 5 minutes is refused.
	m = organiserClaims(c.t)
	m["exp"] = c.t.Add(time.Hour).Unix()
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err == nil {
		t.Fatal("hour-long token accepted")
	}
}

func TestKindRules(t *testing.T) {
	v, _, priv, c := setup(t)
	ctx := context.Background()
	cases := []struct {
		name string
		mod  func(jwt.MapClaims)
		ok   bool
	}{
		{"fan", func(m jwt.MapClaims) { m["kind"] = "fan"; delete(m, "org"); delete(m, "role") }, true},
		{"fan with org", func(m jwt.MapClaims) { m["kind"] = "fan" }, false},
		{"guest", func(m jwt.MapClaims) {
			m["kind"] = "guest"
			m["sub"] = "guest:abc"
			delete(m, "org")
			delete(m, "role")
		}, true},
		{"guest without guest sub", func(m jwt.MapClaims) { m["kind"] = "guest"; delete(m, "org"); delete(m, "role") }, false},
		{"organiser without org", func(m jwt.MapClaims) { delete(m, "org") }, false},
		{"organiser with guest sub", func(m jwt.MapClaims) { m["sub"] = "guest:abc" }, false},
		{"unknown kind", func(m jwt.MapClaims) { m["kind"] = "admin" }, false},
		{"no kind (plain session JWT)", func(m jwt.MapClaims) { delete(m, "kind") }, false},
		{"no sid", func(m jwt.MapClaims) { delete(m, "sid") }, false},
	}
	for _, tc := range cases {
		m := organiserClaims(c.t)
		tc.mod(m)
		_, err := v.Verify(ctx, sign(t, priv, "k1", m))
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

// Scenario "Flood of unknown key ids": unknown key ids reload at most once per UnknownKidEvery
// (15 s), on a limiter of their own; every such token is rejected.
func TestUnknownKeyIDFloodIsRateLimited(t *testing.T) {
	v, src, priv, c := setup(t)
	ctx := context.Background()
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err != nil {
		t.Fatal(err)
	}
	if src.count() != 1 {
		t.Fatalf("initial load calls = %d", src.count())
	}
	// A 10 s flood (inside one limiter window and inside the refresh interval).
	for i := 0; i < 500; i++ {
		tok := sign(t, priv, "unknown-"+string(rune('a'+i%26)), organiserClaims(c.t))
		if _, err := v.Verify(ctx, tok); err == nil {
			t.Fatal("token with unknown kid accepted")
		}
		c.t = c.t.Add(20 * time.Millisecond)
	}
	if got := src.count(); got != 2 {
		t.Fatalf("loads during a 10 s flood = %d, want 2 (initial + one unknown-kid reload)", got)
	}
	// A key published after a rotation is picked up once the limiter window has passed.
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)
	src.mu.Lock()
	src.keys["k2"] = pub2
	src.mu.Unlock()
	c.t = c.t.Add(16 * time.Second)
	if _, err := v.Verify(ctx, sign(t, priv2, "k2", organiserClaims(c.t))); err != nil {
		t.Fatalf("rotated key not picked up: %v", err)
	}
	// Tokens of the previous key still verify during its grace period.
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err != nil {
		t.Fatalf("previous key stopped verifying: %v", err)
	}
}

// The periodic refresh is separate from the unknown-kid limiter: a flood that keeps the limiter
// busy does not stop a stale set from refreshing, and keys are refreshed after RefreshEvery.
func TestPeriodicRefreshIsIndependentOfUnknownKidLimiter(t *testing.T) {
	v, src, priv, c := setup(t)
	ctx := context.Background()
	if err := v.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	// Use up the unknown-kid slot.
	_, _ = v.Verify(ctx, sign(t, priv, "nope", organiserClaims(c.t)))
	before := src.count()
	c.t = c.t.Add(61 * time.Second)
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for src.count() == before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if src.count() != before+1 {
		t.Fatalf("stale key set was not refreshed in the background (loads %d -> %d)", before, src.count())
	}
}

type flakySource struct {
	fakeSource
	fail atomic.Bool
	gate chan struct{} // when set, LoadKeys blocks until it is closed
}

func (f *flakySource) LoadKeys(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	if f.gate != nil {
		<-f.gate
	}
	if f.fail.Load() {
		f.mu.Lock()
		f.calls++
		f.mu.Unlock()
		return nil, errors.New("database down")
	}
	return f.fakeSource.LoadKeys(ctx)
}

// A failed first load does not lock verification out for a minute: after the short back-off the
// next request loads again and succeeds.
func TestFailedFirstLoadDoesNotLockOut(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	src := &flakySource{fakeSource: fakeSource{keys: map[string]ed25519.PublicKey{"k1": pub}}}
	src.fail.Store(true)
	c := &clock{t: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)}
	v := NewVerifier(src, "https://web.example")
	v.Now = c.now
	ctx := context.Background()
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err == nil {
		t.Fatal("verified without keys")
	}
	// Within the back-off, no database call.
	n := src.count()
	_, _ = v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t)))
	if src.count() != n {
		t.Fatal("retried inside the back-off")
	}
	src.fail.Store(false)
	c.t = c.t.Add(2 * time.Second)
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err != nil {
		t.Fatalf("still locked out after the database recovered: %v", err)
	}
}

// Many concurrent requests with an empty cache cause one database load, and readers never wait on
// a lock held across that load.
func TestConcurrentFirstLoadIsCollapsed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	src := &flakySource{fakeSource: fakeSource{keys: map[string]ed25519.PublicKey{"k1": pub}}, gate: make(chan struct{})}
	v := NewVerifier(src, "https://web.example")
	now := time.Now()
	v.Now = func() time.Time { return now }
	tok := sign(t, priv, "k1", organiserClaims(now))
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := v.Verify(context.Background(), tok)
			errs <- err
		}()
	}
	// While the load is blocked, the read lock is free.
	time.Sleep(50 * time.Millisecond)
	done := make(chan struct{})
	go func() { v.lookup("k1"); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lookup blocked while a load was in flight")
	}
	close(src.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if src.count() != 1 {
		t.Fatalf("loads = %d, want 1", src.count())
	}
}

// The issuer is pinned per environment, the role must be one of the membership roles, and a
// rejected key id is bounded before it reaches an error or a log line.
func TestIssuerRoleAndBoundedKid(t *testing.T) {
	v, _, priv, c := setup(t)
	ctx := context.Background()
	m := organiserClaims(c.t)
	m["iss"] = "urn:eps:web:other-env"
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err == nil {
		t.Fatal("token from another environment's issuer accepted")
	}
	m = organiserClaims(c.t)
	delete(m, "iss")
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err == nil {
		t.Fatal("token without issuer accepted")
	}
	for _, role := range []string{"owner", "admin", "box_office", "finance", "staff"} {
		m = organiserClaims(c.t)
		m["role"] = role
		if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err != nil {
			t.Fatalf("membership role %q refused: %v", role, err)
		}
	}
	m = organiserClaims(c.t)
	m["role"] = "superadmin"
	if _, err := v.Verify(ctx, sign(t, priv, "k1", m)); err == nil {
		t.Fatal("unknown role accepted")
	}
	none := NewVerifier(&fakeSource{})
	if _, err := none.Verify(ctx, sign(t, priv, "k1", organiserClaims(time.Now()))); err == nil {
		t.Fatal("a verifier without issuers accepted a token")
	}
	long := strings.Repeat("x", 5000) + "\n"
	_, err := v.Verify(ctx, sign(t, priv, long, organiserClaims(c.t)))
	if err == nil || len(err.Error()) > 300 || strings.Contains(err.Error(), "\n") {
		t.Fatalf("unbounded kid in error (%d bytes)", len(err.Error()))
	}
	if got := IssuersFromEnv(func(k string) string { return map[string]string{"EPS_ENV": "prod"}[k] }); len(got) != 1 || got[0] != "urn:eps:web:prod" {
		t.Fatalf("issuers from EPS_ENV = %v", got)
	}
}

func TestParseEd25519JWK(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	s := `{"kty":"OKP","crv":"Ed25519","x":"` + base64.RawURLEncoding.EncodeToString(pub) + `"}`
	k, err := ParseEd25519JWK(s)
	if err != nil || !k.Equal(pub) {
		t.Fatalf("parse: %v", err)
	}
	if _, err := ParseEd25519JWK(`{"kty":"RSA","n":"x","e":"AQAB"}`); err == nil {
		t.Fatal("RSA key accepted")
	}
}

// Scenario "Guest calls an organiser endpoint" (403), and the middleware's handling of no / bad
// tokens: the request goes on without claims, and the rejection is never echoed.
func TestMiddlewareAndGuestOnOrganiserEndpoint(t *testing.T) {
	v, _, priv, c := setup(t)
	organiserOnly := func(w http.ResponseWriter, r *http.Request) {
		cl, ok := FromContext(r.Context())
		switch {
		case !ok:
			w.WriteHeader(http.StatusUnauthorized)
		case cl.Kind != KindOrganiser:
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}
	h := Middleware(v, discardLogger())(http.HandlerFunc(organiserOnly))
	guest := organiserClaims(c.t)
	guest["kind"], guest["sub"] = "guest", "guest:g1"
	delete(guest, "org")
	delete(guest, "role")
	for name, tc := range map[string]struct {
		header string
		want   int
	}{
		"guest":     {"Bearer " + sign(t, priv, "k1", guest), http.StatusForbidden},
		"organiser": {"Bearer " + sign(t, priv, "k1", organiserClaims(c.t)), http.StatusOK},
		"none":      {"", http.StatusUnauthorized},
		"garbage":   {"Bearer not-a-jwt", http.StatusUnauthorized},
		"basic":     {"Basic dXNlcjpwYXNz", http.StatusUnauthorized},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, tc.want)
		}
	}
}
