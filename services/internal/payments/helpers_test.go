package payments_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// rig is a core with two real simulated providers across HTTP: the providers post signed
// webhooks to the core's webhook handler.
type rig struct {
	env   *salestest.Env
	sale  *salestest.Sale
	svc   *payments.Service
	psp   map[string]*fakepsp.Provider
	srv   map[string]*httptest.Server
	hooks *payments.Webhooks
	core  *httptest.Server
}

func newRig(t *testing.T) *rig {
	t.Helper()
	env := salestest.Setup(t)
	r := &rig{env: env, psp: map[string]*fakepsp.Provider{}, srv: map[string]*httptest.Server{}}
	r.sale = env.NewSale(t, 1000, 1000)
	r.svc = &payments.Service{Pool: env.Pool, Checkout: env.Svc, Providers: map[string]payments.Provider{}, Breaker: &payments.Breaker{Redis: env.Redis}, Log: env.Log}
	r.hooks = &payments.Webhooks{Svc: r.svc, Secrets: map[string][]byte{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/webhooks/psp/{provider}", r.hooks.Handle)
	r.core = httptest.NewServer(mux)
	r.svc.WebhookURL = func(p string) string { return r.core.URL + "/v1/webhooks/psp/" + p }
	for _, id := range []string{"a", "b"} {
		p := &fakepsp.Provider{ID: id, APIKey: "k", AdminToken: "admin", WebhookSecret: []byte("whsec-" + id), Redis: env.Redis, RetryBase: 20 * time.Millisecond}
		m := http.NewServeMux()
		p.Routes(m)
		s := httptest.NewServer(m)
		p.PublicURL = s.URL + "/" + id
		r.psp[id], r.srv[id] = p, s
		r.svc.Providers[id] = &payments.HTTPProvider{ID: id, BaseURL: s.URL, APIKey: "k", Timeout: 700 * time.Millisecond}
		r.hooks.Secrets[id] = []byte("whsec-" + id)
		r.knobs(t, id, fakepsp.Knobs{})
	}
	t.Cleanup(func() {
		for _, p := range r.psp {
			p.Wait()
		}
		for _, s := range r.srv {
			s.Close()
		}
		r.core.Close()
	})
	return r
}

func (r *rig) knobs(t *testing.T, provider string, k fakepsp.Knobs) {
	t.Helper()
	if k.OutageMode == "" {
		k.OutageMode = "hang"
	}
	if err := r.psp[provider].SetKnobs(context.Background(), r.sale.Org, k); err != nil {
		t.Fatal(err)
	}
}

// order holds 1 GA ticket for a fresh account buyer with details given.
func (r *rig) order(t *testing.T) (checkout.Buyer, *checkout.Order) {
	t.Helper()
	b := salestest.Buyer()
	o := r.sale.Hold(t, b, checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	return b, r.sale.Details(t, o)
}

func (r *rig) get(t *testing.T, orderID string) *checkout.Order {
	t.Helper()
	o, err := r.env.Svc.GetIn(context.Background(), r.sale.Org, orderID)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// press is one Pay press.
func (r *rig) press(t *testing.T, b checkout.Buyer, o *checkout.Order, method, key string) (*payments.PayResult, error) {
	t.Helper()
	return r.svc.Pay(context.Background(), b, o, payments.PayRequest{Method: method, ReturnURL: "https://shop.example.test/s/x/en/checkout?order=" + o.ID}, key)
}

// pagePost chooses an outcome on the hosted page, as the fan's browser does.
func (r *rig) pagePost(t *testing.T, actionURL, outcome string) {
	t.Helper()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.PostForm(actionURL, url.Values{"outcome": {outcome}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func (r *rig) waitState(t *testing.T, orderID, state string) *checkout.Order {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if o := r.get(t, orderID); o.State == state {
			return o
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("order %s is %s, want %s", orderID, r.get(t, orderID).State, state)
	return nil
}

func (r *rig) attempts(t *testing.T, orderID string) []payments.Attempt {
	t.Helper()
	var out []payments.Attempt
	err := db.WithTx(context.Background(), r.env.Pool, db.RoleCore, r.sale.Org, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), "select id::text, provider, state, coalesce(error_code, '') from sales.payment_attempts where order_id = $1 order by created_at", orderID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a payments.Attempt
			var code string
			if err := rows.Scan(&a.ID, &a.Provider, &a.State, &code); err != nil {
				return err
			}
			if code != "" {
				a.ErrorCode = &code
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) events(t *testing.T, typ string) int {
	t.Helper()
	var n int
	err := db.WithTx(context.Background(), r.env.Pool, db.RoleCore, r.sale.Org, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "select count(*) from sales.outbox where type = $1", typ).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func code(err error) string {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
