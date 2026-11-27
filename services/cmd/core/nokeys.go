package main

import (
	"context"
	"crypto/ed25519"
	"errors"
)

type noKeys struct{}

func (noKeys) LoadKeys(context.Context) (map[string]ed25519.PublicKey, error) {
	return nil, errors.New("database not configured")
}
