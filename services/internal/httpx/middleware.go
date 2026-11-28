package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

type ctxKey int

const requestIDKey ctxKey = iota

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{8,64}$`)

// RequestID returns the request id stored by WithRequestID.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WithRequestID takes a well-formed X-Request-Id from the caller or makes one, and echoes it.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !validRequestID.MatchString(id) {
			id = newID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// WithAccessLog counts every request by status class and logs one JSON line per request at Debug
// (never headers or bodies, so no tokens). At 100k fans an Info line per request would cost more
// than the request; errors (5xx) are still logged at Warn.
func WithAccessLog(log *slog.Logger, service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			metrics.RequestCounter(service, rec.status).Inc()
			metrics.NewHistogram("eps_http_request_seconds", "Request latency by route (ids folded).", metrics.DefaultLatencyBuckets,
				"service", service, "route", Route(r.URL.Path)).Observe(time.Since(start).Seconds())
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				return // probes every few seconds would drown the log
			}
			level := slog.LevelDebug
			if rec.status >= 500 {
				level = slog.LevelWarn
			}
			log.Log(r.Context(), level, "request",
				"method", r.Method, "path", r.URL.Path, "status", rec.status,
				"durationMs", time.Since(start).Milliseconds(), "requestId", RequestID(r.Context()))
		})
	}
}

// WithRecover turns a panic into a 500 problem and logs it.
func WithRecover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					log.Error("panic", "panic", v, "path", r.URL.Path, "requestId", RequestID(r.Context()))
					WriteProblem(w, r, http.StatusInternalServerError, "internal", "")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies middlewares so that the first one listed is the outermost.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// WithWriteDeadline gives each request a write deadline of d, except requests for which stream
// reports true (server-sent events), which keep an open-ended response. The server itself has no
// global WriteTimeout, so a long-lived stream is never cut by it.
func WithWriteDeadline(d time.Duration, stream func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if stream == nil || !stream(r) {
				_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsEventStream reports whether the client asked for server-sent events.
func IsEventStream(r *http.Request) bool {
	return r.Header.Get("Accept") == "text/event-stream"
}

// Route folds a path into a bounded label: ids (uuids, long hex or digits) become ":id" and only
// the first five segments count (review sec M5: latency by route).
func Route(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 5 {
		parts = parts[:5]
	}
	for i, p := range parts {
		if isID(p) {
			parts[i] = ":id"
		} else if len(p) > 32 {
			parts[i] = ":x"
		}
	}
	return "/" + strings.Join(parts, "/")
}

func isID(p string) bool {
	if len(p) < 8 {
		return false
	}
	digits := 0
	for _, c := range p {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c >= 'a' && c <= 'f', c >= 'A' && c <= 'F', c == '-', c == '_':
		default:
			return false
		}
	}
	return digits > 0
}
