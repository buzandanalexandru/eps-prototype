package simulation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Bot behaviours (design D-4).
const (
	BotSpammer  = "spammer"
	BotFarm     = "farm"
	BotHeadless = "headless"
	BotSkipper  = "skipper"
	BotCapper   = "capper"
)

// BotKinds are spread evenly over the bot share.
var BotKinds = []string{BotSpammer, BotFarm, BotHeadless, BotSkipper, BotCapper}

const (
	browserUA  = "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Mobile Safari/537.36"
	headlessUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/131.0 Safari/537.36"
)

// client is one simulated fan or bot.
type client struct {
	i       int
	bot     string // "" for a real fan
	ip, ua  string
	print   string
	session string
	sid     string
	tab     string
	sub     string
	arrive  time.Time
	qid     string
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// FanAddress is a real fan's synthetic address: one per fan, spread like home connections.
func FanAddress(i int) string {
	return fmt.Sprintf("86.%d.%d.%d", 100+(i>>16)%100, (i>>8)&255, i&255)
}

// BotAddress is a bot's: farms share a few /24s (40 bots each), headless bots come from a hosting
// range, the others from a handful of /24s.
func BotAddress(kind string, i int) string {
	switch kind {
	case BotHeadless:
		return fmt.Sprintf("138.68.%d.%d", (i>>8)&255, i&255) // a DigitalOcean range
	case BotFarm:
		return fmt.Sprintf("46.97.%d.%d", (i/40)%256, 1+i%40)
	}
	return fmt.Sprintf("31.5.%d.%d", (i/200)%256, 1+i%200)
}

// population builds a run's clients and their arrival times (design D-2).
func population(a *Assignment, now time.Time, rng *mrand.Rand) []*client {
	k := a.Knobs
	bots := int(float64(k.Fans)*(*k.BotShare) + 0.5)
	fans := k.Fans - bots
	run := strings.ReplaceAll(a.RunID, "-", "")[:12]
	half := time.Duration(k.DurationSeconds) * time.Second / 2
	arrival := func(early bool) time.Time {
		if early && a.OpensAt.After(now) {
			// Before opening, spread over the pre-queue (it is open from the start of the run).
			return now.Add(time.Duration(rng.Float64() * float64(a.OpensAt.Sub(now)) * 0.9))
		}
		if k.Arrival == "flat" {
			return a.OpensAt.Add(time.Duration(rng.Float64() * float64(half)))
		}
		d := time.Duration(rng.ExpFloat64() * float64(20*time.Second))
		if d > half {
			d = time.Duration(rng.Float64() * float64(half))
		}
		return a.OpensAt.Add(d)
	}
	out := make([]*client, 0, k.Fans)
	for i := 0; i < fans; i++ {
		c := &client{i: i, ip: FanAddress(i), ua: browserUA, print: randHex(16)}
		c.arrive = arrival(rng.Float64() < *k.PrequeueShare)
		out = append(out, c)
	}
	farmPrint := randHex(16)
	for j := 0; j < bots; j++ {
		kind := BotKinds[j%len(BotKinds)]
		c := &client{i: fans + j, bot: kind, ip: BotAddress(kind, j), ua: browserUA, print: randHex(16)}
		switch kind {
		case BotHeadless:
			c.ua = headlessUA
		case BotFarm:
			c.print = farmPrint
		}
		c.arrive = arrival(rng.Float64() < 0.9) // bots come early
		out = append(out, c)
	}
	for _, c := range out {
		c.session = randHex(32)
		c.sid = queue.SessionHash(c.session)
		c.tab = randHex(4)
		c.sub = fmt.Sprintf("guest:sim-%s-%d", run, c.i)
	}
	return out
}

// tally is a run's ground truth and client states.
type tally struct {
	mu        sync.Mutex
	truth     Truth
	fans      map[string]int
	bots      map[string]int
	state     map[int]string
	botPlaces []string
	soldOut   time.Time
}

func (t *tally) set(c *client, s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.fans
	if c.bot != "" {
		m = t.bots
	}
	if old, ok := t.state[c.i]; ok {
		m[old]--
	}
	t.state[c.i] = s
	m[s]++
}

func (t *tally) add(f func(*Truth)) {
	t.mu.Lock()
	f(&t.truth)
	t.mu.Unlock()
}

// session runs one client's journey through the real system.
type session struct {
	r   *runner
	c   *client
	rng *mrand.Rand
}

func (s *session) token() string {
	t, _ := s.r.minter.Mint(s.c.sub, s.r.a.EventID, s.r.a.RunID, s.c.ip, time.Now())
	return t
}

func (s *session) authHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer " + s.token(), "X-EPS-Session": s.c.session, "X-EPS-Tab": s.c.tab, "X-EPS-Client-UA": s.c.ua}
}

type status struct {
	State      string `json:"state"`
	QueueToken string `json:"queueToken"`
	Pass       string `json:"pass"`
	NextPollMs int64  `json:"nextPollMs"`
	Degraded   bool   `json:"degraded"`
}

// placeOf reads the place id from a queue token's payload (the bots' places, for the summary).
func placeOf(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var p struct {
		QID string `json:"qid"`
	}
	_ = json.Unmarshal(b, &p)
	return p.QID
}

// join takes a place (proof of work solved for real, except by headless bots).
func (s *session) join(ctx context.Context) (*status, bool) {
	pow := queue.SolvePow(s.r.a.SaleID, s.c.sid)
	if s.c.bot == BotHeadless {
		pow = ""
	}
	u := s.r.ep.Gateway + "/v1/queue/" + s.r.a.SaleID + "/join"
	for ctx.Err() == nil {
		a := s.r.call.do(ctx, "join", http.MethodPost, u, s.authHeaders(), map[string]any{"pow": pow, "print": s.c.print})
		switch {
		case a.ok():
			var st status
			if json.Unmarshal(a.body, &st) != nil {
				return nil, false
			}
			if st.State == "before" {
				if !sleep(ctx, time.Duration(max(st.NextPollMs, 1000))*time.Millisecond) {
					return nil, false
				}
				continue
			}
			return &st, true
		case a.status == http.StatusTooManyRequests:
			s.r.t.add(func(t *Truth) { t.JoinsRateLimit++ })
			if !sleep(ctx, 10*time.Second+time.Duration(s.rng.IntN(5000))*time.Millisecond) {
				return nil, false
			}
		case a.status >= 500 || a.status == 0:
			if !sleep(ctx, 3*time.Second) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return nil, false
}

// wait polls the place until a pass is given (the server's next poll, floored, with jitter; a
// refresh spammer polls every 200 to 500 ms).
func (s *session) wait(ctx context.Context, st *status) string {
	tok := st.QueueToken
	if s.c.bot != "" {
		if q := placeOf(tok); q != "" {
			s.r.t.mu.Lock()
			s.r.t.botPlaces = append(s.r.t.botPlaces, q)
			s.r.t.mu.Unlock()
		}
	}
	spam := time.Duration(200+s.rng.IntN(300)) * time.Millisecond
	floor := time.Duration(s.r.a.Knobs.PollSeconds) * time.Second
	u := s.r.ep.Gateway + "/v1/queue/" + s.r.a.SaleID + "/status"
	for ctx.Err() == nil {
		if st.State == "admitted" && st.Pass != "" {
			return st.Pass
		}
		if st.State == "ended" {
			return ""
		}
		d := max(time.Duration(st.NextPollMs)*time.Millisecond, floor) + time.Duration(s.rng.IntN(3000))*time.Millisecond
		if s.c.bot == BotSpammer {
			d = spam
		}
		if !sleep(ctx, d) {
			return ""
		}
		a := s.r.call.do(ctx, "status", http.MethodGet, u, map[string]string{"X-EPS-Queue-Token": tok, "X-EPS-Session": s.c.session, "X-EPS-Tab": s.c.tab}, nil)
		if !a.ok() {
			if a.status == http.StatusUnauthorized {
				return ""
			}
			continue
		}
		var next status
		if json.Unmarshal(a.body, &next) == nil {
			if next.QueueToken == "" {
				next.QueueToken = tok
			}
			st = &next
			tok = next.QueueToken
		}
	}
	return ""
}

type order struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	OTPRequired bool   `json:"otpRequired"`
	Questions   []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Required bool   `json:"required"`
	} `json:"questions"`
	Lines []struct {
		Qty int `json:"qty"`
	} `json:"lines"`
}

func (o order) tickets() int {
	n := 0
	for _, l := range o.Lines {
		n += l.Qty
	}
	return n
}

func (s *session) shopHeaders(pass string) map[string]string {
	h := map[string]string{"Authorization": "Bearer " + s.token(), "X-EPS-Session": s.c.session}
	if pass != "" {
		h["X-EPS-Pass"] = pass
	}
	return h
}

// hold creates the order; it waits out the busy answer (degraded mode) like a person would.
func (s *session) hold(ctx context.Context, pass string, offer Offer, qty int) (*order, string) {
	item := map[string]any{"productId": offer.ProductID, "qty": qty}
	if offer.Instance != "" {
		item["instance"] = offer.Instance
	}
	body := map[string]any{"eventId": s.r.a.EventID, "version": s.r.a.Version, "items": []any{item}, "locale": "en"}
	h := s.shopHeaders(pass)
	h["Idempotency-Key"] = "sim-" + randHex(12)
	for ctx.Err() == nil {
		a := s.r.call.do(ctx, "hold", http.MethodPost, s.r.ep.Core+"/v1/shop/orders", h, body)
		switch {
		case a.ok():
			var o order
			if json.Unmarshal(a.body, &o) != nil {
				return nil, "bad_answer"
			}
			return &o, ""
		case a.code == "EPS-BUSY":
			s.r.t.add(func(t *Truth) { t.BusyAnswers++ })
			if !sleep(ctx, max(a.retry, 5*time.Second)) {
				return nil, "cancelled"
			}
		case a.status == http.StatusTooManyRequests || a.status >= 500 || a.status == 0:
			if !sleep(ctx, 5*time.Second) {
				return nil, "cancelled"
			}
		default:
			return nil, a.code
		}
	}
	return nil, "cancelled"
}

// details answers the checkout: a simulated email and every question.
func (s *session) details(ctx context.Context, o *order) bool {
	ans := map[string]any{}
	for _, q := range o.Questions {
		switch q.Kind {
		case "text":
			ans[q.ID] = "Simulated"
		case "choice":
			ans[q.ID] = 0
		case "consent":
			ans[q.ID] = true
		}
	}
	run := strings.ReplaceAll(s.r.a.RunID, "-", "")[:12]
	body := map[string]any{"name": fmt.Sprintf("Simulated fan %d", s.c.i), "email": fmt.Sprintf("fan-%s-%d@sim.invalid", run, s.c.i), "answers": ans}
	a := s.r.call.do(ctx, "details", http.MethodPut, s.r.ep.Core+"/v1/shop/orders/"+o.ID+"/details", s.shopHeaders(""), body)
	return a.ok()
}

// otp reads the simulated code from the answer (as a person reads the screen); 2% type it wrong once.
func (s *session) otp(ctx context.Context, o *order, pass string) bool {
	if s.c.bot != "" {
		s.r.t.add(func(t *Truth) { t.BotsAskedOTP++ })
	} else {
		s.r.t.add(func(t *Truth) { t.FansAskedOTP++ })
	}
	base := s.r.ep.Core + "/v1/shop/orders/" + o.ID + "/otp"
	a := s.r.call.do(ctx, "otp", http.MethodPost, base, s.shopHeaders(pass), map[string]any{})
	if !a.ok() {
		return false
	}
	var ch struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(a.body, &ch) != nil || ch.Code == "" {
		return false
	}
	if s.rng.Float64() < 0.02 {
		_ = s.r.call.do(ctx, "otp", http.MethodPost, base+"/verify", s.shopHeaders(pass), map[string]any{"code": "000000"})
	}
	return s.r.call.do(ctx, "otp", http.MethodPost, base+"/verify", s.shopHeaders(pass), map[string]any{"code": ch.Code}).ok()
}

// pay presses Pay and, on the provider's page in the cluster, Approve (the provider's decline rate
// decides like a bank would).
func (s *session) pay(ctx context.Context, o *order, pass, method string) bool {
	h := s.shopHeaders(pass)
	h["Idempotency-Key"] = "sim-" + randHex(12)
	body := map[string]any{"method": method, "returnUrl": s.r.ep.Return}
	for try := 0; try < 6 && ctx.Err() == nil; try++ {
		a := s.r.call.do(ctx, "pay", http.MethodPost, s.r.ep.Core+"/v1/shop/orders/"+o.ID+"/payments", h, body)
		if a.status == http.StatusServiceUnavailable || a.status == 0 || a.code == "EPS-PAY-PROVIDERS-BUSY" {
			if !sleep(ctx, 5*time.Second) {
				return false
			}
			continue
		}
		if !a.ok() {
			return false
		}
		var res struct {
			ActionURL string `json:"actionUrl"`
		}
		_ = json.Unmarshal(a.body, &res)
		if res.ActionURL != "" {
			s.press(ctx, res.ActionURL)
		}
		return true
	}
	return false
}

// press posts the outcome to the hosted page, rewritten to the provider's in-cluster address.
func (s *session) press(ctx context.Context, action string) {
	u, err := url.Parse(action)
	if err != nil {
		return
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/") // <provider>/pay/<charge>
	if len(parts) != 3 || parts[1] != "pay" {
		return
	}
	base, ok := s.r.ep.PSP[parts[0]]
	if !ok {
		return
	}
	_ = s.r.call.do(ctx, "provider_page", http.MethodPost, base+"/"+parts[0]+"/pay/"+parts[2], nil, url.Values{"outcome": {"approve"}})
}

// follow reads the order until it is paid, failed or expired (every 2 s, at most 150 s).
func (s *session) follow(ctx context.Context, id string) *order {
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		a := s.r.call.do(ctx, "order", http.MethodGet, s.r.ep.Core+"/v1/shop/orders/"+id, s.shopHeaders(""), nil)
		if a.ok() {
			var o order
			if json.Unmarshal(a.body, &o) == nil {
				switch o.State {
				case "paid", "issued", "payment_failed", "expired", "cancelled", "refunded":
					return &o
				}
			}
		}
		if !sleep(ctx, 2*time.Second) {
			return nil
		}
	}
	return nil
}

// skip is a queue skipper's try: hold with no pass and with a forged one (both refused).
func (s *session) skip(ctx context.Context, offer Offer) {
	for _, pass := range []string{"", "v1." + base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"kind":"pass"}`)) + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64))} {
		if o, code := s.hold(ctx, pass, offer, 1); o != nil {
			// The system let it through: cancel so it holds nothing (and it shows in the truth).
			_ = s.r.call.do(ctx, "cancel", http.MethodPost, s.r.ep.Core+"/v1/shop/orders/"+o.ID+"/cancel", s.shopHeaders(""), map[string]any{})
		} else if code != "cancelled" {
			s.r.t.add(func(t *Truth) { t.SkipsRefused++ })
		}
	}
}

func (s *session) qty(offer Offer) int {
	most := min(max(1, s.r.a.CapPerOrder), max(1, offer.MaxPerOrder))
	if s.c.bot != "" {
		return most
	}
	n, _ := strconv.Atoi(s.r.qty.Pick(s.rng.Float64()))
	return min(max(1, n), most)
}

// journey is one client from arrival to the end.
func (s *session) journey(ctx context.Context) {
	t := s.r.t
	if !sleep(ctx, time.Until(s.c.arrive)) {
		return
	}
	t.set(s.c, "joining")
	offer := s.r.pickOffer(s.rng)
	if s.c.bot == BotSkipper {
		s.skip(ctx, offer)
	}
	st, ok := s.join(ctx)
	if !ok {
		t.set(s.c, "refused")
		return
	}
	t.set(s.c, "waiting")
	pass := s.wait(ctx, st)
	if pass == "" {
		t.set(s.c, "lost")
		return
	}
	t.set(s.c, "admitted")
	t.add(func(x *Truth) {
		if s.c.bot != "" {
			x.BotsAdmitted++
		} else {
			x.FansAdmitted++
		}
	})
	if s.c.bot == BotCapper {
		if o, code := s.hold(ctx, pass, offer, 8); o == nil && code != "cancelled" {
			t.add(func(x *Truth) { x.CapsRefused++ })
		}
	}
	o, code := s.hold(ctx, pass, offer, s.qty(offer))
	if o == nil {
		if code == "EPS-HOLD-SOLD-OUT" {
			t.mu.Lock()
			if t.soldOut.IsZero() {
				t.soldOut = time.Now()
			}
			t.mu.Unlock()
			t.set(s.c, "sold_out")
		} else {
			t.set(s.c, "refused")
		}
		return
	}
	t.set(s.c, "holding")
	abandon := *s.r.a.Knobs.AbandonRate / 2
	if s.c.bot == "" && s.rng.Float64() < abandon {
		t.set(s.c, "abandoned")
		t.add(func(x *Truth) { x.FansAbandoned++ })
		return
	}
	if !s.details(ctx, o) {
		t.set(s.c, "refused")
		return
	}
	if o.OTPRequired && !s.otp(ctx, o, pass) {
		t.set(s.c, "refused")
		return
	}
	if s.c.bot == "" && s.rng.Float64() < abandon {
		t.set(s.c, "abandoned")
		t.add(func(x *Truth) { x.FansAbandoned++ })
		return
	}
	method := "hosted_card"
	if s.c.bot == "" {
		method = s.r.pay.Pick(s.rng.Float64())
	}
	t.set(s.c, "paying")
	if !s.pay(ctx, o, pass, method) {
		t.set(s.c, "refused")
		return
	}
	final := s.follow(ctx, o.ID)
	switch {
	case final == nil:
		t.set(s.c, "unknown")
	case final.State == "paid" || final.State == "issued":
		n := final.tickets()
		t.add(func(x *Truth) {
			if s.c.bot != "" {
				x.BotTickets += n
			} else {
				x.FanTickets += n
			}
		})
		t.set(s.c, "paid")
		if s.c.bot == BotCapper {
			// Another order after the cap: refused by the pass (used) or the cap per person.
			if o2, code := s.hold(ctx, pass, offer, s.qty(offer)); o2 == nil && code != "cancelled" {
				t.add(func(x *Truth) { x.CapsRefused++ })
			}
		}
	default:
		if final.State == "payment_failed" {
			t.add(func(x *Truth) { x.FansDeclined++ })
		}
		t.set(s.c, final.State)
	}
}
