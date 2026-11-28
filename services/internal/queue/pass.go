// Package queue is the waiting room (O1, E2-E4; design D-2 to D-5): a pre-queue before opening, a
// uniform shuffle at opening, first come first served after it, admission at the organiser's rate
// within the shopping limit, and a signed pass for every admitted fan that checkout requires.
//
// All of a sale's queue keys share one hash tag {<org>:<event>:<sale>} (redisx.HotKey with the sale
// as the shard part), so join, shuffle and admission are single Lua scripts.
package queue

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// Pass and queue-token kinds.
const (
	KindPass  = "pass"
	KindQueue = "queue"
	// PassLife is how long a pass lasts from admission (O1).
	PassLife = 10 * time.Minute
	// QueueTokenLife bounds a queue token (a place is kept for a day).
	QueueTokenLife = 24 * time.Hour
)

// Claims are a pass's (or queue token's) payload.
type Claims struct {
	V    int    `json:"v"`
	Kind string `json:"kind"`
	PID  string `json:"pid"`
	Org  string `json:"org"`
	Evt  string `json:"evt"`
	Sale string `json:"sale"`
	Sub  string `json:"sub"`
	// Sid is the SHA-256 of the session secret the fan's browser sends in X-EPS-Session.
	Sid string `json:"sid"`
	QID string `json:"qid"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
	Kid string `json:"kid"`
}

// Errors of Verify.
var (
	ErrMalformed = errors.New("queue: malformed pass")
	ErrSignature = errors.New("queue: bad pass signature")
	ErrExpired   = errors.New("queue: pass expired")
)

// SessionHash is the sid of a session secret.
func SessionHash(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// KeyID names a public key (its first 8 bytes of SHA-256, hex).
func KeyID(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:8])
}

// Signer issues passes; only the gateway holds it.
type Signer struct {
	Key ed25519.PrivateKey
	Kid string
}

// NewSigner wraps a private key.
func NewSigner(k ed25519.PrivateKey) *Signer {
	return &Signer{Key: k, Kid: KeyID(k.Public().(ed25519.PublicKey))}
}

// Sign returns v1.<payload>.<signature> (base64url, Ed25519 over "v1.<payload>").
func (s *Signer) Sign(c Claims) string {
	c.V = 1
	c.Kid = s.Kid
	if c.PID == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		c.PID = hex.EncodeToString(b)
	}
	body, _ := json.Marshal(c)
	head := "v1." + base64.RawURLEncoding.EncodeToString(body)
	sig := ed25519.Sign(s.Key, []byte(head))
	return head + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// Verifier checks passes with the gateway's public keys (by kid, so keys can rotate).
type Verifier struct {
	Keys map[string]ed25519.PublicKey
}

// NewVerifier trusts the given public keys.
func NewVerifier(pubs ...ed25519.PublicKey) *Verifier {
	v := &Verifier{Keys: map[string]ed25519.PublicKey{}}
	for _, p := range pubs {
		v.Keys[KeyID(p)] = p
	}
	return v
}

// Verify checks the format, the signature and the kind; expiry is checked against now unless
// ignoreExpiry (a payment on an order already held under the pass).
func (v *Verifier) Verify(tok, kind string, now time.Time, ignoreExpiry bool) (*Claims, error) {
	if len(tok) > 2048 || !strings.HasPrefix(tok, "v1.") {
		return nil, ErrMalformed
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, ErrMalformed
	}
	body, err1 := base64.RawURLEncoding.DecodeString(parts[1])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(sig) != ed25519.SignatureSize {
		return nil, ErrMalformed
	}
	var c Claims
	if err := json.Unmarshal(body, &c); err != nil || c.V != 1 {
		return nil, ErrMalformed
	}
	key, ok := v.Keys[c.Kid]
	if !ok || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return nil, ErrSignature
	}
	if c.Kind != kind {
		return nil, ErrSignature
	}
	if !ignoreExpiry && now.Unix() >= c.Exp {
		return &c, ErrExpired
	}
	return &c, nil
}

// devSeed is the fixed development key (EPS_ENV=dev only), so a local gateway and core agree.
var devSeed = sha256.Sum256([]byte("eps-dev-queue-pass-key"))

// KeysFromEnv reads QUEUE_PASS_PRIVATE_KEY (the 32-byte seed, hex; the gateway) and
// QUEUE_PASS_PUBLIC_KEY (hex; the core). Outside EPS_ENV=dev a missing key is an error (fail
// closed); in dev a fixed development key is used.
func KeysFromEnv(needPrivate bool) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	dev := os.Getenv("EPS_ENV") == "dev"
	var priv ed25519.PrivateKey
	if s := strings.TrimSpace(os.Getenv("QUEUE_PASS_PRIVATE_KEY")); s != "" {
		b, err := hex.DecodeString(s)
		if err != nil || len(b) != ed25519.SeedSize {
			return nil, nil, errors.New("queue: QUEUE_PASS_PRIVATE_KEY must be 32 bytes of hex")
		}
		priv = ed25519.NewKeyFromSeed(b)
	}
	var pub ed25519.PublicKey
	if s := strings.TrimSpace(os.Getenv("QUEUE_PASS_PUBLIC_KEY")); s != "" {
		b, err := hex.DecodeString(s)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return nil, nil, errors.New("queue: QUEUE_PASS_PUBLIC_KEY must be 32 bytes of hex")
		}
		pub = b
	}
	if priv != nil && pub == nil {
		pub = priv.Public().(ed25519.PublicKey)
	}
	if (needPrivate && priv == nil) || pub == nil {
		if !dev {
			return nil, nil, errors.New("queue: the pass keys are required outside EPS_ENV=dev")
		}
		priv = ed25519.NewKeyFromSeed(devSeed[:])
		pub = priv.Public().(ed25519.PublicKey)
	}
	return priv, pub, nil
}
