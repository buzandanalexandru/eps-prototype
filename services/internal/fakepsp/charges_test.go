package fakepsp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
)

func waitHooks(t *testing.T, r *rig, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.hooks.count() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%d webhooks, want %d", r.hooks.count(), n)
}

func approveOnPage(t *testing.T, r *rig, id, outcome string) *http.Response {
	t.Helper()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.PostForm(r.srv.URL+"/a/pay/"+id, url.Values{"outcome": {outcome}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestChargeLifecycleIdempotencyAndSignedWebhook(t *testing.T) {
	r := newRig(t, true)
	st, ch := r.charge(t, "attempt-0001", fakepsp.HostedCard, nil)
	if st != 201 || ch["status"] != "pending" || !strings.Contains(ch["action_url"].(string), "/a/pay/") {
		t.Fatalf("create: %d %v", st, ch)
	}
	id := ch["charge_id"].(string)
	// The same key again returns the same charge, not a second one.
	st2, again := r.charge(t, "attempt-0001", fakepsp.HostedCard, nil)
	if st2 != 200 || again["charge_id"] != id {
		t.Fatalf("replay: %d %v", st2, again)
	}
	// Found by id and by idempotency key; an unknown key is "no such charge".
	if st, got := r.do(t, "GET", "/v1/charges/"+id, "api-key", nil); st != 200 || got["charge_id"] != id {
		t.Fatalf("by id: %d", st)
	}
	if st, got := r.do(t, "GET", "/v1/charges?idempotency_key=attempt-0001", "api-key", nil); st != 200 || got["charge_id"] != id {
		t.Fatalf("by key: %d", st)
	}
	if st, got := r.do(t, "GET", "/v1/charges?idempotency_key=attempt-9999", "api-key", nil); st != 404 || got["error"] != "no_such_charge" {
		t.Fatalf("unknown key: %d %v", st, got)
	}
	// The fan approves on the hosted page: back to the shop, webhook signed.
	r.hooks.fail = 2 // the first two deliveries fail: retried
	resp := approveOnPage(t, r, id, "approve")
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "https://shop.example.test/back" {
		t.Fatalf("redirect %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	waitHooks(t, r, 1)
	var ev struct {
		Type   string `json:"type"`
		Charge struct {
			ID     string `json:"charge_id"`
			Status string `json:"status"`
		} `json:"charge"`
	}
	_ = json.Unmarshal(r.hooks.bodies[0], &ev)
	if ev.Type != "charge.succeeded" || ev.Charge.ID != id {
		t.Fatalf("webhook %+v", ev)
	}
	sig := r.hooks.sigs[0]
	tpart := strings.TrimPrefix(strings.Split(sig, ",")[0], "t=")
	ts, _ := time.Parse("2006", "2006")
	_ = ts
	unix, _ := parseInt(tpart)
	if fakepsp.Sign([]byte("whsec"), time.Unix(unix, 0), r.hooks.bodies[0]) != sig {
		t.Fatal("webhook signature does not verify")
	}
	// Refund, idempotent.
	if st, got := r.do(t, "POST", "/v1/refunds", "api-key", map[string]string{"charge_id": id, "idempotency_key": "refund-0001"}); st != 200 || got["status"] != "refunded" {
		t.Fatalf("refund %d %v", st, got)
	}
	if st, got := r.do(t, "POST", "/v1/refunds", "api-key", map[string]string{"charge_id": id, "idempotency_key": "refund-0001"}); st != 200 || got["status"] != "refunded" {
		t.Fatalf("refund replay %d %v", st, got)
	}
	// Without the API key nothing answers.
	if st, _ := r.do(t, "GET", "/v1/charges/"+id, "wrong", nil); st != 401 {
		t.Fatalf("no key: %d", st)
	}
}

func parseInt(s string) (int64, error) {
	var n int64
	for _, c := range s {
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// Charges are kept in Redis: a restarted provider still knows them.
func TestChargesSurviveARestart(t *testing.T) {
	r := newRig(t, true)
	_, ch := r.charge(t, "attempt-0002", fakepsp.HostedCard, nil)
	fresh := &fakepsp.Provider{ID: "a", APIKey: "api-key", Redis: r.p.Redis}
	mux := http.NewServeMux()
	fresh.Routes(mux)
	srv := &rig{p: fresh, srv: newServer(mux)}
	defer srv.srv.Close()
	if st, got := srv.do(t, "GET", "/v1/charges?idempotency_key=attempt-0002", "api-key", nil); st != 200 || got["charge_id"] != ch["charge_id"] {
		t.Fatalf("after restart: %d %v", st, got)
	}
}

// Saved cards: a succeeded charge with save_card returns a token; the token pays in one tap for
// the same customer and merchant only, and a revoked token pays nothing.
func TestSavedCards(t *testing.T) {
	r := newRig(t, true)
	_, ch := r.charge(t, "attempt-0003", fakepsp.HostedCard, map[string]any{"save_card": true, "customer_ref": "fan-1"})
	approveOnPage(t, r, ch["charge_id"].(string), "approve")
	_, got := r.do(t, "GET", "/v1/charges/"+ch["charge_id"].(string), "api-key", nil)
	saved, _ := got["saved_card"].(map[string]any)
	if saved == nil || saved["label"] != "Test Visa •••• 4242" {
		t.Fatalf("no saved card: %v", got)
	}
	tok := saved["payment_method_token"].(string)
	st, one := r.charge(t, "attempt-0004", fakepsp.SavedCard, map[string]any{"payment_method_token": tok, "customer_ref": "fan-1"})
	if st != 201 || one["status"] != "succeeded" {
		t.Fatalf("one tap: %d %v", st, one)
	}
	if st, _ := r.charge(t, "attempt-0005", fakepsp.SavedCard, map[string]any{"payment_method_token": tok, "customer_ref": "fan-2"}); st != 402 {
		t.Fatalf("another customer's token: %d", st)
	}
	if st, _ := r.do(t, "DELETE", "/v1/payment-methods/"+tok, "api-key", nil); st != 200 {
		t.Fatalf("revoke %d", st)
	}
	if st, _ := r.charge(t, "attempt-0006", fakepsp.SavedCard, map[string]any{"payment_method_token": tok, "customer_ref": "fan-1"}); st != 402 {
		t.Fatalf("revoked token: %d", st)
	}
	_ = context.Background()
}
