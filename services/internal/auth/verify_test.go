package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeSource struct {
	mu    sync.Mutex
	keys  map[string]ed25519.PublicKey
	calls int
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
	v := NewVerifier(src)
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
		{"guest", func(m jwt.MapClaims) { m["kind"] = "guest"; m["sub"] = "guest:abc"; delete(m, "org"); delete(m, "role") }, true},
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

// Scenario "Flood of unknown key ids".
func TestUnknownKeyIDFloodReloadsAtMostOncePerMinute(t *testing.T) {
	v, src, priv, c := setup(t)
	ctx := context.Background()
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 {
		t.Fatalf("initial load calls = %d", src.calls)
	}
	// A flood within the same minute: every token is rejected, at most one reload.
	c.t = c.t.Add(61 * time.Second)
	for i := 0; i < 500; i++ {
		tok := sign(t, priv, "unknown-"+string(rune('a'+i%26)), organiserClaims(c.t))
		if _, err := v.Verify(ctx, tok); err == nil {
			t.Fatal("token with unknown kid accepted")
		}
		c.t = c.t.Add(50 * time.Millisecond) // 25 s in total
	}
	if src.calls > 2 {
		t.Fatalf("reloaded %d times during a 25 s flood, want at most once more", src.calls-1)
	}
	// A key published after a rotation is picked up once the minute has passed.
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)
	src.mu.Lock()
	src.keys["k2"] = pub2
	src.mu.Unlock()
	c.t = c.t.Add(61 * time.Second)
	if _, err := v.Verify(ctx, sign(t, priv2, "k2", organiserClaims(c.t))); err != nil {
		t.Fatalf("rotated key not picked up: %v", err)
	}
	// Tokens of the previous key still verify during its grace period.
	if _, err := v.Verify(ctx, sign(t, priv, "k1", organiserClaims(c.t))); err != nil {
		t.Fatalf("previous key stopped verifying: %v", err)
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
