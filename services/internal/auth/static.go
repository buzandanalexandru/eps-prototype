package auth

import (
	"context"
	"crypto/ed25519"
)

// StaticKeys is a fixed key set (a binary without a database, and tests).
type StaticKeys map[string]ed25519.PublicKey

// LoadKeys returns the keys.
func (k StaticKeys) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) { return k, nil }
