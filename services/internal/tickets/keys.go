package tickets

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
)

// PublicKey is one published ticket key (GET /v1/ticket-keys).
type PublicKey struct {
	KeyID     int        `json:"keyId"`
	PublicKey string     `json:"publicKey"` // raw 32 bytes, base64url without padding
	ValidFrom time.Time  `json:"validFrom"`
	ValidTo   *time.Time `json:"validTo"`
}

// KeyRing is the core's signing key and the public keys it publishes (design D3). Only the core
// holds the private half; the web app, the browser and the door never do.
type KeyRing struct {
	id      byte
	priv    ed25519.PrivateKey
	public  []PublicKey
	keys    map[byte]ed25519.PublicKey
	devOnly bool
}

// ErrNoKey means the core has no ticket signing key: it then refuses to be ready and issues no
// codes (fail closed).
var ErrNoKey = errors.New("tickets: no ticket signing key (TICKET_SIGNING_KEY)")

// ValidFrom is when an environment's first key starts (published for the door's information).
var keyEpoch = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// KeyRingFromEnv reads TICKET_SIGNING_KEY (the 32-byte Ed25519 seed, hex), TICKET_KEY_ID (1 to
// 255, default 1) and TICKET_RETIRED_KEYS (older public keys still published until the last event
// signed with them ended: "<id>:<hex public key>:<valid to, RFC 3339>" separated by commas).
// Outside EPS_ENV=dev a missing or malformed key is an error; in dev an ephemeral key is made.
func KeyRingFromEnv(getenv func(string) string) (*KeyRing, error) {
	seedHex := strings.TrimSpace(getenv("TICKET_SIGNING_KEY"))
	env := getenv("EPS_ENV")
	if seedHex == "" {
		if env != "dev" {
			return nil, ErrNoKey
		}
		seed := make([]byte, ed25519.SeedSize)
		_, _ = rand.Read(seed)
		seedHex = hex.EncodeToString(seed)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("tickets: TICKET_SIGNING_KEY must be a 32-byte hex seed")
	}
	id := 1
	if v := strings.TrimSpace(getenv("TICKET_KEY_ID")); v != "" {
		if id, err = strconv.Atoi(v); err != nil || id < 1 || id > 255 {
			return nil, fmt.Errorf("tickets: TICKET_KEY_ID must be 1 to 255")
		}
	}
	r := NewKeyRing(byte(id), ed25519.NewKeyFromSeed(seed))
	r.devOnly = getenv("TICKET_SIGNING_KEY") == ""
	for _, part := range strings.Split(getenv("TICKET_RETIRED_KEYS"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f := strings.SplitN(part, ":", 3)
		if len(f) != 3 {
			return nil, fmt.Errorf("tickets: TICKET_RETIRED_KEYS entry must be id:publickey:validTo")
		}
		kid, err1 := strconv.Atoi(f[0])
		pub, err2 := hex.DecodeString(f[1])
		to, err3 := time.Parse(time.RFC3339, f[2])
		if err1 != nil || err2 != nil || err3 != nil || kid < 1 || kid > 255 || byte(kid) == r.id || len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("tickets: bad TICKET_RETIRED_KEYS entry for key %q", f[0])
		}
		if to.Before(time.Now()) {
			continue // every event signed with it has ended: no longer published
		}
		r.keys[byte(kid)] = ed25519.PublicKey(pub)
		r.public = append(r.public, PublicKey{KeyID: kid, PublicKey: base64.RawURLEncoding.EncodeToString(pub), ValidFrom: keyEpoch, ValidTo: &to})
	}
	return r, nil
}

// NewKeyRing is a ring with one signing key.
func NewKeyRing(id byte, priv ed25519.PrivateKey) *KeyRing {
	pub := priv.Public().(ed25519.PublicKey)
	return &KeyRing{id: id, priv: priv, keys: map[byte]ed25519.PublicKey{id: pub},
		public: []PublicKey{{KeyID: int(id), PublicKey: base64.RawURLEncoding.EncodeToString(pub), ValidFrom: keyEpoch}}}
}

// Ephemeral reports whether the key was made at start (EPS_ENV=dev without a key).
func (r *KeyRing) Ephemeral() bool { return r != nil && r.devOnly }

// Public is the published key set: ids, public keys and validity, no private material.
func (r *KeyRing) Public() []PublicKey {
	if r == nil {
		return nil
	}
	return append([]PublicKey(nil), r.public...)
}

// Keys are the verification keys by id.
func (r *KeyRing) Keys() map[byte]ed25519.PublicKey { return r.keys }

// Sign signs a code for a ticket generation and window with the current key.
func (r *KeyRing) Sign(eventID, ticketID string, generation uint16, window uint32) (string, error) {
	if r == nil || r.priv == nil {
		return "", ErrNoKey
	}
	return Sign(Token{KeyID: r.id, EventID: eventID, TicketID: ticketID, Generation: generation, Window: window}, r.priv)
}

// Check is the readiness check "ticket-key": not ready without a signing key (design D3).
func (r *KeyRing) Check(ctx context.Context) api.Check {
	if r == nil || r.priv == nil {
		d := ErrNoKey.Error()
		return api.Check{Name: "ticket-key", Ok: false, Detail: &d}
	}
	return api.Check{Name: "ticket-key", Ok: true}
}
