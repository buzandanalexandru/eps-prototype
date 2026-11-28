// Package botdefence keeps bots and bulk buyers from crowding out fans (E5, E6, E12; design D-6 to
// D-8): a silent 0-100 score per queue place from behaviour signals, caps per person across an
// event's sale moments, and a one-time code asked only from risky or bulk buyers.
//
// Scoring never blocks and never shows anything to the fan: the waiting room uses the score only
// after the shuffle, to move suspects to the back, and the checkout uses it to decide on a code.
package botdefence

import (
	"context"
	"net/netip"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// Thresholds (D-6).
const (
	RiskyAt    = 40 // 40-69: asked for a code at checkout when OTP is on
	SuspectAt  = 70 // moved behind every other place after the shuffle
	SlowLaneAt = 90 // admitted only when no one else waits
)

// Signals and their points (D-6). Each has a cap: a signal adds its points until the cap.
type Signal struct {
	Name   string
	Points int
	Cap    int
}

// The signals.
var (
	NoProofOfWork   = Signal{"pow", 40, 40}
	FastPolling     = Signal{"fast", 10, 30} // per 10 polls under 2 s apart
	RegularPolling  = Signal{"regular", 20, 20}
	ManyPerIP       = Signal{"ip", 15, 15} // more than 3 places from one client address
	ManyPerSubnet   = Signal{"subnet", 25, 25}
	DataCentre      = Signal{"dc", 20, 20}
	Headless        = Signal{"headless", 25, 25}
	SharedPrint     = Signal{"print", 15, 15}
	NewIdentity     = Signal{"new", 10, 10}
	BadPass         = Signal{"pass", 30, 60} // checkout without a pass, with a forged one or another session's
	OTPExhausted    = Signal{"otp", 20, 20}
	allSignalFields = []string{"pow", "fast", "regular", "ip", "subnet", "dc", "headless", "print", "new", "pass", "otp"}
)

var suspects = metrics.NewCounter("eps_bot_suspects_total", "Queue places whose score crossed the suspect threshold.")

// addScript adds a signal's points (up to its cap) and returns the new score.
// KEYS: score hash. ARGV: field, points, cap, suspect threshold.
var addScript = redis.NewScript(`
local f = 's:' .. ARGV[1]
local cur = tonumber(redis.call('HGET', KEYS[1], f) or '0')
local nxt = math.min(tonumber(ARGV[3]), cur + tonumber(ARGV[2]))
local before = tonumber(redis.call('HGET', KEYS[1], 'score') or '0')
if nxt ~= cur then redis.call('HSET', KEYS[1], f, nxt) end
local score = math.min(100, before + (nxt - cur))
redis.call('HSET', KEYS[1], 'score', score)
redis.call('EXPIRE', KEYS[1], 172800)
if before < tonumber(ARGV[4]) and score >= tonumber(ARGV[4]) then return -score end
return score
`)

// pollScript records one status poll at ARGV[1] (ms) and scores fast and regular polling.
// KEYS: score hash. ARGV: now ms, suspect threshold.
var pollScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local before = tonumber(redis.call('HGET', KEYS[1], 'score') or '0')
local last = tonumber(redis.call('HGET', KEYS[1], 'lp') or '0')
redis.call('HSET', KEYS[1], 'lp', now)
redis.call('EXPIRE', KEYS[1], 172800)
if last == 0 then return before end
local iv = now - last
local add = function(f, pts, cap)
  local cur = tonumber(redis.call('HGET', KEYS[1], 's:' .. f) or '0')
  local nxt = math.min(cap, cur + pts)
  if nxt ~= cur then
    redis.call('HSET', KEYS[1], 's:' .. f, nxt)
    before = math.min(100, before + nxt - cur)
  end
end
if iv < 2000 then
  local fast = redis.call('HINCRBY', KEYS[1], 'fast', 1)
  if fast % 10 == 0 then add('fast', 10, 30) end
end
local list = {}
local s = redis.call('HGET', KEYS[1], 'iv')
if s then for x in string.gmatch(s, '[^,]+') do list[#list + 1] = tonumber(x) end end
list[#list + 1] = iv
while #list > 10 do table.remove(list, 1) end
redis.call('HSET', KEYS[1], 'iv', table.concat(list, ','))
if #list == 10 and not redis.call('HGET', KEYS[1], 's:regular') then
  local sum = 0
  for _, x in ipairs(list) do sum = sum + x end
  local mean = sum / 10
  local var = 0
  for _, x in ipairs(list) do var = var + (x - mean) * (x - mean) end
  if math.sqrt(var / 10) < 100 then add('regular', 20, 20) end
end
redis.call('HSET', KEYS[1], 'score', before)
return before
`)

// Scorer records signals into a place's score hash (the waiting room names the key).
type Scorer struct {
	Redis *redis.Client
}

// Add records a signal and returns the score.
func (s *Scorer) Add(ctx context.Context, key string, sig Signal) (int, error) {
	n, err := addScript.Run(ctx, s.Redis, []string{key}, sig.Name, sig.Points, sig.Cap, SuspectAt).Int()
	if n < 0 {
		suspects.Inc()
		n = -n
	}
	return n, err
}

// Poll records a status poll at nowMs and returns the score.
func (s *Scorer) Poll(ctx context.Context, key string, nowMs int64) (int, error) {
	before, _ := s.Score(ctx, key)
	n, err := pollScript.Run(ctx, s.Redis, []string{key}, nowMs, SuspectAt).Int()
	if err == nil && before < SuspectAt && n >= SuspectAt {
		suspects.Inc()
	}
	return n, err
}

// Score reads a place's score (0 when none).
func (s *Scorer) Score(ctx context.Context, key string) (int, error) {
	n, err := s.Redis.HGet(ctx, key, "score").Int()
	if err == redis.Nil {
		return 0, nil
	}
	return n, err
}

// Signals reads which signals a place has (for the organiser's view and tests).
func (s *Scorer) Signals(ctx context.Context, key string) (map[string]int, error) {
	fields := make([]string, len(allSignalFields))
	for i, f := range allSignalFields {
		fields[i] = "s:" + f
	}
	v, err := s.Redis.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for i, x := range v {
		if sx, ok := x.(string); ok && sx != "0" {
			n := 0
			for _, c := range sx {
				n = n*10 + int(c-'0')
			}
			out[allSignalFields[i]] = n
		}
	}
	return out, nil
}

// IsHeadless reports automation markers in a user agent.
func IsHeadless(ua string) bool {
	u := strings.ToLower(ua)
	if strings.TrimSpace(u) == "" {
		return true
	}
	for _, m := range []string{"headless", "phantomjs", "selenium", "webdriver", "puppeteer", "playwright", "python-requests", "curl/", "wget/", "go-http-client", "scrapy", "httpclient", "node-fetch", "axios/"} {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// dataCentres is a small static list of hosting ranges (the prototype's stand-in for a feed).
var dataCentres = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"3.0.0.0/9", "13.32.0.0/12", "18.128.0.0/9", "34.64.0.0/10", "35.184.0.0/13", // AWS, Google Cloud
		"20.0.0.0/11", "40.64.0.0/10", "52.0.0.0/10", // Azure
		"104.16.0.0/13", "138.68.0.0/16", "139.59.0.0/16", "159.65.0.0/16", "167.99.0.0/16", "164.90.0.0/16", // Cloudflare, DigitalOcean
		"95.216.0.0/15", "135.181.0.0/16", "5.9.0.0/16", "88.198.0.0/16", // Hetzner
		"51.68.0.0/16", "51.75.0.0/16", "54.36.0.0/16", // OVH
		"198.51.100.0/24", // documentation range, used by the crowd test as a hosting range
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// InDataCentre reports a client address in a known hosting range.
func InDataCentre(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, p := range dataCentres {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Subnet is the /24 (IPv4) or /48 (IPv6) of a client address, "" when it is not an address.
func Subnet(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	bits := 24
	if a.Is6() && !a.Is4In6() {
		bits = 48
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return ""
	}
	return strings.NewReplacer(":", "_", "/", "-").Replace(p.String())
}
