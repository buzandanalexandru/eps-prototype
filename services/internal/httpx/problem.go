// Package httpx holds the HTTP plumbing shared by every EPS binary: problem+json errors (RFC 9457),
// request ids, structured access logs, panic recovery and CORS.
package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
)

// ProblemContentType is the media type of every error body.
const ProblemContentType = "application/problem+json"

// Problem builds a problem document for the request.
func Problem(r *http.Request, status int, code, detail string) api.Problem {
	p := api.Problem{
		Type:      "https://eps.dev/problems/" + code,
		Title:     http.StatusText(status),
		Status:    status,
		Code:      code,
		RequestId: RequestID(r.Context()),
	}
	if detail != "" {
		p.Detail = &detail
	}
	return p
}

// WriteProblem writes a problem+json response.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	WriteProblemDoc(w, Problem(r, status, code, detail))
}

// WriteProblemDoc writes an already built problem document.
func WriteProblemDoc(w http.ResponseWriter, p api.Problem) {
	w.Header().Set("Content-Type", ProblemContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// CodeForStatus maps an HTTP status to the stable problem code used when nothing more specific applies.
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusServiceUnavailable:
		return "unavailable"
	default:
		if status >= 500 {
			return "internal"
		}
		return "error"
	}
}
