package httpx

import (
	"net/http"
	"strings"
)

// CORS allows browser calls only from the platform's web origins: exact origins (production web app)
// and patterns with one `*` standing for a single DNS label fragment (Vercel previews, for example
// `https://eps-prototype-*-devblocks.vercel.app`).
type CORS struct {
	Exact    map[string]bool
	Patterns [][2]string // prefix, suffix
}

// NewCORS parses a comma-separated list of origins and patterns.
func NewCORS(list string) *CORS {
	c := &CORS{Exact: map[string]bool{}}
	for _, o := range strings.Split(list, ",") {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "" {
			continue
		}
		if i := strings.Index(o, "*"); i >= 0 {
			if strings.Count(o, "*") == 1 {
				c.Patterns = append(c.Patterns, [2]string{o[:i], o[i+1:]})
			}
			continue
		}
		c.Exact[o] = true
	}
	return c
}

// Allowed reports whether the origin may call the service from a browser.
func (c *CORS) Allowed(origin string) bool {
	if origin == "" {
		return false
	}
	if c.Exact[origin] {
		return true
	}
	for _, p := range c.Patterns {
		if len(origin) <= len(p[0])+len(p[1]) || !strings.HasPrefix(origin, p[0]) || !strings.HasSuffix(origin, p[1]) {
			continue
		}
		mid := origin[len(p[0]) : len(origin)-len(p[1])]
		if isLabelFragment(mid) {
			return true
		}
	}
	return false
}

func isLabelFragment(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
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
