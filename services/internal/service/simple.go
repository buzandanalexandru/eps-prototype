package service

import (
	"log/slog"
	"net/http"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/version"
)

// SimpleHandler is the HTTP surface of binaries outside the contract in step 0 (gateway, worker,
// fake PSP, simulator): /healthz, /readyz, /v1/version, problem+json 404s, logs and optional CORS.
func SimpleHandler(name string, log *slog.Logger, checks []health.Checker, cors *httpx.CORS, extra func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	health.Handlers(mux, checks)
	mux.HandleFunc("GET /v1/version", func(w http.ResponseWriter, r *http.Request) {
		v := api.Version{Service: name, Commit: version.Commit}
		if version.BuiltAt != "" {
			b := version.BuiltAt
			v.BuiltAt = &b
		}
		httpx.WriteJSON(w, http.StatusOK, v)
	})
	if extra != nil {
		extra(mux)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteProblem(w, r, http.StatusNotFound, "not_found", "")
	})
	mws := []func(http.Handler) http.Handler{httpx.WithRequestID, httpx.WithRecover(log), httpx.WithAccessLog(log)}
	if cors != nil {
		mws = append(mws, cors.Middleware)
	}
	return httpx.Chain(mux, mws...)
}
