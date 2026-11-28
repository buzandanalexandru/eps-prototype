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
	mu         sync.Mutex
	counters   = map[string]*series{}
	gauges     = map[string]func() float64{}
	histograms = map[string]*Histogram{}
)

// Histogram counts observations in fixed buckets (seconds).
type Histogram struct {
	name, help, labels string
	bounds             []float64
	mu                 sync.Mutex
	counts             []int64
	sum                float64
	n                  int64
}

// DefaultLatencyBuckets are for calls that take milliseconds to seconds.
var DefaultLatencyBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewHistogram returns the histogram for name and labels, creating it once.
func NewHistogram(name, help string, bounds []float64, labelPairs ...string) *Histogram {
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
	if h, ok := histograms[key]; ok {
		return h
	}
	h := &Histogram{name: name, help: help, labels: b.String(), bounds: bounds, counts: make([]int64, len(bounds))}
	histograms[key] = h
	return h
}

// Observe records one value.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, b := range h.bounds {
		if v <= b {
			h.counts[i]++
		}
	}
	h.sum += v
	h.n++
}

func (h *Histogram) write(b *strings.Builder) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sep := ""
	if h.labels != "" {
		sep = ","
	}
	for i, bound := range h.bounds {
		fmt.Fprintf(b, "%s_bucket{%s%sle=\"%g\"} %d\n", h.name, h.labels, sep, bound, h.counts[i])
	}
	fmt.Fprintf(b, "%s_bucket{%s%sle=\"+Inf\"} %d\n", h.name, h.labels, sep, h.n)
	lb := ""
	if h.labels != "" {
		lb = "{" + h.labels + "}"
	}
	fmt.Fprintf(b, "%s_sum%s %g\n%s_count%s %d\n", h.name, lb, h.sum, h.name, lb, h.n)
}

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

// NewGauge registers a gauge read at scrape time. The name may carry labels: name{k="v"}.
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
		typed := map[string]bool{}
		for _, n := range names {
			base, _, _ := strings.Cut(n, "{")
			if !typed[base] {
				fmt.Fprintf(&b, "# TYPE %s gauge\n", base)
				typed[base] = true
			}
			fmt.Fprintf(&b, "%s %g\n", n, gauges[n]())
		}
		hkeys := make([]string, 0, len(histograms))
		for k := range histograms {
			hkeys = append(hkeys, k)
		}
		sort.Strings(hkeys)
		htyped := map[string]bool{}
		for _, k := range hkeys {
			h := histograms[k]
			if !htyped[h.name] {
				fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
				htyped[h.name] = true
			}
			h.write(&b)
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

// RemoveGauge drops a gauge (a sale that left the waiting rooms' index: review sec M5).
func RemoveGauge(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(gauges, name)
}
