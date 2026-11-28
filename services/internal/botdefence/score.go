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

// Signals and their points (D-6, with step 5's review D-e). Each has a cap: a signal adds its
// points until the cap. The address signals (places per address or /64, per /24 or /48) do not add
// up: the larger counts, and alone they reach at most "risky" (a code at checkout), never
// "suspect"; with any other signal a full address signal makes a suspect.
type Signal struct {
	Name   string
	Points int
	Cap    int
}

// The signals.
var (
	NoProofOfWork   = Signal{"pow", 40, 40}
	FastPolling     = Signal{"fast", 10, 30} // per 10 polls under 2 s apart and before the time the page was told
	RegularPolling  = Signal{"regular", 20, 20}
	ManyPerIP       = Signal{"ip", 15, 15}     // more than 3 places from one address (or /64)
	BusyIP          = Signal{"ip", 40, 40}     // more than 20 places from one address: risky alone
	ManyPerSubnet   = Signal{"subnet", 25, 25} // more than 50 places from one /24 (or /48)
	DataCentre      = Signal{"dc", 20, 20}
	Headless        = Signal{"headless", 25, 25}
	SharedPrint     = Signal{"print", 15, 15} // one browser print shared by more than 20 people
	NewIdentity     = Signal{"new", 10, 10}
	BadPass         = Signal{"pass", 30, 60} // another account's pass presented from another session
	OTPExhausted    = Signal{"otp", 20, 20}
	allSignalFields = []string{"pow", "fast", "regular", "ip", "subnet", "dc", "headless", "print", "new", "pass", "otp"}
)

// Thresholds of the counting signals.
const (
	ManyPerIPAt     = 3
	BusyIPAt        = 20
	ManyPerSubnetAt = 50
	SharedPrintAt   = 20
)

var suspects = metrics.NewCounter("eps_bot_suspects_total", "Queue places whose score crossed the suspect threshold.")

// totalLua recomputes a place's score from its signal fields (b:<name>) in KEYS[1].
const totalLua = `
local function total()
  local f = redis.call('HGETALL', KEYS[1])
  local addr, other = 0, 0
  for i = 1, #f, 2 do
    local name = f[i]
    if string.sub(name, 1, 2) == 'b:' then
      local v = tonumber(f[i + 1]) or 0
      if name == 'b:ip' or name == 'b:subnet' then
        if v > addr then addr = v end
      else
        other = other + v
      end
    end
  end
  local score = math.min(100, addr + other)
  if addr >= 40 and other > 0 and score < 70 then score = 70 end
  redis.call('HSET', KEYS[1], 'score', score)
  return score
end
`

// addScript adds a signal's points (up to its cap) and returns the new score (negative when it
// just crossed the suspect threshold). KEYS: the place's hash. ARGV: field, points, cap, suspect.
var addScript = redis.NewScript(totalLua + `
local f = 'b:' .. ARGV[1]
local cur = tonumber(redis.call('HGET', KEYS[1], f) or '0')
local nxt = math.min(tonumber(ARGV[3]), cur + tonumber(ARGV[2]))
local before = tonumber(redis.call('HGET', KEYS[1], 'score') or '0')
if nxt ~= cur then redis.call('HSET', KEYS[1], f, nxt) end
local score = total()
if before < tonumber(ARGV[4]) and score >= tonumber(ARGV[4]) then return -score end
return score
`)

// setScript raises a signal to at least a value (the address signals grow by stages, not by sums).
var setScript = redis.NewScript(totalLua + `
local f = 'b:' .. ARGV[1]
local cur = tonumber(redis.call('HGET', KEYS[1], f) or '0')
local before = tonumber(redis.call('HGET', KEYS[1], 'score') or '0')
if tonumber(ARGV[2]) > cur then redis.call('HSET', KEYS[1], f, ARGV[2]) end
local score = total()
if before < tonumber(ARGV[3]) and score >= tonumber(ARGV[3]) then return -score end
return score
`)

// pollScript records one status poll of one tab at ARGV[1] (ms) and scores fast and regular
// polling (review D-e: per tab, and a poll at or after the time the server told the page is
// never "fast"). KEYS: the place's hash. ARGV: now ms, tab id.
var pollScript = redis.NewScript(totalLua + `
local now = tonumber(ARGV[1])
local tab = ARGV[2]
if redis.call('HEXISTS', KEYS[1], 'lp:' .. tab) == 0 then
  local tabs = tonumber(redis.call('HGET', KEYS[1], 'tabs') or '0')
  if tabs >= 4 then tab = 'x' else redis.call('HSET', KEYS[1], 'tabs', tabs + 1) end
end
local last = tonumber(redis.call('HGET', KEYS[1], 'lp:' .. tab) or '0')
local told = tonumber(redis.call('HGET', KEYS[1], 'np:' .. tab) or '0')
redis.call('HSET', KEYS[1], 'lp:' .. tab, now)
if last == 0 then return tonumber(redis.call('HGET', KEYS[1], 'score') or '0') end
local iv = now - last
local changed = false
if iv < 2000 and (told == 0 or now < told - 1000) then
  local fast = redis.call('HINCRBY', KEYS[1], 'fast:' .. tab, 1)
  if fast % 10 == 0 then
    local cur = tonumber(redis.call('HGET', KEYS[1], 'b:fast') or '0')
    if cur < 30 then redis.call('HSET', KEYS[1], 'b:fast', math.min(30, cur + 10)); changed = true end
  end
end
local list = {}
local s = redis.call('HGET', KEYS[1], 'iv:' .. tab)
if s then for x in string.gmatch(s, '[^,]+') do list[#list + 1] = tonumber(x) end end
list[#list + 1] = iv
while #list > 10 do table.remove(list, 1) end
redis.call('HSET', KEYS[1], 'iv:' .. tab, table.concat(list, ','))
if #list == 10 and not redis.call('HGET', KEYS[1], 'b:regular') then
  local sum = 0
  for _, x in ipairs(list) do sum = sum + x end
  local mean = sum / 10
  local var = 0
  for _, x in ipairs(list) do var = var + (x - mean) * (x - mean) end
  if math.sqrt(var / 10) < 100 then redis.call('HSET', KEYS[1], 'b:regular', 20); changed = true end
end
if changed then return total() end
return tonumber(redis.call('HGET', KEYS[1], 'score') or '0')
`)

// Scorer records signals into a place's hash (the waiting room names the key).
type Scorer struct {
	Redis *redis.Client
}

func (s *Scorer) counted(ctx context.Context, key string, n int, err error) (int, error) {
	if n < 0 {
		s.flagged(ctx, key)
		n = -n
	}
	return n, err
}

// flagged counts a place that crossed the suspect threshold, in total and on its sale's state
// hash (field "sus", step 6: the live dashboard's bots flagged per sale).
func (s *Scorer) flagged(ctx context.Context, key string) {
	suspects.Inc()
	if i := strings.Index(key, "}:qf:"); i > 0 {
		_ = s.Redis.HIncrBy(ctx, key[:i+2]+"qs", "sus", 1).Err()
	}
}

// Add records a signal and returns the score.
func (s *Scorer) Add(ctx context.Context, key string, sig Signal) (int, error) {
	n, err := addScript.Run(ctx, s.Redis, []string{key}, sig.Name, sig.Points, sig.Cap, SuspectAt).Int()
	return s.counted(ctx, key, n, err)
}

// Raise sets a signal to at least its points (the address signals).
func (s *Scorer) Raise(ctx context.Context, key string, sig Signal) (int, error) {
	n, err := setScript.Run(ctx, s.Redis, []string{key}, sig.Name, sig.Points, SuspectAt).Int()
	return s.counted(ctx, key, n, err)
}

// Poll records a status poll of a tab at nowMs and returns the score.
func (s *Scorer) Poll(ctx context.Context, key, tab string, nowMs int64) (int, error) {
	before, _ := s.Score(ctx, key)
	n, err := pollScript.Run(ctx, s.Redis, []string{key}, nowMs, tab).Int()
	if err == nil && before < SuspectAt && n >= SuspectAt {
		s.flagged(ctx, key)
	}
	return n, err
}

// Told records when the server told a tab to poll next (polls at or after it are never fast).
func (s *Scorer) Told(ctx context.Context, key, tab string, dueMs int64) {
	_ = s.Redis.HSet(ctx, key, "np:"+tab, dueMs).Err()
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
		fields[i] = "b:" + f
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
		// IPv6 (review D-e): AWS, Google Cloud, Azure, DigitalOcean, Hetzner, OVH.
		"2600:1f00::/24", "2600:1900::/28", "2603:1000::/24", "2604:a880::/32", "2a03:b0c0::/32", "2a01:4f8::/32", "2a01:4f9::/32", "2001:41d0::/32",
		"2001:db8:dc::/48", // documentation range, used by the tests as a hosting range
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

// Address is the unit one person's places are counted per: the address itself (IPv4) or its /64
// (IPv6, one household's prefix), "" when it is not an address.
func Address(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	if a.Is4() || a.Is4In6() {
		return a.Unmap().String()
	}
	p, err := a.Prefix(64)
	if err != nil {
		return ""
	}
	return strings.NewReplacer(":", "_", "/", "-").Replace(p.String())
}

// Subnet is the /24 (IPv4) or /48 (IPv6) of a client address, "" when it is not an address.
func Subnet(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	a = a.Unmap()
	bits := 24
	if a.Is6() {
		bits = 48
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return ""
	}
	return strings.NewReplacer(":", "_", "/", "-").Replace(p.String())
}
