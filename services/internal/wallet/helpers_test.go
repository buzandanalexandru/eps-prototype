package wallet_test

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
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type rig struct {
	env  *salestest.Env
	sale *salestest.Sale
	pay  *payments.Service
	w    *wallet.Service
	psp  *fakepsp.Provider
}

func newRig(t *testing.T) *rig {
	t.Helper()
	env := salestest.Setup(t)
	r := &rig{env: env, sale: env.NewSale(t, 1000, 1000)}
	r.pay = &payments.Service{Pool: env.Pool, Checkout: env.Svc, Providers: map[string]payments.Provider{}, Breaker: &payments.Breaker{Redis: env.Redis}, Log: env.Log}
	r.w = &wallet.Service{Pool: env.Pool, Redis: env.Redis, Checkout: env.Svc, Payments: r.pay}
	r.pay.Topups = r.w
	hooks := &payments.Webhooks{Svc: r.pay, Secrets: map[string][]byte{"a": []byte("s")}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/webhooks/psp/{provider}", hooks.Handle)
	core := httptest.NewServer(mux)
	r.pay.WebhookURL = func(p string) string { return core.URL + "/v1/webhooks/psp/" + p }
	r.psp = &fakepsp.Provider{ID: "a", APIKey: "k", AdminToken: "x", WebhookSecret: []byte("s"), Redis: env.Redis}
	m := http.NewServeMux()
	r.psp.Routes(m)
	srv := httptest.NewServer(m)
	r.psp.PublicURL = srv.URL + "/a"
	_ = r.psp.SetKnobs(context.Background(), r.sale.Org, fakepsp.Knobs{OutageMode: "hang"})
	r.pay.Providers["a"] = &payments.HTTPProvider{ID: "a", BaseURL: srv.URL, APIKey: "k", Timeout: time.Second}
	t.Cleanup(func() { r.psp.Wait(); srv.Close(); core.Close() })
	return r
}

// fund credits a fan's wallet as a confirmed top-up would.
func (r *rig) fund(t *testing.T, org, owner string, amount int64) {
	t.Helper()
	err := db.WithTx(context.Background(), r.env.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		bal, err := wallet.AccountTx(context.Background(), tx, org, wallet.FanBalance, owner, "RON")
		if err != nil {
			return err
		}
		clr, err := wallet.AccountTx(context.Background(), tx, org, wallet.PSPClearing, "a", "RON")
		if err != nil {
			return err
		}
		_, err = wallet.PostTx(context.Background(), tx, org, "topup", "test", "test top-up", "RON", []wallet.Line{{AccountID: clr, AmountMinor: -amount}, {AccountID: bal, AmountMinor: amount}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *rig) balance(t *testing.T, org, owner string) int64 {
	t.Helper()
	w, err := r.w.Wallet(context.Background(), org, owner, "RON")
	if err != nil {
		t.Fatal(err)
	}
	return w.BalanceMinor
}

// order holds `product` for buyer with details given.
func (r *rig) order(t *testing.T, sale *salestest.Sale, b checkout.Buyer, product string, qty int) *checkout.Order {
	t.Helper()
	return sale.Details(t, sale.Hold(t, b, checkout.Item{ProductID: product, Qty: qty}))
}

func (r *rig) audit(t *testing.T, org string) {
	t.Helper()
	probs, err := r.w.Audit(context.Background(), org)
	if err != nil || len(probs) != 0 {
		t.Fatalf("ledger audit: %v %v", probs, err)
	}
}

func approve(t *testing.T, actionURL, outcome string) {
	t.Helper()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.PostForm(actionURL, url.Values{"outcome": {outcome}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
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

// stand charges at a stand with a fresh pay code from the fan.
func (r *rig) stand(t *testing.T, org, owner string, amount int64, label string) (*wallet.Wallet, error) {
	t.Helper()
	code, pc, _, err := r.w.PayCode(context.Background(), org, owner, "RON")
	if err != nil {
		t.Fatal(err)
	}
	return r.w.Stand(context.Background(), org, code, pc, amount, label, "stand-"+pc+"-"+label)
}
