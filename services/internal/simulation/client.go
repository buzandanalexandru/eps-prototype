package simulation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// Minter signs simulated fans' core tokens with the simulator's own key (design D-3).
type Minter struct {
	Key    ed25519.PrivateKey
	Issuer string
}

// Mint makes a 5-minute guest token for one simulated fan of a run.
func (m *Minter) Mint(sub, rehearsal, run, ip string, now time.Time) (string, error) {
	c := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Issuer: m.Issuer, Subject: sub, Audience: jwt.ClaimStrings{auth.Audience},
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))},
		Kind: auth.KindGuest, Sid: "sim-" + run[:8], ClientIP: ip, Rehearsal: rehearsal, Run: run,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	t.Header["kid"] = "sim"
	return t.SignedString(m.Key)
}

// Endpoints are the in-cluster addresses the simulator calls (never the web app).
type Endpoints struct {
	Gateway  string
	Core     string
	Internal string            // the core's internal port
	PSP      map[string]string // provider id → base URL (hosted pages)
	Return   string            // a return address on the web app's origin the core allows
}

// stats keeps the counts and latencies of each step (a bounded sample for the percentiles).
type stats struct {
	mu    sync.Mutex
	steps map[string]*stepStat
	start time.Time
	total int
}

type stepStat struct {
	count, errors int
	sample        []float64
	next          int
}

const sampleSize = 2000

var (
	simRequests = map[string]*metrics.Counter{}
	simLatency  = map[string]*metrics.Histogram{}
	simMu       sync.Mutex
)

func observeMetric(step, result string, d time.Duration) {
	simMu.Lock()
	k := step + "/" + result
	c, ok := simRequests[k]
	if !ok {
		c = metrics.NewCounter("sim_requests_total", "Simulator requests by step and result.", "step", step, "result", result)
		simRequests[k] = c
	}
	h, ok := simLatency[step]
	if !ok {
		h = metrics.NewHistogram("sim_request_seconds", "Simulator request latency by step.", metrics.DefaultLatencyBuckets, "step", step)
		simLatency[step] = h
	}
	simMu.Unlock()
	c.Inc()
	h.Observe(d.Seconds())
}

func (s *stats) observe(step string, d time.Duration, failed bool) {
	res := "ok"
	if failed {
		res = "error"
	}
	observeMetric(step, res, d)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.steps == nil {
		s.steps = map[string]*stepStat{}
	}
	st, ok := s.steps[step]
	if !ok {
		st = &stepStat{}
		s.steps[step] = st
	}
	st.count++
	s.total++
	if failed {
		st.errors++
	}
	ms := float64(d.Microseconds()) / 1000
	if len(st.sample) < sampleSize {
		st.sample = append(st.sample, ms)
	} else {
		st.sample[st.next] = ms
		st.next = (st.next + 1) % sampleSize
	}
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p * float64(len(sorted)-1))
	return sorted[i]
}

func (s *stats) snapshot(now time.Time) (map[string]StepStats, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]StepStats{}
	for k, st := range s.steps {
		c := append([]float64(nil), st.sample...)
		sort.Float64s(c)
		out[k] = StepStats{Count: st.count, Errors: st.errors, P50Ms: round1(pct(c, 0.5)), P95Ms: round1(pct(c, 0.95))}
	}
	rps := 0.0
	if el := now.Sub(s.start).Seconds(); el > 1 {
		rps = round1(float64(s.total) / el)
	}
	return out, rps
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

// answer is one HTTP answer: status, body and a problem's code.
type answer struct {
	status int
	body   []byte
	code   string
	retry  time.Duration
}

func (a answer) ok() bool { return a.status >= 200 && a.status < 300 }

// caller sends requests with a bounded number in flight (SIM_MAX_INFLIGHT).
type caller struct {
	http  *http.Client
	slots chan struct{}
	st    *stats
	pace  *pacer
}

// pacer spaces requests to at most rate a second over all fans (review D-b).
type pacer struct {
	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

func newPacer(rate int) *pacer { return &pacer{gap: time.Second / time.Duration(rate)} }

// wait blocks until the request's turn (false when ctx ended first).
func (p *pacer) wait(ctx context.Context) bool {
	if p == nil {
		return true
	}
	p.mu.Lock()
	now := time.Now()
	if p.next.Before(now) {
		p.next = now
	}
	at := p.next
	p.next = p.next.Add(p.gap)
	p.mu.Unlock()
	return sleep(ctx, time.Until(at))
}

func (c *caller) do(ctx context.Context, step, method, u string, hdr map[string]string, body any) answer {
	var rd io.Reader
	if body != nil {
		switch b := body.(type) {
		case url.Values:
			rd = strings.NewReader(b.Encode())
		default:
			j, _ := json.Marshal(b)
			rd = bytes.NewReader(j)
		}
	}
	if !c.pace.wait(ctx) {
		return answer{status: 0, code: "cancelled"}
	}
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return answer{status: 0, code: "cancelled"}
	}
	defer func() { <-c.slots }()
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return answer{code: "bad_request"}
	}
	if body != nil {
		if _, form := body.(url.Values); form {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			c.st.observe(step, time.Since(start), true)
		}
		return answer{code: "network"}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	c.st.observe(step, time.Since(start), resp.StatusCode >= 500)
	a := answer{status: resp.StatusCode, body: b}
	if resp.StatusCode >= 400 {
		var p struct {
			Code string `json:"code"`
			Data struct {
				RetryAfter int `json:"retryAfter"`
			} `json:"data"`
		}
		_ = json.Unmarshal(b, &p)
		a.code = p.Code
		if p.Data.RetryAfter > 0 {
			a.retry = time.Duration(p.Data.RetryAfter) * time.Second
		}
	}
	return a
}

// sleep waits d or until ctx ends (false then).
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
