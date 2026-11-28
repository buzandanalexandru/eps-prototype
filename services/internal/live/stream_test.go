package live_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/live"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
)

func open(t *testing.T, s *simtest.Stack, saleID, token string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.Gateway.URL+"/v1/live/"+saleID, nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Origin", "https://studio.test")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return resp, cancel
}

// Scenario "Frame rate": a watched sale delivers at least 15 frames in 30 seconds, after the
// stream's retry hint; the stream outlives the 30-second write deadline of other requests.
func TestFrameRate(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := s.Context(t)
	s.RunAggregator(ctx)
	resp, cancel := open(t, s, sale.ID, s.OrganiserToken(t, ev.Org))
	defer cancel()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("Access-Control-Allow-Origin") != "https://studio.test" {
		t.Fatalf("stream: %d %v", resp.StatusCode, resp.Header)
	}
	frames, retry := 0, false
	done := time.After(32 * time.Second)
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
loop:
	for {
		select {
		case <-done:
			break loop
		case l, ok := <-lines:
			if !ok {
				break loop
			}
			if l == "retry: 2000" {
				retry = true
			}
			if l == "event: frame" {
				frames++
			}
		}
	}
	if !retry || frames < 15 {
		t.Fatalf("retry %v, %d frames in 32 s", retry, frames)
	}
	// A new connection starts with the history and the latest frame.
	resp2, cancel2 := open(t, s, sale.ID, s.OrganiserToken(t, ev.Org))
	defer cancel2()
	defer resp2.Body.Close()
	sc := bufio.NewScanner(resp2.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var events []string
	for sc.Scan() && len(events) < 2 {
		if strings.HasPrefix(sc.Text(), "event: ") {
			events = append(events, sc.Text())
		}
	}
	if len(events) < 2 || events[0] != "event: history" || events[1] != "event: frame" {
		t.Fatalf("reconnect: %v", events)
	}
}

// Scenarios "Other organiser" and "No token": only the sale's organiser may open its stream.
func TestStreamOrganiserOnly(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	other, _ := s.RealSale(t, 100, 20)
	for name, tok := range map[string]string{
		"another organiser": s.OrganiserToken(t, other.Org),
		"no token":          "",
		"a fan":             s.FanToken(t, "192.0.2.9"),
		"a simulated fan":   s.SimToken(t, uuid.NewString(), uuid.NewString(), "86.100.0.1"),
		"a forged token":    "eyJhbGciOiJFZERTQSJ9.e30.AAAA",
	} {
		resp, cancel := open(t, s, sale.ID, tok)
		resp.Body.Close()
		cancel()
		if resp.StatusCode == http.StatusOK || resp.Header.Get("Content-Type") == "text/event-stream" {
			t.Fatalf("%s opened the stream (%d)", name, resp.StatusCode)
		}
	}
	resp, cancel := open(t, s, sale.ID, s.OrganiserToken(t, ev.Org))
	defer cancel()
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("own organiser: %d", resp.StatusCode)
	}
}

// Review D-g: a stream ends when its token does (the dashboard reconnects with a fresh one), and an
// old latest frame is not sent as if it were live.
func TestStreamEndsWithItsTokenAndSkipsOldFrames(t *testing.T) {
	s := simtest.Start(t)
	ev, sale := s.RealSale(t, 100, 20)
	ctx := context.Background()
	old, _ := json.Marshal(map[string]any{"ts": time.Now().Add(-time.Minute).UnixMilli(), "sale": sale.ID})
	if err := live.Publish(ctx, s.Redis, sale.ID, old, live.Point{TS: time.Now().Add(-time.Minute).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	resp, cancel := open(t, s, sale.ID, s.OrganiserTokenFor(t, ev.Org, 3*time.Second))
	defer cancel()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body) // returns when the server ends the stream
	if el := time.Since(start); el > 10*time.Second {
		t.Fatalf("the stream outlived its token: %s", el)
	}
	if strings.Contains(string(body), "event: frame") {
		t.Fatalf("an old frame was sent as live: %q", body)
	}
	if !strings.Contains(string(body), "event: history") {
		t.Fatalf("history missing: %q", body)
	}
}
