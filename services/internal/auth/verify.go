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
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
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
	// Email is the account's email, present only when it is verified (fan and organiser tokens;
	// step 4's guest-order claim relies on it).
	Email string `json:"email,omitempty"`
}

// ErrInvalidToken is returned for every rejected token; the reason is only in the wrapped error,
// which callers log but never return to clients.
var ErrInvalidToken = errors.New("invalid token")

// KeySource loads the currently published public keys by key id.
type KeySource interface {
	LoadKeys(ctx context.Context) (map[string]ed25519.PublicKey, error)
}

// Roles an organiser token may carry (control.memberships.role).
// organiserRoles are the exact membership roles a core token may carry (step 1, design D1.6);
// "staff" is step 0's value, still valid.
var organiserRoles = map[string]bool{"owner": true, "admin": true, "box_office": true, "finance": true, "staff": true}

// maxKidLen bounds the key id taken from an attacker-supplied header before it reaches an error
// message or a log line.
const maxKidLen = 64

// Verifier checks signature, issuer, audience, expiry (with leeway), kind and role.
//
// Keys: lookups take a read lock on an in-memory set and never wait for the database. The set is
// refreshed every RefreshEvery in the background (Start), and lazily when a lookup finds it older
// than that. An unknown key id (perhaps a fresh rotation) triggers a reload at most once per
// UnknownKidEvery, a limiter of its own, so a flood of made-up ids neither hammers the database nor
// delays the periodic refresh. Concurrent loads collapse into one (singleflight), and no lock is
// held across the database call. A failed load never locks verification out: the first load is
// retried on the next request after FailedLoadBackoff.
type Verifier struct {
	Source            KeySource
	Issuers           map[string]bool // allowed iss values; empty refuses every token
	Now               func() time.Time
	Leeway            time.Duration
	RefreshEvery      time.Duration
	UnknownKidEvery   time.Duration
	FailedLoadBackoff time.Duration

	mu            sync.RWMutex
	keys          map[string]ed25519.PublicKey
	loadedAt      time.Time // last successful load
	failedAt      time.Time // last failed load
	lastUnknownAt time.Time // last reload caused by an unknown key id
	loads         atomic.Int64
	group         singleflight.Group
}

// NewVerifier returns a verifier with the design defaults: 30 s leeway, refresh every 60 s, at most
// one unknown-kid reload every 15 s, 1 s back-off after a failed load.
func NewVerifier(src KeySource, issuers ...string) *Verifier {
	iss := map[string]bool{}
	for _, i := range issuers {
		if i = strings.TrimSpace(i); i != "" {
			iss[i] = true
		}
	}
	return &Verifier{
		Source: src, Issuers: iss, Now: time.Now, Leeway: 30 * time.Second,
		RefreshEvery: time.Minute, UnknownKidEvery: 15 * time.Second, FailedLoadBackoff: time.Second,
	}
}

// Loads reports how many times keys were loaded (for tests and metrics).
func (v *Verifier) Loads() int { return int(v.loads.Load()) }

// Start refreshes the keys every RefreshEvery until ctx ends. Optional: without it, lookups refresh
// a stale set themselves.
func (v *Verifier) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(v.RefreshEvery)
		defer t.Stop()
		_ = v.Refresh(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = v.Refresh(ctx)
			}
		}
	}()
}

// Refresh loads the keys now (collapsed with any load already running).
func (v *Verifier) Refresh(ctx context.Context) error {
	_, err, _ := v.group.Do("load", func() (any, error) {
		keys, err := v.Source.LoadKeys(context.WithoutCancel(ctx))
		v.loads.Add(1)
		now := v.Now()
		v.mu.Lock()
		defer v.mu.Unlock()
		if err != nil {
			v.failedAt = now
			return nil, err
		}
		v.keys, v.loadedAt, v.failedAt = keys, now, time.Time{}
		return nil, nil
	})
	return err
}

func (v *Verifier) lookup(kid string) (k ed25519.PublicKey, loaded bool, stale bool, canRetryFailed bool) {
	now := v.Now()
	v.mu.RLock()
	defer v.mu.RUnlock()
	loaded = v.keys != nil
	k = v.keys[kid]
	stale = loaded && now.Sub(v.loadedAt) >= v.RefreshEvery
	canRetryFailed = v.failedAt.IsZero() || now.Sub(v.failedAt) >= v.FailedLoadBackoff
	return
}

// takeUnknownSlot reports whether an unknown key id may trigger a reload now, and claims the slot.
func (v *Verifier) takeUnknownSlot() bool {
	now := v.Now()
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.lastUnknownAt.IsZero() && now.Sub(v.lastUnknownAt) < v.UnknownKidEvery {
		return false
	}
	v.lastUnknownAt = now
	return true
}

func (v *Verifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	k, loaded, stale, canRetry := v.lookup(kid)
	if !loaded {
		// No key set yet (first request, or every load so far failed): load now, unless a load
		// failed less than FailedLoadBackoff ago.
		if !canRetry {
			return nil, errors.New("keys unavailable")
		}
		if err := v.Refresh(ctx); err != nil {
			return nil, fmt.Errorf("load keys: %w", err)
		}
		k, _, _, _ = v.lookup(kid)
	} else if stale && canRetry {
		// Refresh in the background; this request uses the current set.
		go func() { _ = v.Refresh(context.WithoutCancel(ctx)) }()
	}
	if k != nil {
		return k, nil
	}
	if v.takeUnknownSlot() {
		if err := v.Refresh(ctx); err != nil {
			return nil, fmt.Errorf("load keys: %w", err)
		}
		if k, _, _, _ = v.lookup(kid); k != nil {
			return k, nil
		}
	}
	return nil, fmt.Errorf("unknown key id %q", boundedKid(kid))
}

func boundedKid(kid string) string {
	if len(kid) > maxKidLen {
		kid = kid[:maxKidLen] + "..."
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '?'
		}
		return r
	}, kid)
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
	if !v.Issuers[claims.Issuer] {
		return nil, fmt.Errorf("%w: issuer not allowed in this environment", ErrInvalidToken)
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
		if !organiserRoles[c.Role] {
			return errors.New("organiser token with an unknown role")
		}
		if isGuestSub {
			return errors.New("guest subject with organiser kind")
		}
	case KindFan:
		if c.Org != "" || c.Role != "" {
			return errors.New("fan token with organiser")
		}
		if isGuestSub {
			return errors.New("guest subject with fan kind")
		}
	case KindGuest:
		if c.Org != "" || c.Role != "" {
			return errors.New("guest token with organiser")
		}
		if !isGuestSub || len(c.Subject) <= len("guest:") {
			return errors.New("guest kind without guest subject")
		}
		if c.Email != "" {
			return errors.New("guest token with an email")
		}
	default:
		return fmt.Errorf("unknown kind %q", c.Kind)
	}
	return nil
}

// IssuerForEnv is the issuer the web app of environment slug env puts in its core tokens
// (design D0.4): "urn:eps:web:<slug>", so a token from one environment never verifies in another.
func IssuerForEnv(env string) string {
	if env == "" {
		env = "dev"
	}
	return "urn:eps:web:" + env
}

// IssuersFromEnv reads the allowed issuers: CORE_TOKEN_ISSUERS (comma-separated) when set, else the
// issuer of EPS_ENV (default dev).
func IssuersFromEnv(getenv func(string) string) []string {
	if list := getenv("CORE_TOKEN_ISSUERS"); strings.TrimSpace(list) != "" {
		return strings.Split(list, ",")
	}
	return []string{IssuerForEnv(getenv("EPS_ENV"))}
}
