package coreapi_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type keys map[string]ed25519.PublicKey

func (k keys) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) { return k, nil }

const (
	org   = "0a000000-0000-4000-8000-000000000001"
	brand = "0b000000-0000-4000-8000-000000000001"
	event = "0e000000-0000-4000-8000-000000000001"
)

func publishFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	doc, err := os.ReadFile("../snapshot/testdata/galaxia.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(doc, &m)
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"insert into control.organisers (id, slug, name) values ($1, 'fixture-org', 'Org')", []any{org}},
		{"insert into control.brands (id, organiser_id, slug, name) values ($1, $2, 'fixture-brand', 'Brand')", []any{brand, org}},
		{"insert into control.events (id, organiser_id, brand_id, slug, starts_at, ends_at) values ($1, $2, $3, 'galaxia-2027', '2027-07-15T14:00Z', '2027-07-19T00:00Z')", []any{event, org, brand}},
		{`insert into control.sale_snapshots (id, organiser_id, event_id, version, schema_version, document_text, sha256, terms_sha256)
		  values ($1, $2, $3, 1, 1, $4, $5, $5)`, []any{m["snapshotId"], org, event, string(b), hex.EncodeToString(sum[:])}},
		{"update control.events set live_snapshot_version = 1 where id = $1", []any{event}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
}

func newServer(t *testing.T) (http.Handler, func(claims jwt.MapClaims) string) {
	t.Helper()
	pool, _ := testdb.Migrated(t)
	publishFixture(t, pool)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	store := snapshot.NewStore(snapshot.DBSource{Pool: pool}, nil, nil, snapshot.Options{})
	h, err := coreapi.NewHandler(&coreapi.Server{
		Service: "core", Pool: pool, Snapshots: store,
		Availability: &snapshot.AvailabilityService{Store: store, Usage: snapshot.NoUsage{}},
	}, coreapi.Options{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Verifier: auth.NewVerifier(keys{"k1": pub}, auth.IssuerForEnv("test")),
		CORS:     httpx.NewCORS(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	sign := func(c jwt.MapClaims) string {
		now := time.Now()
		base := jwt.MapClaims{"iss": auth.IssuerForEnv("test"), "aud": "eps-core", "sub": "user-1", "sid": "s1", "jti": "j", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()}
		for k, v := range c {
			base[k] = v
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, base)
		tok.Header["kid"] = "k1"
		s, _ := tok.SignedString(priv)
		return s
	}
	return h, sign
}

func call(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// activate without a token, with a fan token and with another organiser's token; then with the
// owner's token. Pool usage the same way. Availability is public and short-cached.
func TestSnapshotEndpoints(t *testing.T) {
	h, sign := newServer(t)
	activate := "/v1/organiser/events/" + event + "/snapshots/1/activate"
	if rec := call(h, "POST", activate, ""); rec.Code != 401 {
		t.Fatalf("no token: %d", rec.Code)
	}
	if rec := call(h, "POST", activate, sign(jwt.MapClaims{"kind": "fan"})); rec.Code != 403 {
		t.Fatalf("fan token: %d %s", rec.Code, rec.Body.String())
	}
	other := sign(jwt.MapClaims{"kind": "organiser", "org": "0a000000-0000-4000-8000-0000000000ff", "role": "owner"})
	if rec := call(h, "POST", activate, other); rec.Code != 404 {
		t.Fatalf("other organiser: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(h, "GET", "/v1/organiser/events/"+event+"/pool-usage", other); rec.Code != 404 {
		t.Fatalf("other organiser's usage: %d", rec.Code)
	}
	owner := sign(jwt.MapClaims{"kind": "organiser", "org": org, "role": "owner"})
	rec := call(h, "POST", activate, owner)
	var a api.Activation
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &a) != nil || a.Version != 1 || len(a.Sha256) != 64 {
		t.Fatalf("owner activate: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(h, "POST", "/v1/organiser/events/"+event+"/snapshots/2/activate", owner); rec.Code != 404 {
		t.Fatalf("unknown version: %d", rec.Code)
	}
	rec = call(h, "GET", "/v1/organiser/events/"+event+"/pool-usage", owner)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"instances":[]`) {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(h, "GET", "/v1/public/events/"+event+"/availability", "")
	var av api.Availability
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &av) != nil {
		t.Fatalf("availability: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=2" || av.Version != 1 || len(av.Products) != 2 || av.Products[0].AllInMinor != 55248 {
		t.Fatalf("availability: %s %s", rec.Header().Get("Cache-Control"), rec.Body.String())
	}
	if rec := call(h, "GET", "/v1/public/events/0e000000-0000-4000-8000-0000000000ff/availability", ""); rec.Code != 404 {
		t.Fatalf("unknown event: %d", rec.Code)
	}
}
