package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// GracePeriod is how long a rotated key keeps verifying after it stops signing (design D0.4: 7 days).
const GracePeriod = 7 * 24 * time.Hour

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
}

// ParseEd25519JWK reads an OKP/Ed25519 public JWK.
func ParseEd25519JWK(s string) (ed25519.PublicKey, error) {
	var k jwk
	if err := json.Unmarshal([]byte(s), &k); err != nil {
		return nil, err
	}
	if k.Kty != "OKP" || k.Crv != "Ed25519" {
		return nil, fmt.Errorf("unsupported key type %s/%s", k.Kty, k.Crv)
	}
	raw, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("bad Ed25519 key length")
	}
	return ed25519.PublicKey(raw), nil
}

// DBKeySource reads public keys from control.jwks through the core role's column grant.
type DBKeySource struct {
	Pool db.Beginner
}

func (s DBKeySource) LoadKeys(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	out := map[string]ed25519.PublicKey{}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select "id", "publicKey" from control."jwks"
			where "expiresAt" is null or "expiresAt" + make_interval(secs => $1) > now()`, GracePeriod.Seconds())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, pub string
			if err := rows.Scan(&id, &pub); err != nil {
				return err
			}
			k, err := ParseEd25519JWK(pub)
			if err != nil {
				continue // keys of other algorithms are not core-token keys
			}
			out[id] = k
		}
		return rows.Err()
	})
	return out, err
}
