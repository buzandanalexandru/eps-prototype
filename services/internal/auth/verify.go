// Package auth verifies the short-lived core tokens issued by the control plane (U1, design D0.4)
// without calling back to it: public keys come from the control schema's key table.
package auth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Audience is the audience reserved for the core.
const Audience = "eps-core"

// Kinds of subject.
const (
	KindOrganiser = "organiser"
	KindFan       = "fan"
	KindGuest     = "guest"
)

// Claims are the core token claims.
type Claims struct {
	jwt.RegisteredClaims
	Kind string `json:"kind"`
	Org  string `json:"org,omitempty"`
	Role string `json:"role,omitempty"`
	Sid  string `json:"sid"`
}

// ErrInvalidToken is returned for every rejected token; the reason is only in the wrapped error,
// which callers log but never return to clients.
var ErrInvalidToken = errors.New("invalid token")

// KeySource loads the currently published public keys by key id.
type KeySource interface {
	LoadKeys(ctx context.Context) (map[string]ed25519.PublicKey, error)
}

// Verifier checks signature, audience, expiry (with leeway) and kind. Keys are cached for CacheTTL;
// an unknown key id triggers at most one reload per ReloadEvery.
type Verifier struct {
	Source      KeySource
	Now         func() time.Time
	Leeway      time.Duration
	CacheTTL    time.Duration
	ReloadEvery time.Duration

	mu         sync.Mutex
	keys       map[string]ed25519.PublicKey
	loadedAt   time.Time
	lastReload time.Time
	loads      int
}

// NewVerifier returns a verifier with the design defaults (30 s leeway, 60 s cache, one reload a minute).
func NewVerifier(src KeySource) *Verifier {
	return &Verifier{Source: src, Now: time.Now, Leeway: 30 * time.Second, CacheTTL: time.Minute, ReloadEvery: time.Minute}
}

// Loads reports how many times keys were loaded (for tests and metrics).
func (v *Verifier) Loads() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.loads
}

func (v *Verifier) reload(ctx context.Context, now time.Time) error {
	keys, err := v.Source.LoadKeys(ctx)
	v.lastReload = now
	v.loads++
	if err != nil {
		return err
	}
	v.keys = keys
	v.loadedAt = now
	return nil
}

func (v *Verifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.Now()
	// Reloads, whatever triggers them, happen at most once per ReloadEvery, so a flood of made-up
	// key ids (or a database outage) cannot hammer the database.
	canReload := v.loads == 0 || now.Sub(v.lastReload) >= v.ReloadEvery
	if (v.keys == nil || now.Sub(v.loadedAt) >= v.CacheTTL) && canReload {
		if err := v.reload(ctx, now); err != nil && v.keys == nil {
			return nil, fmt.Errorf("load keys: %w", err)
		}
		canReload = false
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	// Unknown key id: maybe a fresh rotation, so reload once if allowed.
	if canReload {
		if err := v.reload(ctx, now); err != nil {
			return nil, fmt.Errorf("load keys: %w", err)
		}
		if k, ok := v.keys[kid]; ok {
			return k, nil
		}
	}
	return nil, fmt.Errorf("unknown key id %q", kid)
}

// Verify parses and checks a token.
func (v *Verifier) Verify(ctx context.Context, token string) (*Claims, error) {
	claims := &Claims{}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithAudience(Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.Leeway),
		jwt.WithTimeFunc(v.Now),
	)
	_, err := parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing key id")
		}
		return v.key(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if err := checkKind(claims); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if claims.ExpiresAt.Sub(v.Now()) > 6*time.Minute {
		return nil, fmt.Errorf("%w: lifetime longer than 5 minutes", ErrInvalidToken)
	}
	return claims, nil
}

func checkKind(c *Claims) error {
	if c.Subject == "" {
		return errors.New("missing subject")
	}
	if c.Sid == "" {
		return errors.New("missing sid")
	}
	isGuestSub := strings.HasPrefix(c.Subject, "guest:")
	switch c.Kind {
	case KindOrganiser:
		if c.Org == "" || c.Role == "" {
			return errors.New("organiser token without organiser or role")
		}
		if isGuestSub {
			return errors.New("guest subject with organiser kind")
		}
	case KindFan:
		if c.Org != "" {
			return errors.New("fan token with organiser")
		}
		if isGuestSub {
			return errors.New("guest subject with fan kind")
		}
	case KindGuest:
		if c.Org != "" {
			return errors.New("guest token with organiser")
		}
		if !isGuestSub || len(c.Subject) <= len("guest:") {
			return errors.New("guest kind without guest subject")
		}
	default:
		return fmt.Errorf("unknown kind %q", c.Kind)
	}
	return nil
}
