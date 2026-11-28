package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// Stream limits (design D-7).
const (
	MaxStreams        = 200
	MaxStreamsPerOrg  = 10
	Heartbeat         = 15 * time.Second
	watchFor          = 30 * time.Second
	clientBuffer      = 8
	historyOnConnect  = HistoryLen
	retryMilliseconds = 2000
	// WriteTimeout bounds each write to a client; MaxFrameAge is how old a frame may be to be sent.
	WriteTimeout = 10 * time.Second
	MaxFrameAge  = 5 * time.Second
)

func fresh(b []byte) bool {
	var head struct {
		TS int64 `json:"ts"`
	}
	return json.Unmarshal(b, &head) == nil && time.Since(time.UnixMilli(head.TS)) <= MaxFrameAge
}

var (
	streamsOpen = metrics.NewCounter("eps_live_streams_opened_total", "Live dashboard streams opened.")
	framesSent  = metrics.NewCounter("eps_live_frames_sent_total", "Live frames sent to dashboard streams.")
)

// Hub serves GET /v1/live/{saleId} on the gateway: one Redis subscription per gateway, fanned out
// to the streams in process.
type Hub struct {
	Redis    *redis.Client
	Sales    *sales.Service
	Verifier *auth.Verifier
	Log      *slog.Logger

	once   sync.Once
	mu     sync.Mutex
	ps     *redis.PubSub
	subs   map[string]map[chan []byte]struct{}
	perOrg map[string]int
	total  int
}

// Routes mounts the stream.
func (h *Hub) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/live/{saleId}", h.serve)
	metrics.NewGauge("eps_live_streams", func() float64 {
		h.mu.Lock()
		defer h.mu.Unlock()
		return float64(h.total)
	})
}

func (h *Hub) start() {
	h.subs = map[string]map[chan []byte]struct{}{}
	h.perOrg = map[string]int{}
	h.ps = h.Redis.Subscribe(context.Background())
	go func() {
		for m := range h.ps.Channel() {
			sale := strings.TrimPrefix(m.Channel, "eps:live:")
			h.mu.Lock()
			for c := range h.subs[sale] {
				select {
				case c <- []byte(m.Payload):
				default: // a slow client misses a frame; the next one carries every total
				}
			}
			h.mu.Unlock()
		}
	}()
}

func (h *Hub) join(ctx context.Context, sale, org string) (chan []byte, bool) {
	h.once.Do(h.start)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.total >= MaxStreams || h.perOrg[org] >= MaxStreamsPerOrg {
		return nil, false
	}
	c := make(chan []byte, clientBuffer)
	if h.subs[sale] == nil {
		h.subs[sale] = map[chan []byte]struct{}{}
		_ = h.ps.Subscribe(ctx, Channel(sale))
	}
	h.subs[sale][c] = struct{}{}
	h.perOrg[org]++
	h.total++
	return c, true
}

func (h *Hub) leave(sale, org string, c chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[sale], c)
	if len(h.subs[sale]) == 0 {
		delete(h.subs, sale)
		_ = h.ps.Unsubscribe(context.Background(), Channel(sale))
	}
	h.perOrg[org]--
	h.total--
}

// organiser checks the bearer core token: an organiser token of the sale's organiser.
func (h *Hub) organiser(r *http.Request) (*auth.Claims, bool) {
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, "Bearer ") || h.Verifier == nil {
		return nil, false
	}
	c, err := h.Verifier.Verify(r.Context(), strings.TrimPrefix(a, "Bearer "))
	if err != nil || c.Kind != auth.KindOrganiser || c.Org == "" || c.Sim {
		return nil, false
	}
	return c, true
}

func (h *Hub) serve(w http.ResponseWriter, r *http.Request) {
	c, ok := h.organiser(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthenticated", "An organiser core token is required.")
		return
	}
	sale, err := h.Sales.Lookup(r.Context(), r.PathValue("saleId"))
	if err != nil || sale.OrganiserID != c.Org {
		// Another organiser's sale reads as missing (no hint that it exists).
		httpx.WriteProblem(w, r, http.StatusNotFound, apperr.SaleNotFound, "Sale not found.")
		return
	}
	if !IsStreamRequest(r) {
		httpx.WriteProblem(w, r, http.StatusNotAcceptable, "not_acceptable", "Ask with Accept: text/event-stream.")
		return
	}
	ch, ok := h.join(r.Context(), sale.ID, c.Org)
	if !ok {
		w.Header().Set("Retry-After", "10")
		httpx.WriteProblem(w, r, http.StatusTooManyRequests, apperr.RateLimited, "Too many dashboards open.")
		return
	}
	defer h.leave(sale.ID, c.Org, ch)
	streamsOpen.Inc()
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	ctx := r.Context()
	_ = Watch(ctx, h.Redis, sale.ID, watchFor)
	// Every write has a deadline of its own (review D-g): a stuck client is dropped, never held.
	write := func(s string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(WriteTimeout))
		if _, err := w.Write([]byte(s)); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("retry: " + strconv.Itoa(retryMilliseconds) + "\n\n") {
		return
	}
	if pts, err := History(ctx, h.Redis, sale.ID, historyOnConnect); err == nil && len(pts) > 0 {
		b, _ := json.Marshal(pts)
		if !write("event: history\ndata: " + string(b) + "\n\n") {
			return
		}
	}
	sendFrame := func(b []byte) bool {
		var head struct {
			TS int64 `json:"ts"`
		}
		_ = json.Unmarshal(b, &head)
		framesSent.Inc()
		return write("event: frame\nid: " + strconv.FormatInt(head.TS, 10) + "\ndata: " + string(b) + "\n\n")
	}
	// The latest frame only if it is recent: an old one would show old numbers as live (D-g).
	if b := Latest(ctx, h.Redis, sale.ID); b != nil && fresh(b) && !sendFrame(b) {
		return
	}
	// The stream ends when the token does; the dashboard reconnects with a fresh one (D-g).
	life := time.NewTimer(time.Until(c.ExpiresAt.Time))
	defer life.Stop()
	beat := time.NewTicker(Heartbeat)
	defer beat.Stop()
	rewatch := time.NewTicker(watchFor / 3)
	defer rewatch.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-life.C:
			return
		case b := <-ch:
			if !sendFrame(b) {
				return
			}
		case <-beat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		case <-rewatch.C:
			_ = Watch(ctx, h.Redis, sale.ID, watchFor)
		}
	}
}

// IsStreamRequest is true for a request that asks for server-sent events (exempt from the 30 s
// write deadline, step 0).
func IsStreamRequest(r *http.Request) bool { return httpx.IsEventStream(r) }
