package queue

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
)

// HTTP serves the waiting room's two operations on the gateway (contract tag "queue").
type HTTP struct {
	Engine   *Engine
	Verifier *auth.Verifier
	Log      *slog.Logger
}

// Routes registers the operations.
func (h *HTTP) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/queue/{saleId}/join", h.join)
	mux.HandleFunc("GET /v1/queue/{saleId}/status", h.status)
}

func (h *HTTP) claims(r *http.Request) (*auth.Claims, bool) {
	a := r.Header.Get("Authorization")
	if !strings.HasPrefix(a, "Bearer ") || h.Verifier == nil {
		return nil, false
	}
	c, err := h.Verifier.Verify(r.Context(), strings.TrimPrefix(a, "Bearer "))
	return c, err == nil && c.Subject != ""
}

func (h *HTTP) fail(w http.ResponseWriter, r *http.Request, err error) {
	var p *apperr.Problem
	switch {
	case errors.As(err, &p):
		httpx.WriteProblem(w, r, p.Status, p.Code, p.Detail)
	case errors.Is(err, ErrRateLimited):
		w.Header().Set("Retry-After", "30")
		httpx.WriteProblem(w, r, http.StatusTooManyRequests, apperr.RateLimited, "Too many joins from this connection. Wait a moment.")
	case errors.Is(err, ErrBadToken):
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "EPS-QUEUE-REJOIN", "Join the waiting room again.")
	default:
		if h.Log != nil {
			h.Log.Error("queue operation failed", "error", err.Error(), "requestId", httpx.RequestID(r.Context()))
		}
		httpx.WriteProblem(w, r, http.StatusServiceUnavailable, apperr.Unavailable, "")
	}
}

func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

func (h *HTTP) join(w http.ResponseWriter, r *http.Request) {
	c, ok := h.claims(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthenticated", "")
		return
	}
	var body struct {
		Pow   string `json:"pow"`
		Print string `json:"print"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid_request", "")
		return
	}
	kind := "account"
	if c.Kind == auth.KindGuest {
		kind = "guest"
	}
	// A simulator token (step 6) is only good on its own rehearsal (the engine checks), so its
	// synthetic address only ever counts against that rehearsal's per-sale limits.
	ip, sim := c.ClientIP, ""
	if c.Sim {
		sim = c.Rehearsal
	}
	ua := r.Header.Get("X-EPS-Client-UA")
	if len(ua) > 400 {
		ua = ua[:400]
	}
	st, err := h.Engine.Join(r.Context(), r.PathValue("saleId"), JoinInput{Sub: c.Subject, Kind: kind, Session: r.Header.Get("X-EPS-Session"),
		Pow: body.Pow, Print: body.Print, UA: ua, IP: ip, Tab: r.Header.Get("X-EPS-Tab"), SimRehearsal: sim})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	noStore(w)
	httpx.WriteJSON(w, http.StatusOK, st)
}

// status needs only the queue token and the session secret it is bound to (review D-i): no core
// token, so the web app forwards polls without reading a session.
func (h *HTTP) status(w http.ResponseWriter, r *http.Request) {
	st, err := h.Engine.StatusOf(r.Context(), r.PathValue("saleId"), r.Header.Get("X-EPS-Queue-Token"), r.Header.Get("X-EPS-Session"), r.Header.Get("X-EPS-Tab"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	noStore(w)
	httpx.WriteJSON(w, http.StatusOK, st)
}
