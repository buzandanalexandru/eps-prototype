package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
)

type keys map[string]ed25519.PublicKey

func (k keys) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) { return k, nil }

func newCore(t *testing.T) (http.Handler, string) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	h, err := coreapi.NewHandler(&coreapi.Server{Service: "core"}, coreapi.Options{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Verifier: auth.NewVerifier(keys{"k1": pub}, auth.IssuerForEnv("test")),
		CORS:     httpx.NewCORS(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"iss": auth.IssuerForEnv("test"), "aud": "eps-core", "sub": "user-1", "kind": "fan", "sid": "s1", "jti": "j",
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	})
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return h, s
}

// Scenario "Go server matches the contract": every operation in the contract has a registered
// handler that answers with a status the contract declares.
func TestEveryOperationHasAHandler(t *testing.T) {
	spec, err := api.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	h, token := newCore(t)
	n := 0
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			n++
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("%s %s (%s): no handler (status %d)", method, path, op.OperationID, rec.Code)
				continue
			}
			declared := op.Responses.Value(strconv.Itoa(rec.Code)) != nil || op.Responses.Default() != nil
			if !declared {
				t.Errorf("%s %s answered %d, which the contract does not declare", method, path, rec.Code)
			}
		}
	}
	if n < 4 {
		t.Fatalf("only %d operations found in the contract", n)
	}
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content type %q", ct)
	}
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Code != code || p.Status != status || p.RequestId == "" || p.Type == "" || p.Title == "" {
		t.Fatalf("problem %+v", p)
	}
}

// Scenario "Go server matches the contract": requests that do not match the contract's schema are
// rejected with problem+json.
func TestSchemaInvalidRequestsGetProblemJSON(t *testing.T) {
	h, _ := newCore(t)
	do := func(method, target, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	assertProblem(t, do("GET", "/readyz?verbose=perhaps", ""), http.StatusBadRequest, "invalid_request")
	assertProblem(t, do("GET", "/v1/whoami", ""), http.StatusUnauthorized, "unauthenticated")
	assertProblem(t, do("GET", "/v1/whoami", "Bearer forged.token.value"), http.StatusUnauthorized, "unauthenticated")
	assertProblem(t, do("GET", "/v1/nope", ""), http.StatusNotFound, "not_found")
	assertProblem(t, do("POST", "/v1/version", ""), http.StatusMethodNotAllowed, "method_not_allowed")

	// The 401 carries no identity details or verification reasons.
	rec := do("GET", "/v1/whoami", "Bearer forged.token.value")
	if strings.Contains(rec.Body.String(), "sub") || strings.Contains(rec.Body.String(), "signature") {
		t.Fatalf("401 leaks details: %s", rec.Body.String())
	}
}

func TestWhoamiEchoesVerifiedClaims(t *testing.T) {
	h, token := newCore(t)
	req := httptest.NewRequest("GET", "/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var id api.Identity
	if err := json.Unmarshal(rec.Body.Bytes(), &id); err != nil || rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if id.Sub != "user-1" || id.Kind != "fan" || id.Org != nil || id.Sid != "s1" || id.Exp == 0 {
		t.Fatalf("identity %+v", id)
	}
}

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func generate(t *testing.T, specPath string) []byte {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "api.gen.go")
	cfg, err := os.ReadFile("oapi-codegen.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte(strings.Replace(string(cfg), "output: api.gen.go", "output: "+out, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "tool", "oapi-codegen", "-config", cfgPath, specPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("oapi-codegen: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Scenario "Contract changed without regenerating": the committed stubs equal a fresh generation
// from the contract, and a hand edit to the contract makes them differ (the drift check, which
// also covers the TypeScript types, is scripts/check-contract-drift.sh).
func TestCommittedStubsMatchContract(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the generator")
	}
	spec := repoFile(t, "api/openapi.yaml")
	committed, err := os.ReadFile("api.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generate(t, spec), committed) {
		t.Fatal("services/internal/api/api.gen.go differs from api/openapi.yaml: run `make -C services gen`")
	}
	raw, _ := os.ReadFile(spec)
	edited := filepath.Join(t.TempDir(), "openapi.yaml")
	mod := strings.Replace(string(raw), "summary: Readiness", "summary: Readiness (edited by hand)", 1)
	if err := os.WriteFile(edited, []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(generate(t, edited), committed) {
		t.Fatal("a hand edit to the contract did not change the generated stubs")
	}
}
