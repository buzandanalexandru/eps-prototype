package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRendersCountersAndGauges(t *testing.T) {
	RequestCounter("core", 200).Inc()
	RequestCounter("core", 204).Inc()
	RequestCounter("core", 503).Inc()
	NewCounter("eps_test_total", "Test counter.").Add(3)
	NewGauge("eps_test_gauge", func() float64 { return 1.5 })
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	for _, want := range []string{
		`eps_http_requests_total{service="core",code="2xx"} 2`,
		`eps_http_requests_total{service="core",code="5xx"} 1`,
		"eps_test_total 3",
		"eps_test_gauge 1.5",
		"# TYPE eps_http_requests_total counter",
		"go_goroutines ",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}
