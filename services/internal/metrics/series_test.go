package metrics_test

import (
	"io"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func value(t *testing.T, body, series string) string {
	t.Helper()
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, series+" ") {
			return strings.TrimPrefix(l, series+" ")
		}
	}
	return ""
}

// Scenarios "Metrics present" and "Label check": after one guest checkout with an email, the core's
// series are present and count the paid order, and no metric label or log line holds the email,
// the pass or the session secret.
func TestSeriesAfterAPaidOrderWithoutPersonalData(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	before := scrape(t)
	b := simtest.NewBrowser(s.FanToken(t, "192.0.2.77"))
	b.Admit(t, s, sale.ID)
	o := b.Buy(t, s, ev.Event, ev.Version, 2)
	if o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("order %v", o["state"])
	}
	email := "ana-" + o["id"].(string)[:6] + "@sim.invalid"
	body := scrape(t)
	for _, series := range []string{`eps_orders_paid_total{rehearsal="false"}`, `eps_tickets_sold_total{rehearsal="false"}`, "eps_holds_created_total",
		`eps_payment_attempts_total{provider="a",method="apple_pay",result="succeeded"}`, "eps_payments_succeeded_total"} {
		if value(t, body, series) == "" {
			t.Errorf("missing %s", series)
		}
	}
	if paid := `eps_orders_paid_total{rehearsal="false"}`; value(t, body, paid) == value(t, before, paid) {
		t.Error("the paid order was not counted")
	}
	for _, re := range []string{`eps_http_request_seconds_bucket\{service="core",route="[^"]+"`, `eps_queue_joins_total`, `eps_http_requests_total\{service="gateway"`} {
		if !regexp.MustCompile(re).MatchString(body) {
			t.Errorf("missing %s", re)
		}
	}
	pass, session := b.Secrets()
	logs := s.Logs.String()
	if logs == "" {
		t.Fatal("no logs captured")
	}
	labels := regexp.MustCompile(`\{[^}]*\}`).FindAllString(body, -1)
	for _, secret := range []string{email, pass, session, "192.0.2.77"} {
		for _, l := range labels {
			if strings.Contains(l, secret) {
				t.Fatalf("a metric label holds personal data: %s", l)
			}
		}
		if strings.Contains(logs, secret) {
			t.Fatalf("a log line holds %q", secret[:min(12, len(secret))])
		}
	}
}
