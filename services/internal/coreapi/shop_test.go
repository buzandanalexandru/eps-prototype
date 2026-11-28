package coreapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

type shopRig struct {
	env  *salestest.Env
	sale *salestest.Sale
	srv  *coreapi.Server
}

func newShopRig(t *testing.T) *shopRig {
	t.Helper()
	env := salestest.Setup(t)
	ps := &payments.Service{Pool: env.Pool, Checkout: env.Svc, Providers: map[string]payments.Provider{}, Breaker: &payments.Breaker{Redis: env.Redis}}
	w := &wallet.Service{Pool: env.Pool, Redis: env.Redis, Checkout: env.Svc, Payments: ps}
	srv := &coreapi.Server{Pool: env.Pool, Snapshots: env.Store, Shop: &coreapi.Shop{Checkout: env.Svc, Payments: ps, Wallet: w, ReturnOrigins: []string{"https://web.example.test"},
		Redis: env.Redis, HoldsPerMin: 3, LiveHoldsPer: 10}}
	return &shopRig{env: env, sale: env.NewSale(t, 100, 100), srv: srv}
}

func as(sub, kind, ip string) context.Context {
	return auth.WithClaims(context.Background(), &auth.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: sub}, Kind: kind, Sid: "s", ClientIP: ip})
}

func body(t *testing.T, resp any) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	type visitor interface{ VisitGetOrderResponse(http.ResponseWriter) error }
	if v, ok := resp.(visitor); ok {
		_ = v.VisitGetOrderResponse(rec)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

var _ = pgx.ErrNoRows
var _ = db.RoleCore

func uuidOf(t *testing.T, s string) openapi_types.UUID {
	var u openapi_types.UUID
	if err := u.UnmarshalText([]byte(s)); err != nil {
		t.Fatal(err)
	}
	return u
}

func ptr(s string) *string { return &s }

// Review D-f: an expired resume link shows the state only, a month later nothing; a changed link
// is "Order not found" like any unknown order.
func TestExpiredAndChangedLinks(t *testing.T) {
	r := newShopRig(t)
	g := salestest.Guest()
	o := r.sale.Hold(t, g, checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	o = r.sale.Details(t, o)
	links := r.env.Svc.Links
	stranger := as("guest:00000000-0000-4000-8000-0000000000aa", auth.KindGuest, "")
	get := func(tok string) (int, map[string]any) {
		resp, _ := r.srv.GetOrder(stranger, api.GetOrderRequestObject{OrderId: uuidOf(t, o.ID), Params: api.GetOrderParams{XOrderToken: ptr(tok)}})
		return body(t, resp)
	}
	st, v := get(links.Sign(o.ID, checkout.LinkResume, time.Now().Add(-time.Minute)))
	if st != 200 || v["access"] != "expired_link" || v["buyerEmail"] != nil || v["canPay"] != false || len(v["history"].([]any)) != 0 {
		t.Fatalf("expired link: %d %v", st, v)
	}
	if st, v := get(links.Sign(o.ID, checkout.LinkResume, time.Now().Add(-31*24*time.Hour))); st != 404 || v["code"] != "EPS-ORDER-NOT-FOUND" {
		t.Fatalf("a month later: %d %v", st, v)
	}
	if st, v := get("changed.token"); st != 404 || v["code"] != "EPS-ORDER-NOT-FOUND" {
		t.Fatalf("changed link: %d %v", st, v)
	}
}

// Review D-e: a resume-link holder pays by card, Apple Pay or Google Pay only.
func TestLinkHolderCannotPayWithWalletOrSavedCard(t *testing.T) {
	r := newShopRig(t)
	o := r.sale.Details(t, r.sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1}))
	tok := r.env.Svc.Links.ResumeLink(o)
	stranger := as("guest:00000000-0000-4000-8000-0000000000ab", auth.KindGuest, "")
	for _, m := range []api.PayRequestMethod{"wallet", "saved_card"} {
		resp, _ := r.srv.PayOrder(stranger, api.PayOrderRequestObject{OrderId: uuidOf(t, o.ID), Params: api.PayOrderParams{XOrderToken: ptr(tok), IdempotencyKey: "link-pay-key-" + string(m)},
			Body: &api.PayOrderJSONRequestBody{Method: m}})
		if st, v := body(t, resp); st != 422 || v["code"] != "EPS-PAY-METHOD" {
			t.Fatalf("%s through a link: %d %v", m, st, v)
		}
	}
}

// Review D-h: hold creation is limited per client address.
func TestHoldsAreRateLimitedPerClient(t *testing.T) {
	r := newShopRig(t)
	last := 0
	var code any
	for i := 0; i < 4; i++ {
		g := salestest.Guest()
		ctx := as(g.ID, auth.KindGuest, "203.0.113.7")
		req := checkout.CreateRequest{EventID: r.sale.Event, Version: r.sale.Version, Items: []checkout.Item{{ProductID: salestest.ProductGA, Qty: 1}}}
		b := api.CreateOrderJSONRequestBody(req)
		resp, _ := r.srv.CreateOrder(ctx, api.CreateOrderRequestObject{Body: &b})
		st, v := body(t, resp)
		last, code = st, v["code"]
	}
	if last != 429 || code != "EPS-RATE-LIMITED" {
		t.Fatalf("4th hold from one address: %d %v", last, code)
	}
	// Another address is not affected.
	g := salestest.Guest()
	b := api.CreateOrderJSONRequestBody(checkout.CreateRequest{EventID: r.sale.Event, Version: r.sale.Version, Items: []checkout.Item{{ProductID: salestest.ProductGA, Qty: 1}}})
	resp, _ := r.srv.CreateOrder(as(g.ID, auth.KindGuest, "203.0.113.8"), api.CreateOrderRequestObject{Body: &b})
	if st, _ := body(t, resp); st != 201 {
		t.Fatalf("another address: %d", st)
	}
}

// Review D-i: a brand's configured hostname is an allowed provider return origin.
func TestBrandHostnameIsAReturnOrigin(t *testing.T) {
	r := newShopRig(t)
	ctx := context.Background()
	if _, err := r.env.Pool.Exec(ctx, "insert into control.brand_hostnames (hostname, brand_id, organiser_id) values ('shop.brand.example', $1, $2)", r.sale.Brand, r.sale.Org); err != nil {
		t.Fatal(err)
	}
	if !r.srv.ReturnURLAllowed(ctx, r.sale.Org, "https://shop.brand.example/ro/checkout?order=x") {
		t.Fatal("brand hostname refused")
	}
	if !r.srv.ReturnURLAllowed(ctx, r.sale.Org, "https://web.example.test/s/b/ro/checkout") {
		t.Fatal("web origin refused")
	}
	if r.srv.ReturnURLAllowed(ctx, r.sale.Org, "https://evil.example/") || r.srv.ReturnURLAllowed(ctx, r.sale.Org, "http://shop.brand.example/") {
		t.Fatal("another origin allowed")
	}
	other := r.env.NewSale(t, 10, 10)
	if r.srv.ReturnURLAllowed(ctx, other.Org, "https://shop.brand.example/") {
		t.Fatal("another organiser's hostname allowed")
	}
}
