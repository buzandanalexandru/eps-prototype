package fakepsp_test

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
)

// Scenario "Outage switched on": within 2 seconds every new call fails or hangs until the client
// gives up; switching it off restores the provider.
func TestOutageSwitchedOnAndOff(t *testing.T) {
	r := newRig(t, true)
	for _, mode := range []string{"503", "hang"} {
		r.knobs(t, "m1", fakepsp.Knobs{Outage: true, OutageMode: mode})
		time.Sleep(1100 * time.Millisecond) // past the 1 s knob cache: well within 2 s
		start := time.Now()
		st, _ := r.charge(t, fmt.Sprintf("outage-%s-1", mode), fakepsp.HostedCard, nil)
		if mode == "503" && st != http.StatusServiceUnavailable {
			t.Fatalf("503 outage answered %d", st)
		}
		if mode == "hang" && (st != 0 || time.Since(start) < 2*time.Second) {
			t.Fatalf("hang outage answered %d after %s (want the client timeout)", st, time.Since(start))
		}
		// Another merchant is not affected.
		r.knobs(t, "m2", fakepsp.Knobs{})
		if st, _ := r.charge(t, fmt.Sprintf("outage-%s-m2", mode), fakepsp.HostedCard, map[string]any{"merchant": "m2"}); st != 201 {
			t.Fatalf("another merchant during the outage: %d", st)
		}
	}
	r.knobs(t, "m1", fakepsp.Knobs{})
	time.Sleep(1100 * time.Millisecond)
	if st, _ := r.charge(t, "outage-off-1", fakepsp.HostedCard, nil); st != 201 {
		t.Fatalf("after the outage: %d", st)
	}
}

// Scenario "Failure rate": 0.3 gives between 250 and 350 failures of 1,000 charges.
func TestFailureRate(t *testing.T) {
	r := newRig(t, false)
	r.knobs(t, "m1", fakepsp.Knobs{FailureRate: 0.3})
	var failed, ok atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			st, _ := r.charge(t, fmt.Sprintf("rate-%05d", i), fakepsp.HostedCard, nil)
			switch st {
			case http.StatusServiceUnavailable:
				failed.Add(1)
			case http.StatusCreated:
				ok.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if f := failed.Load(); f < 250 || f > 350 || f+ok.Load() != 1000 {
		t.Fatalf("%d failed, %d created", f, ok.Load())
	}
}

// Decline rate, latency, timeout and webhook knobs.
func TestOtherKnobs(t *testing.T) {
	r := newRig(t, true)
	r.knobs(t, "m1", fakepsp.Knobs{LatencyMs: 300})
	time.Sleep(1100 * time.Millisecond)
	start := time.Now()
	if st, _ := r.charge(t, "latency-001", fakepsp.HostedCard, nil); st != 201 || time.Since(start) < 300*time.Millisecond {
		t.Fatalf("latency: %d after %s", st, time.Since(start))
	}
	r.knobs(t, "m1", fakepsp.Knobs{DeclineRate: 1, WebhookDropRate: 1})
	time.Sleep(1100 * time.Millisecond)
	_, ch := r.charge(t, "decline-001", fakepsp.HostedCard, nil)
	approveOnPage(t, r, ch["charge_id"].(string), "approve")
	_, got := r.do(t, "GET", "/v1/charges/"+ch["charge_id"].(string), "api-key", nil)
	if got["status"] != "declined" || got["decline_reason"] != "insufficient_funds" {
		t.Fatalf("decline knob: %v", got)
	}
	time.Sleep(100 * time.Millisecond)
	if r.hooks.count() != 0 || r.p.Stats()["webhooks_dropped"] != 1 {
		t.Fatalf("drop knob: %d delivered, stats %v", r.hooks.count(), r.p.Stats())
	}
	// Timeout: the charge is accepted but the answer never comes; the status check finds it.
	r.knobs(t, "m1", fakepsp.Knobs{TimeoutRate: 1})
	time.Sleep(1100 * time.Millisecond)
	if st, _ := r.charge(t, "timeout-001", fakepsp.HostedCard, nil); st != 0 {
		t.Fatalf("timeout knob answered %d", st)
	}
	if st, got := r.do(t, "GET", "/v1/charges?idempotency_key=timeout-001", "api-key", nil); st != 200 || got["status"] != "pending" {
		t.Fatalf("the timed-out charge exists: %d %v", st, got)
	}
	// Admin knobs API: token required, values validated.
	if st, _ := r.do(t, "PUT", "/admin/knobs?merchant=m1", "admin-token", map[string]any{"failure_rate": 2}); st != 400 {
		t.Fatalf("invalid knob accepted: %d", st)
	}
	if st, _ := r.do(t, "PUT", "/admin/knobs?merchant=m1", "api-key", map[string]any{"outage": true}); st != 401 {
		t.Fatalf("knobs without the admin token: %d", st)
	}
	if st, got := r.do(t, "PUT", "/admin/knobs?merchant=m1", "admin-token", map[string]any{"outage": true, "outage_mode": "503"}); st != 200 || got["outage"] != true {
		t.Fatalf("knobs: %d %v", st, got)
	}
}
