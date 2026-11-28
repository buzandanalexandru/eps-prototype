package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

// Step 5: the gateway reads the core-token keys the core published to Redis, and refuses a copy
// signed with another environment's key.
func TestPublishedKeys(t *testing.T) {
	rdb, err := redisx.Open(testdb.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer rdb.Close()
	ctx := context.Background()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := PublishKeys(ctx, StaticKeys{"k1": pub}, rdb, []byte("env-key")); err != nil {
		t.Fatal(err)
	}
	got, err := RedisKeySource{Redis: rdb, MACKey: []byte("env-key")}.LoadKeys(ctx)
	if err != nil || !got["k1"].Equal(pub) {
		t.Fatalf("keys %v (%v)", got, err)
	}
	if _, err := (RedisKeySource{Redis: rdb, MACKey: []byte("other")}).LoadKeys(ctx); err == nil {
		t.Fatal("a copy under another key was trusted")
	}
}
