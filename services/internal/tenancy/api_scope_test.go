package tenancy_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tenancy"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

type staticKeys map[string]ed25519.PublicKey

func (k staticKeys) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) { return k, nil }

// probeAPI is an organiser API shaped like the ones later steps add: the organiser comes from the
// token only, the database enforces it, and a foreign id looks exactly like a missing one.
func probeAPI(pool *pgxpool.Pool, reached *int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/probes/{id}", func(w http.ResponseWriter, r *http.Request) {
		*reached++
		org, ok := tenancy.OrganiserFromContext(r.Context())
		if !ok {
			httpx.WriteProblem(w, r, http.StatusForbidden, "organiser_required", "")
			return
		}
		id := r.PathValue("id")
		if !tenancy.ValidID(id) {
			tenancy.WriteNotFound(w, r)
			return
		}
		var label string
		err := db.WithTx(r.Context(), pool, db.RoleCore, org, func(tx pgx.Tx) error {
			err := tx.QueryRow(r.Context(), "select label from sales.probe where id = $1", id).Scan(&label)
			if errors.Is(err, pgx.ErrNoRows) {
				return tenancy.ErrNotFound
			}
			return err
		})
		if tenancy.Handle(w, r, err) {
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "label": label})
	})
	return mux
}

func TestOrganiserAPIScope(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	ids := map[string]string{}
	for _, org := range []string{orgA, orgB} {
		err := db.WithTx(ctx, pool, db.RoleCore, org, func(tx pgx.Tx) error {
			var id string
			err := tx.QueryRow(ctx, "insert into sales.probe (organiser_id, label) values ($1, 'p') returning id::text", org).Scan(&id)
			ids[org] = id
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	v := auth.NewVerifier(staticKeys{"k": pub})
	reached := 0
	h := httpx.Chain(probeAPI(pool, &reached),
		httpx.WithRequestID,
		auth.Middleware(v, slog.New(slog.NewTextHandler(io.Discard, nil))),
		tenancy.RequireOrganiser,
	)
	token := func(kind, sub, org string) string {
		now := time.Now()
		m := jwt.MapClaims{"aud": "eps-core", "sub": sub, "kind": kind, "sid": "s", "iat": now.Unix(), "exp": now.Add(4 * time.Minute).Unix()}
		if org != "" {
			m["org"], m["role"] = org, "owner"
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, m)
		tok.Header["kid"] = "k"
		s, _ := tok.SignedString(priv)
		return s
	}
	call := func(tok, id string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/v1/probes/"+id, nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		delete(body, "requestId")
		return rec.Code, body
	}
	aTok := token("organiser", "user-a", orgA)

	// Own record.
	if code, _ := call(aTok, ids[orgA]); code != http.StatusOK {
		t.Fatalf("own record: %d", code)
	}
	// Scenario "Guessing another organiser's id": 404, identical to an unknown id.
	codeB, bodyB := call(aTok, ids[orgB])
	codeU, bodyU := call(aTok, "11111111-1111-4111-8111-111111111111")
	if codeB != http.StatusNotFound || codeU != http.StatusNotFound {
		t.Fatalf("foreign %d, unknown %d", codeB, codeU)
	}
	bb, _ := json.Marshal(bodyB)
	bu, _ := json.Marshal(bodyU)
	if string(bb) != string(bu) {
		t.Fatalf("foreign and unknown differ:\n%s\n%s", bb, bu)
	}

	// Scenario "Token without organiser": fan and guest get 403 and the handler (where the scope
	// would be set) is never reached.
	before := reached
	for _, tok := range []string{token("fan", "user-f", ""), token("guest", "guest:g", "")} {
		if code, body := call(tok, ids[orgA]); code != http.StatusForbidden || body["code"] != "organiser_required" {
			t.Fatalf("fan/guest: %d %v", code, body)
		}
	}
	if code, _ := call("", ids[orgA]); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if reached != before {
		t.Fatal("handler reached without an organiser token")
	}
}
