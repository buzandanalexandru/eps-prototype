package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
)

const allowList = "https://eps-prototype-devblocks.vercel.app,https://eps-prototype-*-devblocks.vercel.app"

func preflight(h http.Handler, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, "/v1/whoami", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Scenario "Browser origin not allowed", plus the origins that are.
func TestCORSAllowsOnlyPlatformOrigins(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := httpx.NewCORS(allowList).Middleware(ok)

	for _, origin := range []string{
		"https://eps-prototype-devblocks.vercel.app",
		"https://eps-prototype-git-step-0-foundation-devblocks.vercel.app",
		"https://eps-prototype-a1b2c3d4e-devblocks.vercel.app",
	} {
		rec := preflight(h, origin)
		if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("%s: preflight %d, allow-origin %q", origin, rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}
	for _, origin := range []string{
		"https://evil.example",
		"https://eps-prototype.vercel.app.evil.example",
		"https://eps-prototype-x.evil.example-devblocks.vercel.app", // a dot inside the wildcard
		"https://eps-prototype--devblocks.vercel.app.evil",
		"http://eps-prototype.vercel.app",
		"null",
	} {
		rec := preflight(h, origin)
		if rec.Code != http.StatusForbidden || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: preflight %d, allow-origin %q", origin, rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
		}
		// A simple GET still runs (CORS is a browser rule), but without allow headers the browser
		// will not hand the response to the page.
		req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
		req.Header.Set("Origin", origin)
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s: simple request got allow-origin", origin)
		}
	}
}
