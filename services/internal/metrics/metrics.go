// Package metrics exposes a small set of process counters in the Prometheus text format on
// /metrics (served on its own port, outside the ingress). It has no dependencies; step 6 may swap
// in the Prometheus client behind the same names.
package metrics

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing counter.
type Counter struct{ v atomic.Int64 }

// Inc adds one.
func (c *Counter) Inc() { c.v.Add(1) }

// Add adds n (n >= 0).
func (c *Counter) Add(n int64) { c.v.Add(n) }

// Value returns the current value.
func (c *Counter) Value() int64 { return c.v.Load() }

type series struct {
	name, help string
	labels     string // rendered label set, e.g. `code="2xx"`
	c          *Counter
}

var (
	mu       sync.Mutex
	counters = map[string]*series{}
	gauges   = map[string]func() float64{}
)

// NewCounter returns the counter for name and labels (k1, v1, k2, v2, ...), creating it once.
func NewCounter(name, help string, labelPairs ...string) *Counter {
	var b strings.Builder
	for i := 0; i+1 < len(labelPairs); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%q", labelPairs[i], labelPairs[i+1])
	}
	key := name + "{" + b.String() + "}"
	mu.Lock()
	defer mu.Unlock()
	if s, ok := counters[key]; ok {
		return s.c
	}
	s := &series{name: name, help: help, labels: b.String(), c: &Counter{}}
	counters[key] = s
	return s.c
}

// NewGauge registers a gauge read at scrape time.
func NewGauge(name string, f func() float64) {
	mu.Lock()
	defer mu.Unlock()
	gauges[name] = f
}

// Handler serves every registered series.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		mu.Lock()
		keys := make([]string, 0, len(counters))
		for k := range counters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		seen := map[string]bool{}
		var b strings.Builder
		for _, k := range keys {
			s := counters[k]
			if !seen[s.name] {
				fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s counter\n", s.name, s.help, s.name)
				seen[s.name] = true
			}
			if s.labels == "" {
				fmt.Fprintf(&b, "%s %d\n", s.name, s.c.Value())
			} else {
				fmt.Fprintf(&b, "%s{%s} %d\n", s.name, s.labels, s.c.Value())
			}
		}
		names := make([]string, 0, len(gauges))
		for n := range gauges {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(&b, "# TYPE %s gauge\n%s %g\n", n, n, gauges[n]())
		}
		mu.Unlock()
		fmt.Fprintf(&b, "# TYPE go_goroutines gauge\ngo_goroutines %d\n", runtime.NumGoroutine())
		_, _ = w.Write([]byte(b.String()))
	})
}

// HTTP request counters by status class, shared by every binary's access middleware.
func RequestCounter(service string, status int) *Counter {
	class := fmt.Sprintf("%dxx", status/100)
	return NewCounter("eps_http_requests_total", "HTTP requests served, by status class.", "service", service, "code", class)
}
