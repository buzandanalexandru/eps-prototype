package live

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Prom asks the environment's Prometheus server for the system strip (instant queries over the
// last 30 s; PROMETHEUS_URL). Rates lag by a scrape or two; the on-sale numbers never come from it.
type Prom struct {
	BaseURL string
	Client  *http.Client
}

// Queries of the system strip (the series of design D-5).
const (
	qGatewayRPS = `sum(rate(eps_http_requests_total{service="gateway"}[30s]))`
	qCoreP95    = `histogram_quantile(0.95, sum by (le) (rate(eps_http_request_seconds_bucket{service="core"}[30s])))`
	// Deliberate refusals (degraded mode's EPS-BUSY) are not errors (review D-d).
	qCoreErrors = `clamp_min(sum(rate(eps_http_requests_total{service="core",code="5xx"}[30s])) - sum(rate(eps_degrade_refused_total[30s]) or vector(0)), 0) / clamp_min(sum(rate(eps_http_requests_total{service="core"}[30s])), 1e-9)`
	qPods       = `count by (app) (up{app=~"gateway|core|worker"} == 1)`
)

type promAnswer struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]any            `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (p *Prom) query(ctx context.Context, q string) (*promAnswer, error) {
	c := p.Client
	if c == nil {
		c = &http.Client{Timeout: 700 * time.Millisecond}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/api/v1/query?query="+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var a promAnswer
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

func value(v [2]any) (float64, bool) {
	s, ok := v[1].(string)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func (p *Prom) scalar(ctx context.Context, q string) *float64 {
	a, err := p.query(ctx, q)
	if err != nil || a.Status != "success" || len(a.Data.Result) == 0 {
		return nil
	}
	if f, ok := value(a.Data.Result[0].Value); ok {
		return &f
	}
	return nil
}

// System reads the strip (nil when Prometheus does not answer at all).
func (p *Prom) System(ctx context.Context) *System {
	if p == nil || p.BaseURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	s := &System{Pods: map[string]int{}}
	a, err := p.query(ctx, qPods)
	if err != nil {
		return nil
	}
	for _, r := range a.Data.Result {
		if f, ok := value(r.Value); ok {
			s.Pods[r.Metric["app"]] = int(f)
		}
	}
	s.GatewayRPS = p.scalar(ctx, qGatewayRPS)
	if v := p.scalar(ctx, qCoreP95); v != nil {
		ms := *v * 1000
		s.CoreP95Ms = &ms
	}
	s.CoreErrorShare = p.scalar(ctx, qCoreErrors)
	return s
}
