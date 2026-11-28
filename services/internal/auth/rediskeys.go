package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// The core publishes the core-token public keys to Redis (signed with SNAPSHOT_CACHE_KEY), so the
// gateway verifies core tokens without a Postgres pool (step 5: the gateway stays database-free).

func keysKey() string {
	k, _ := redisx.PlatformKey("core-token-keys")
	return k
}

func mac(key, body []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write(body)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// PublishKeys copies the current public keys from src to Redis.
func PublishKeys(ctx context.Context, src KeySource, rdb *redis.Client, macKey []byte) error {
	keys, err := src.LoadKeys(ctx)
	if err != nil {
		return err
	}
	out := map[string]string{}
	for kid, k := range keys {
		out[kid] = base64.RawURLEncoding.EncodeToString(k)
	}
	body, _ := json.Marshal(out)
	return rdb.Set(ctx, keysKey(), mac(macKey, body)+"."+string(body), 24*time.Hour).Err()
}

// RunPublisher publishes the keys now and every interval.
func RunPublisher(ctx context.Context, src KeySource, rdb *redis.Client, macKey []byte, every time.Duration) {
	_ = PublishKeys(ctx, src, rdb, macKey)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = PublishKeys(ctx, src, rdb, macKey)
		}
	}
}

// RedisKeySource reads the keys the core published (refusing a copy with a bad signature).
type RedisKeySource struct {
	Redis  *redis.Client
	MACKey []byte
}

// LoadKeys implements KeySource.
func (s RedisKeySource) LoadKeys(ctx context.Context) (map[string]ed25519.PublicKey, error) {
	v, err := s.Redis.Get(ctx, keysKey()).Result()
	if err != nil {
		return nil, err
	}
	sig, body, ok := strings.Cut(v, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(mac(s.MACKey, []byte(body)))) {
		return nil, errors.New("auth: the published keys are not signed with this environment's key")
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, err
	}
	out := map[string]ed25519.PublicKey{}
	for kid, b := range m {
		raw, err := base64.RawURLEncoding.DecodeString(b)
		if err == nil && len(raw) == ed25519.PublicKeySize {
			out[kid] = raw
		}
	}
	return out, nil
}
