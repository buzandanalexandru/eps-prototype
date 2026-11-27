package httpx

import (
	"net/http"
	"strings"
)

// CORS allows browser calls only from exact origins: the production web app and, on a preview,
// the preview's own Vercel URLs, which the pipeline injects (design D0.5). There are no wildcards:
// any Vercel account can name a project so that its URL matches a pattern such as
// `https://eps-prototype-*-devblocks.vercel.app` (a project called "eps-prototype-evil-devblocks"
// gets exactly that host), so an entry containing "*" is ignored.
type CORS struct {
	Exact map[string]bool
}

// NewCORS parses a comma-separated list of exact origins.
func NewCORS(list string) *CORS {
	c := &CORS{Exact: map[string]bool{}}
	for _, o := range strings.Split(list, ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "" || strings.Contains(o, "*") {
			continue
		}
		c.Exact[o] = true
	}
	return c
}

// Allowed reports whether the origin may call the service from a browser.
func (c *CORS) Allowed(origin string) bool {
	return origin != "" && c.Exact[origin]
}

// Middleware answers preflights and sets CORS headers for allowed origins only.
func (c *CORS) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		allowed := c.Allowed(origin)
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "false")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			if !allowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-Id")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
