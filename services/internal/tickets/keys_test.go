package tickets_test

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// Scenario "Public key set is published": ids, public keys and validity, no private material; a
// retired key stays published until its validity ends.
func TestPublicKeySet(t *testing.T) {
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	oldPub := hex.EncodeToString(vecKey().Public().(ed25519.PublicKey))
	r, err := tickets.KeyRingFromEnv(envOf(map[string]string{"EPS_ENV": "prod", "TICKET_KEY_ID": "2",
		"TICKET_SIGNING_KEY":  "4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb",
		"TICKET_RETIRED_KEYS": "1:" + oldPub + ":" + future + ",3:" + oldPub + ":" + past}))
	if err != nil {
		t.Fatal(err)
	}
	pub := r.Public()
	if len(pub) != 2 || pub[0].KeyID != 2 || pub[1].KeyID != 1 || pub[1].ValidTo == nil || pub[0].ValidTo != nil {
		t.Fatalf("published %+v", pub)
	}
	for _, k := range pub {
		if len(k.PublicKey) != 43 || strings.Contains(k.PublicKey, "4ccd089b") {
			t.Fatalf("key %d is not a bare 32-byte public key", k.KeyID)
		}
	}
	c, err := r.Sign("1b4e28ba-2fa1-4d2a-883f-0016d3cca427", "6f1c1a3e-9b7a-4c55-b1b0-3d1f2e4a5b6c", 1, 9)
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := tickets.Verify(c, r.Keys()); err != nil || tok.KeyID != 2 {
		t.Fatalf("signed with key %d: %v", tok.KeyID, err)
	}
}

// Scenario "Missing key stops the service": outside dev the key is required (the core stops); a
// service without one is not ready and signs nothing.
func TestMissingKeyFailsClosed(t *testing.T) {
	for _, env := range []string{"prod", "pr-7-abc123", ""} {
		if _, err := tickets.KeyRingFromEnv(envOf(map[string]string{"EPS_ENV": env})); err != tickets.ErrNoKey {
			t.Fatalf("EPS_ENV=%q without a key: %v, want ErrNoKey", env, err)
		}
	}
	if _, err := tickets.KeyRingFromEnv(envOf(map[string]string{"EPS_ENV": "prod", "TICKET_SIGNING_KEY": "abcd"})); err == nil {
		t.Fatal("a short key was accepted")
	}
	var none *tickets.KeyRing
	if c := none.Check(context.Background()); c.Ok || c.Name != "ticket-key" {
		t.Fatalf("readiness without a key: %+v", c)
	}
	if _, err := none.Sign("1b4e28ba-2fa1-4d2a-883f-0016d3cca427", "6f1c1a3e-9b7a-4c55-b1b0-3d1f2e4a5b6c", 1, 1); err != tickets.ErrNoKey {
		t.Fatalf("signing without a key: %v", err)
	}
	dev, err := tickets.KeyRingFromEnv(envOf(map[string]string{"EPS_ENV": "dev"}))
	if err != nil || !dev.Ephemeral() || !dev.Check(context.Background()).Ok {
		t.Fatalf("dev: %v", err)
	}
}
