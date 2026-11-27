package redisx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// Scenario "Key for event stock".
func TestOrganiserEventKeyPrefix(t *testing.T) {
	k, err := Key("org-a", "event-e", "stock", "pool-1", "shard-0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(k, "eps:{org-a}:event-e:") || k != "eps:{org-a}:event-e:stock:pool-1:shard-0" {
		t.Fatalf("key %q", k)
	}
	k, err = Key("org-a", "", "ratelimit")
	if err != nil || k != "eps:{org-a}:_:ratelimit" {
		t.Fatalf("organiser-wide key %q (%v)", k, err)
	}
	if k, _ := PlatformKey("ratelimit", "ip"); k != "eps:platform:ratelimit:ip" {
		t.Fatalf("platform key %q", k)
	}
}

// Scenario "Missing organiser".
func TestEmptyOrganiserIsRejected(t *testing.T) {
	if k, err := Key("", "event-e", "stock"); !errors.Is(err, ErrNoOrganiser) || k != "" {
		t.Fatalf("got %q, %v", k, err)
	}
	for _, bad := range [][3]string{{"a:b", "e", "k"}, {"a", "e}x", "k"}, {"a", "e", ""}, {"a", "e", "k k"}} {
		if _, err := Key(bad[0], bad[1], bad[2]); err == nil {
			t.Errorf("Key(%q) accepted", bad)
		}
	}
	if _, err := Key("a", "e", "k", ""); err == nil {
		t.Error("empty rest part accepted")
	}
	// An organiser id can never pose as the platform prefix's hash tag.
	if _, err := Key("{platform}", "e", "k"); err == nil {
		t.Error("brace in organiser id accepted")
	}
}

// The chart's Redis (redis:8) answers through the client used by every binary.
func TestRedisPing(t *testing.T) {
	url := testdb.RedisURL(t)
	c, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	k, _ := Key("org-a", "e1", "probe")
	if err := c.Set(ctx, k, "1", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
}
