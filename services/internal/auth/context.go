package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

type ctxKey int

const (
	claimsKey ctxKey = iota
	authErrKey
)

// FromContext returns the verified claims of the request, if any.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok && c != nil
}

// WithClaims stores claims in a context (used by tests and middleware).
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ErrFromContext returns why a presented token was rejected (nil when no token or a valid one).
func ErrFromContext(ctx context.Context) error {
	err, _ := ctx.Value(authErrKey).(error)
	return err
}

// ErrNoToken means the request carried no bearer token.
var ErrNoToken = errors.New("no bearer token")

// Middleware verifies a bearer token when one is present and stores the claims or the failure in
// the request context. Whether an operation requires a token is decided by the contract
// (the request validator), so public endpoints keep working with or without a header.
func Middleware(v *Verifier, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			ctx := r.Context()
			if h == "" {
				ctx = context.WithValue(ctx, authErrKey, ErrNoToken)
			} else if tok, ok := strings.CutPrefix(h, "Bearer "); !ok || tok == "" {
				ctx = context.WithValue(ctx, authErrKey, ErrInvalidToken)
			} else if c, err := v.Verify(ctx, strings.TrimSpace(tok)); err != nil {
				log.Info("token rejected", "reason", err.Error(), "path", r.URL.Path)
				ctx = context.WithValue(ctx, authErrKey, err)
			} else {
				ctx = WithClaims(ctx, c)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
