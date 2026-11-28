package botdefence

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// NormaliseEmail is the cap identity of a guest email (D-7): lower case, trimmed, "+tag" removed,
// and for Gmail the dots of the local part removed too (googlemail.com is gmail.com).
func NormaliseEmail(e string) string {
	e = strings.ToLower(strings.TrimSpace(e))
	local, domain, ok := strings.Cut(e, "@")
	if !ok {
		return e
	}
	if i := strings.IndexByte(local, '+'); i >= 0 {
		local = local[:i]
	}
	if domain == "googlemail.com" {
		domain = "gmail.com"
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}

// Identity is the person a cap counts: an account id, or a guest's normalised email (a guest
// without an email yet counts under its guest id until it gives one).
func Identity(kind, id, email string) string {
	if kind == "account" {
		return "acct:" + id
	}
	if email != "" {
		return "mail:" + NormaliseEmail(email)
	}
	return "guest:" + strings.TrimPrefix(id, "guest:")
}

// IdentityHash is what is stored for a cap identity (never the email itself): a keyed hash, so a
// stored value cannot be matched against guessed emails without the environment's key (review
// sec L4).
func IdentityHash(secret []byte, org, identity string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("cap|" + org + "|" + identity))
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// Identities are every identity a buyer's tickets count under (review D-g): an account, and its
// verified email when it has one (so a guest order with that email shares the cap); a guest, under
// its guest id until it gives an email, then under the email.
func Identities(kind, id, email string) []string {
	if kind == "account" {
		out := []string{"acct:" + id}
		if email != "" {
			out = append(out, "mail:"+NormaliseEmail(email))
		}
		return out
	}
	return []string{Identity(kind, id, email)}
}

// capScript adds an order's tickets under a person's cap unless that goes above it. Entries of
// orders whose hold lapsed stop counting by themselves; Postgres's paid tickets come in as the
// baseline. KEYS: the person's cap hash. ARGV: order id, tickets, cap, baseline, until ms, now ms.
// Returns {1, left after} or {0, left before}.
var capScript = redis.NewScript(`
local now = tonumber(ARGV[6])
local total = tonumber(ARGV[4])
local f = redis.call('HGETALL', KEYS[1])
for i = 1, #f, 2 do
  if f[i] ~= ARGV[1] then
    local q, u = string.match(f[i + 1], '^(%d+)|(%d+)$')
    if q and tonumber(u) > now then total = total + tonumber(q) else redis.call('HDEL', KEYS[1], f[i]) end
  end
end
local cap = tonumber(ARGV[3])
local qty = tonumber(ARGV[2])
if total + qty > cap then return {0, math.max(0, cap - total)} end
redis.call('HSET', KEYS[1], ARGV[1], ARGV[2] .. '|' .. ARGV[5])
local ttl = tonumber(ARGV[5]) - now + 3600000
if ttl > redis.call('PTTL', KEYS[1]) then redis.call('PEXPIRE', KEYS[1], ttl) end
return {1, cap - total - qty}
`)

// Caps counts tickets per person per event across sale moments (D-7).
type Caps struct {
	Redis *redis.Client
	Now   func() time.Time
}

func (c *Caps) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func capKey(org, event, identityHash string) (string, error) {
	return redisx.Key(org, event, "cap", identityHash)
}

// Hold adds an order's tickets under the person's cap (atomic: two sessions of one person at the
// cap cannot both pass). baseline is the person's paid tickets in Postgres. It returns whether the
// order fits and how many the person can still buy.
func (c *Caps) Hold(ctx context.Context, org, event, identityHash, orderID string, tickets, cap, baseline int, until time.Time) (bool, int, error) {
	k, err := capKey(org, event, identityHash)
	if err != nil {
		return false, 0, err
	}
	v, err := capScript.Run(ctx, c.Redis, []string{k}, orderID, tickets, cap, baseline, until.UnixMilli(), c.now().UnixMilli()).Int64Slice()
	if err != nil {
		return false, 0, err
	}
	return v[0] == 1, int(v[1]), nil
}

// Drop removes an order from the person's cap (cancelled, expired, or paid and now in Postgres).
func (c *Caps) Drop(ctx context.Context, org, event, identityHash, orderID string) error {
	k, err := capKey(org, event, identityHash)
	if err != nil {
		return err
	}
	return c.Redis.HDel(ctx, k, orderID).Err()
}
