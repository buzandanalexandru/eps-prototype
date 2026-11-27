package tenancy

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
)

type ctxKey int

const organiserKey ctxKey = iota

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// OrganiserFromContext returns the organiser scope set by RequireOrganiser.
func OrganiserFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(organiserKey).(string)
	return id, ok && id != ""
}

// RequireOrganiser guards organiser APIs: the organiser comes only from the verified token, never
// from the path or body. No token → 401; a fan or guest token → 403 with no scope set.
func RequireOrganiser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := auth.FromContext(r.Context())
		if !ok {
			httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthenticated", "")
			return
		}
		if c.Kind != auth.KindOrganiser || !uuidRe.MatchString(c.Org) {
			httpx.WriteProblem(w, r, http.StatusForbidden, "organiser_required", "This endpoint is for organisers.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), organiserKey, c.Org)))
	})
}

// ErrNotFound is the one answer for a record that does not exist and for a record of another
// organiser (row-level security makes them indistinguishable, and so must the API).
var ErrNotFound = errors.New("not found")

// WriteNotFound writes the identical 404 for both cases.
func WriteNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, http.StatusNotFound, "not_found", "No such record.")
}

// ValidID reports whether s is a canonical lowercase UUID (ids in organiser APIs).
func ValidID(s string) bool { return uuidRe.MatchString(s) }

// Handle maps a handler error: ErrNotFound → 404 (same body for missing and foreign records).
func Handle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		WriteNotFound(w, r)
		return true
	}
	httpx.WriteProblem(w, r, http.StatusInternalServerError, "internal", "")
	return true
}
