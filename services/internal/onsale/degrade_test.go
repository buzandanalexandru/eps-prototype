package onsale_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
)

// Scenarios "New hold refused politely" and "Held order still pays": in degraded mode a new hold
// gets EPS-BUSY with Retry-After 30, while an order held before still pays; the availability
// answer says degraded; off again, holds work.
func TestDegradedHoldsAndPayments(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	held := simtest.NewBrowser(s.FanToken(t, "192.0.2.21"))
	held.Admit(t, s, sale.ID)
	code, o := held.Hold(t, s, ev.Event, ev.Version, 2)
	if code != http.StatusCreated {
		t.Fatalf("hold: %d %v", code, o)
	}
	late := simtest.NewBrowser(s.FanToken(t, "192.0.2.22"))
	late.Admit(t, s, sale.ID)
	if _, err := s.Sales.SetDegraded(ctx, ev.Org, sale.ID, "test", true); err != nil {
		t.Fatal(err)
	}
	// The hold of an admitted fan: 503 EPS-BUSY, retry in 30 s (header and body), counted in
	// eps_degrade_refused_total (scenario "Metrics present").
	refusedBefore := refusedCount()
	code, out := late.Hold(t, s, ev.Event, ev.Version, 1)
	if code != http.StatusServiceUnavailable || out["code"] != apperr.Busy || out["retryable"] != true {
		t.Fatalf("degraded hold: %d %v", code, out)
	}
	if got := refusedCount(); got != refusedBefore+1 {
		t.Fatalf("eps_degrade_refused_total %v, want %v", got, refusedBefore+1)
	}
	if d, _ := out["data"].(map[string]any); d["retryAfter"] != 30.0 {
		t.Fatalf("retry time: %v", out["data"])
	}
	resp := rawHold(t, s, late, ev.Event, ev.Version)
	if resp.Header.Get("Retry-After") != "30" {
		t.Fatalf("Retry-After: %q", resp.Header.Get("Retry-After"))
	}
	// The order held before pays normally.
	if o := held.Buy(t, s, ev.Event, ev.Version, 2); o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("held order in degraded mode: %v", o["state"])
	}
	// The shop's availability answer says degraded (the event page shows the busy message).
	if code, out := simtest.Call(t, http.MethodGet, s.Core.URL+"/v1/public/events/"+ev.Event+"/availability", nil, nil); code != http.StatusOK || out["degraded"] != true {
		t.Fatalf("availability: %d %v", code, out["degraded"])
	}
	// Off again: holds work.
	if _, err := s.Sales.SetDegraded(ctx, ev.Org, sale.ID, "test", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // the in-process copy of the sales lives 1 s
	if code, out := late.Hold(t, s, ev.Event, ev.Version, 1); code != http.StatusCreated {
		t.Fatalf("hold after degraded mode: %d %v", code, out)
	}
}

func rawHold(t *testing.T, s *simtest.Stack, b *simtest.Browser, event string, version int) *http.Response {
	t.Helper()
	body := `{"eventId":"` + event + `","version":` + strconv.Itoa(version) + `,"locale":"en","items":[{"productId":"` + salestest.ProductGA + `","qty":1}]}`
	req, _ := http.NewRequest(http.MethodPost, s.Core.URL+"/v1/shop/orders", strings.NewReader(body))
	for k, v := range b.Shop() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

// Scenario "Queue keeps its place": while degraded no one is admitted, positions stay and the
// status says degraded; off again, admissions resume in the same order.
func TestDegradedQueueKeepsPlaces(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	if _, err := s.Sales.SetDegraded(ctx, ev.Org, sale.ID, "test", true); err != nil {
		t.Fatal(err)
	}
	var fans []*simtest.Browser
	for i := 0; i < 3; i++ {
		b := simtest.NewBrowser(s.FanToken(t, "192.0.2."+strconv.Itoa(30+i)))
		if code, out := b.Join(t, s, sale.ID); code != http.StatusOK {
			t.Fatalf("join: %d %v", code, out)
		}
		fans = append(fans, b)
	}
	time.Sleep(3 * time.Second) // admitter ticks every 500 ms
	c, _ := queue.CountsOf(ctx, s.Redis, ev.Org, ev.Event, sale.ID, time.Now())
	if c.Admitted != 0 || c.Line != 3 {
		t.Fatalf("degraded queue: %+v", c)
	}
	st := fans[0].Status(t, s, sale.ID)
	if st["degraded"] != true || st["state"] != "waiting" {
		t.Fatalf("status: %v", st)
	}
	ahead := []float64{}
	for _, b := range fans {
		a, _ := b.Status(t, s, sale.ID)["ahead"].(float64)
		ahead = append(ahead, a)
	}
	if _, err := s.Sales.SetDegraded(ctx, ev.Org, sale.ID, "test", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, _ := queue.CountsOf(ctx, s.Redis, ev.Org, ev.Event, sale.ID, time.Now()); c.Admitted == 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if c, _ := queue.CountsOf(ctx, s.Redis, ev.Org, ev.Event, sale.ID, time.Now()); c.Admitted != 3 {
		t.Fatalf("admissions did not resume: %+v", c)
	}
	if ahead[0] != 0 || ahead[1] != 1 || ahead[2] != 2 {
		t.Fatalf("positions while degraded: %v", ahead)
	}
}

// Review D-d: degraded mode stops new admissions, not the fans already admitted: a pass that ran
// out inside a degraded window gets that window's length again, and no more.
func TestPassesOutliveADegradedWindow(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	fan := simtest.NewBrowser(s.FanToken(t, "192.0.2.41"))
	fan.Admit(t, s, sale.ID)
	if _, err := s.Sales.SetDegraded(ctx, ev.Org, sale.ID, "test", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sales.SetDegraded(ctx, ev.Org, sale.ID, "test", false); err != nil {
		t.Fatal(err)
	}
	// A 4-minute window from 5 to 9 minutes after admission, before the pass (10 minutes) ran out:
	// at 12 minutes it still holds (10 + 4 = 14), at 15 it does not.
	now := time.Now()
	if _, err := s.Pool.Exec(ctx, `update sales.sales set degraded_window_start = $2, degraded_window_end = $3 where id = $1`,
		sale.ID, now.Add(5*time.Minute), now.Add(9*time.Minute)); err != nil {
		t.Fatal(err)
	}
	s.Sales.Refresh(ctx, ev.Org, ev.Event)
	time.Sleep(1100 * time.Millisecond)
	s.Gate.Now = func() time.Time { return now.Add(12 * time.Minute) }
	defer func() { s.Gate.Now = nil }()
	if code, out := fan.Hold(t, s, ev.Event, ev.Version, 1); code != http.StatusCreated {
		t.Fatalf("a pass within its degraded grace: %d %v", code, out)
	}
	fan2 := simtest.NewBrowser(s.FanToken(t, "192.0.2.42"))
	fan2.Admit(t, s, sale.ID)
	s.Gate.Now = func() time.Time { return now.Add(15 * time.Minute) }
	if code, out := fan2.Hold(t, s, ev.Event, ev.Version, 1); code != http.StatusForbidden || out["code"] != apperr.PassRequired {
		t.Fatalf("a pass past its grace: %d %v", code, out)
	}
}

func refusedCount() float64 {
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	b, _ := io.ReadAll(rec.Body)
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, "eps_degrade_refused_total "); ok {
			n, _ := strconv.ParseFloat(v, 64)
			return n
		}
	}
	return 0
}
