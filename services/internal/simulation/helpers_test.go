package simulation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// browser is one client of the stack over HTTP (a person or a simulated fan).
type browser struct {
	token, session, tab string
	queueToken, pass    string
}

func newBrowser(token string) *browser {
	return &browser{token: token, session: strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", ""), tab: "t1"}
}

func call(t *testing.T, method, u string, hdr map[string]string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, u, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

// join takes a place in a sale's waiting room through the gateway.
func (b *browser) join(t *testing.T, s *simtest.Stack, saleID string) (int, map[string]any) {
	t.Helper()
	code, out := call(t, http.MethodPost, s.Gateway.URL+"/v1/queue/"+saleID+"/join",
		map[string]string{"Authorization": "Bearer " + b.token, "X-EPS-Session": b.session, "X-EPS-Tab": b.tab, "X-EPS-Client-UA": "Mozilla/5.0 (Macintosh) Safari/605"},
		map[string]any{"pow": queue.SolvePow(saleID, queue.SessionHash(b.session))})
	if qt, _ := out["queueToken"].(string); qt != "" {
		b.queueToken = qt
	}
	if p, _ := out["pass"].(string); p != "" {
		b.pass = p
	}
	return code, out
}

// admit joins and polls (the stack's admitter runs for real) until the pass is given.
func (b *browser) admit(t *testing.T, s *simtest.Stack, saleID string) {
	t.Helper()
	if code, out := b.join(t, s, saleID); code != http.StatusOK {
		t.Fatalf("join: %d %v", code, out)
	}
	deadline := time.Now().Add(40 * time.Second)
	for b.pass == "" && time.Now().Before(deadline) {
		time.Sleep(2100 * time.Millisecond)
		_, out := call(t, http.MethodGet, s.Gateway.URL+"/v1/queue/"+saleID+"/status",
			map[string]string{"X-EPS-Queue-Token": b.queueToken, "X-EPS-Session": b.session, "X-EPS-Tab": b.tab}, nil)
		if p, _ := out["pass"].(string); p != "" {
			b.pass = p
		}
	}
	if b.pass == "" {
		t.Fatal("not admitted")
	}
}

func (b *browser) shop() map[string]string {
	h := map[string]string{"Authorization": "Bearer " + b.token, "X-EPS-Session": b.session, "Idempotency-Key": "k-" + uuid.NewString()}
	if b.pass != "" {
		h["X-EPS-Pass"] = b.pass
	}
	return h
}

// hold creates an order of n General Access tickets on an event.
func (b *browser) hold(t *testing.T, s *simtest.Stack, event string, version, n int) (int, map[string]any) {
	t.Helper()
	return call(t, http.MethodPost, s.Core.URL+"/v1/shop/orders", b.shop(),
		map[string]any{"eventId": event, "version": version, "locale": "en", "items": []any{map[string]any{"productId": salestest.ProductGA, "qty": n}}})
}

// buy holds, gives details, pays by Apple Pay (approving on the provider's page) and waits until
// the order is final; it returns the order.
func (b *browser) buy(t *testing.T, s *simtest.Stack, event string, version, n int) map[string]any {
	t.Helper()
	code, o := b.hold(t, s, event, version, n)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("hold: %d %v", code, o)
	}
	id := o["id"].(string)
	if code, out := call(t, http.MethodPut, s.Core.URL+"/v1/shop/orders/"+id+"/details", b.shop(), map[string]any{"name": "Ana", "email": "ana-" + id[:6] + "@sim.invalid"}); code != http.StatusOK {
		t.Fatalf("details: %d %v", code, out)
	}
	code, res := call(t, http.MethodPost, s.Core.URL+"/v1/shop/orders/"+id+"/payments", b.shop(), map[string]any{"method": "apple_pay", "returnUrl": "http://localhost:3000/back"})
	if code != http.StatusOK {
		t.Fatalf("pay: %d %v", code, res)
	}
	if action, _ := res["actionUrl"].(string); action != "" {
		u, _ := url.Parse(action)
		base := s.PSPA.URL
		if strings.HasPrefix(u.Path, "/b/") {
			base = s.PSPB.URL
		}
		resp, err := http.PostForm(base+u.Path, url.Values{"outcome": {"approve"}})
		if err == nil {
			resp.Body.Close()
		}
	}
	for i := 0; i < 100; i++ {
		_, cur := call(t, http.MethodGet, s.Core.URL+"/v1/shop/orders/"+id, b.shop(), nil)
		switch cur["state"] {
		case "paid", "issued", "payment_failed", "expired":
			return cur
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("order not final")
	return nil
}

// openRehearsal makes a rehearsal of a real sale and opens it a second ago.
func openRehearsal(t *testing.T, s *simtest.Stack, real *sales.Sale) *sales.Sale {
	t.Helper()
	ctx := context.Background()
	rh, err := s.Sim.CreateRehearsal(ctx, real.OrganiserID, real.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	rh, err = s.Sales.OpenRehearsal(ctx, rh.OrganiserID, rh.ID, "test", time.Now().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return rh
}

func count(t *testing.T, s *simtest.Stack, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
