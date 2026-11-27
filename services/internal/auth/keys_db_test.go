package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// The core reads public keys from control.jwks under its own role: live keys and keys inside the
// 7-day grace period verify; keys past it do not.
func TestDBKeySourceHonoursGracePeriod(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	jwkOf := func() string {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		return `{"kty":"OKP","crv":"Ed25519","x":"` + base64.RawURLEncoding.EncodeToString(pub) + `"}`
	}
	now := time.Now()
	rows := []struct {
		id      string
		expires *time.Time
	}{
		{"live", ptr(now.Add(29 * 24 * time.Hour))},
		{"no-expiry", nil},
		{"in-grace", ptr(now.Add(-6 * 24 * time.Hour))},
		{"past-grace", ptr(now.Add(-8 * 24 * time.Hour))},
	}
	for _, r := range rows {
		_, err := pool.Exec(ctx, `insert into control."jwks" ("id", "publicKey", "privateKey", "createdAt", "expiresAt", "alg", "crv")
			values ($1, $2, 'encrypted-private-key', now(), $3, 'EdDSA', 'Ed25519')`, r.id, jwkOf(), r.expires)
		if err != nil {
			t.Fatal(err)
		}
	}
	keys, err := DBKeySource{Pool: pool}.LoadKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"live", "no-expiry", "in-grace"} {
		if _, ok := keys[id]; !ok {
			t.Errorf("key %s missing", id)
		}
	}
	if _, ok := keys["past-grace"]; ok {
		t.Error("key past its grace period still loaded")
	}
}

func ptr[T any](v T) *T { return &v }
