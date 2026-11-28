package botdefence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// OTP limits (D-8).
const (
	CodeLife   = 5 * time.Minute
	MaxTries   = 3
	MaxResends = 3
)

// OTP outcomes.
const (
	OTPVerified = "verified"
	OTPWrong    = "wrong"
	OTPExpired  = "expired"
	OTPLocked   = "locked" // every try of every code used: the order is cancelled
	OTPNone     = "none"   // no code was sent
)

var otpOutcomes = map[string]*metrics.Counter{}

func init() {
	for _, o := range []string{"sent", OTPVerified, OTPWrong, OTPExpired, OTPLocked} {
		otpOutcomes[o] = metrics.NewCounter("eps_otp_total", "One-time codes sent and checked, by outcome.", "outcome", o)
	}
}

// sendScript stores a new code unless every resend is used.
// KEYS: challenge. ARGV: code hash, expires ms, max sends. Returns sends so far, or -1.
var sendScript = redis.NewScript(`
local sends = tonumber(redis.call('HGET', KEYS[1], 'sends') or '0')
if redis.call('HGET', KEYS[1], 'ok') == '1' then return sends end
if sends >= tonumber(ARGV[3]) then return -1 end
redis.call('HSET', KEYS[1], 'h', ARGV[1], 'exp', ARGV[2], 'tries', 0, 'sends', sends + 1)
redis.call('EXPIRE', KEYS[1], 1800)
return sends + 1
`)

// verifyScript checks a code. KEYS: challenge. ARGV: code hash, now ms, max tries, max sends.
// Returns {outcome, tries left, resends left, counted (1 when this call used a try or changed
// the challenge)}.
var verifyScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return {'none', 0, 0, 0} end
local sends = tonumber(redis.call('HGET', KEYS[1], 'sends') or '0')
local resends = math.max(0, tonumber(ARGV[4]) - sends)
if redis.call('HGET', KEYS[1], 'ok') == '1' then return {'verified', 0, resends, 0} end
local tries = tonumber(redis.call('HGET', KEYS[1], 'tries') or '0')
if tries >= tonumber(ARGV[3]) then
  if resends == 0 then return {'locked', 0, 0, 0} end
  return {'wrong', 0, resends, 0}
end
if tonumber(ARGV[2]) > tonumber(redis.call('HGET', KEYS[1], 'exp')) then return {'expired', tonumber(ARGV[3]) - tries, resends, 0} end
if redis.call('HGET', KEYS[1], 'h') == ARGV[1] then
  redis.call('HSET', KEYS[1], 'ok', '1')
  return {'verified', 0, resends, 1}
end
tries = tries + 1
redis.call('HSET', KEYS[1], 'tries', tries)
if tries >= tonumber(ARGV[3]) and resends == 0 then return {'locked', 0, 0, 1} end
return {'wrong', tonumber(ARGV[3]) - tries, resends, 1}
`)

// OTP sends and checks the simulated one-time codes (shown on screen, never sent anywhere).
type OTP struct {
	Redis *redis.Client
	Now   func() time.Time
}

func (o *OTP) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func otpKey(org, event, orderID string) (string, error) {
	return redisx.Key(org, event, "otp", orderID)
}

func codeHash(orderID, code string) string {
	h := sha256.Sum256([]byte(orderID + "|" + code))
	return hex.EncodeToString(h[:])
}

// Challenge is a sent code (the code itself only because it is simulated: it goes to the screen).
type Challenge struct {
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expiresAt"`
	ResendsLeft int       `json:"resendsLeft"`
	TriesLeft   int       `json:"triesLeft"`
}

// ErrNoResends means every resend is used.
var ErrNoResends = fmt.Errorf("botdefence: no resends left")

// Send makes a new 6-digit code for an order (the first send or a resend).
func (o *OTP) Send(ctx context.Context, org, event, orderID string) (*Challenge, error) {
	k, err := otpKey(org, event, orderID)
	if err != nil {
		return nil, err
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return nil, err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	exp := o.now().Add(CodeLife)
	sends, err := sendScript.Run(ctx, o.Redis, []string{k}, codeHash(orderID, code), exp.UnixMilli(), 1+MaxResends).Int()
	if err != nil {
		return nil, err
	}
	if sends < 0 {
		return nil, ErrNoResends
	}
	otpOutcomes["sent"].Inc()
	return &Challenge{Code: code, ExpiresAt: exp, ResendsLeft: 1 + MaxResends - sends, TriesLeft: MaxTries}, nil
}

// Result of a check.
type Result struct {
	Outcome     string `json:"outcome"`
	TriesLeft   int    `json:"triesLeft"`
	ResendsLeft int    `json:"resendsLeft"`
	// Counted is false when the call changed nothing (no code sent, every try already used, or
	// already verified): nothing is recorded for it (review sec M3).
	Counted bool `json:"-"`
}

// Verify checks a code: verified, wrong (tries left), expired (resend offered) or locked (every
// try of every code used).
func (o *OTP) Verify(ctx context.Context, org, event, orderID, code string) (Result, error) {
	k, err := otpKey(org, event, orderID)
	if err != nil {
		return Result{}, err
	}
	v, err := verifyScript.Run(ctx, o.Redis, []string{k}, codeHash(orderID, code), o.now().UnixMilli(), MaxTries, 1+MaxResends).Slice()
	if err != nil {
		return Result{}, err
	}
	r := Result{Outcome: fmt.Sprint(v[0])}
	r.TriesLeft = int(v[1].(int64))
	r.ResendsLeft = int(v[2].(int64))
	r.Counted = v[3].(int64) == 1
	if m := otpOutcomes[r.Outcome]; m != nil && r.Counted {
		m.Inc()
	}
	return r, nil
}

// Verified reports whether the order's code was verified.
func (o *OTP) Verified(ctx context.Context, org, event, orderID string) bool {
	k, err := otpKey(org, event, orderID)
	if err != nil {
		return false
	}
	v, _ := o.Redis.HGet(ctx, k, "ok").Result()
	return v == "1"
}
