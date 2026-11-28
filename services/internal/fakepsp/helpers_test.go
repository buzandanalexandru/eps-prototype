package fakepsp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type hooks struct {
	mu     sync.Mutex
	bodies [][]byte
	sigs   []string
	fail   int // answer 500 to the first n deliveries
}

func (h *hooks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fail > 0 {
		h.fail--
		w.WriteHeader(500)
		return
	}
	h.bodies = append(h.bodies, b)
	h.sigs = append(h.sigs, r.Header.Get("EPS-Signature"))
}

func (h *hooks) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.bodies)
}

type rig struct {
	p     *fakepsp.Provider
	srv   *httptest.Server
	hooks *hooks
	hook  *httptest.Server
}

func newRig(t *testing.T, withRedis bool) *rig {
	t.Helper()
	p := &fakepsp.Provider{ID: "a", APIKey: "api-key", AdminToken: "admin-token", WebhookSecret: []byte("whsec"), RetryBase: 10 * time.Millisecond}
	if withRedis {
		rdb, err := redisx.Open(testdb.RedisURL(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = rdb.FlushAll(context.Background()).Err()
		t.Cleanup(func() { _ = rdb.Close() })
		p.Redis = rdb
	}
	mux := http.NewServeMux()
	p.Routes(mux)
	srv := httptest.NewServer(mux)
	p.PublicURL = srv.URL + "/a"
	h := &hooks{}
	hook := httptest.NewServer(h)
	t.Cleanup(func() { p.Wait(); srv.Close(); hook.Close() })
	r := &rig{p: p, srv: srv, hooks: h, hook: hook}
	r.knobs(t, "m1", fakepsp.Knobs{OutageMode: "hang"}) // no latency in tests
	return r
}

func (r *rig) do(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, r.srv.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+token)
	c := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *rig) knobs(t *testing.T, merchant string, k fakepsp.Knobs) {
	t.Helper()
	if k.OutageMode == "" {
		k.OutageMode = "hang"
	}
	if err := r.p.SetKnobs(context.Background(), merchant, k); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) charge(t *testing.T, key, method string, extra map[string]any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{"amount_minor": 55248, "currency": "RON", "idempotency_key": key, "method": method, "merchant": "m1",
		"return_url": "https://shop.example.test/back", "webhook_url": r.hook.URL + "/hook", "metadata": map[string]string{"attempt_id": key}}
	for k, v := range extra {
		body[k] = v
	}
	return r.do(t, "POST", "/v1/charges", "api-key", body)
}

func newServer(h http.Handler) *httptest.Server { return httptest.NewServer(h) }
