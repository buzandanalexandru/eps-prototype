// Package fakepsp is the simulated payment provider (O4, design D-6): charges with idempotency
// keys, a hosted payment page with test outcomes (and no card number field, ever), a simulated
// bank challenge, simulated Apple Pay and Google Pay sheets, provider-saved cards, refunds, signed
// webhooks with retries, and live knobs for failures, declines, latency, outages and dropped or
// late webhooks. No real payment is ever made and no card data exists anywhere in it.
//
// Knobs are kept per merchant (each organiser is its own merchant account at the provider), so
// one organiser's simulated outage never touches another's checkout.
package fakepsp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	mrand "math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Knobs are a merchant's live settings.
type Knobs struct {
	FailureRate     float64 `json:"failure_rate"`      // share of create calls answered 503 before acceptance
	DeclineRate     float64 `json:"decline_rate"`      // share of approvals declined (insufficient funds)
	LatencyMs       int     `json:"latency_ms"`        // added to every API call
	LatencyJitterMs int     `json:"latency_jitter_ms"` // ± around latency_ms
	Outage          bool    `json:"outage"`            // every API call fails (outage_mode)
	OutageMode      string  `json:"outage_mode"`       // hang (until the client gives up) | 503
	TimeoutRate     float64 `json:"timeout_rate"`      // share of create calls accepted but never answered
	WebhookDelayMs  int     `json:"webhook_delay_ms"`
	WebhookDropRate float64 `json:"webhook_drop_rate"`
	ChallengeRate   float64 `json:"challenge_rate"` // share of charges that ask for the bank code
	// ResetsAt is when changed knobs go back to normal by themselves (read only).
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// KnobLife is how long changed knobs last before they reset to the defaults (review D-d).
const KnobLife = 15 * time.Minute

// DefaultKnobs: everything off, 150 ms ± 100 latency; an outage answers 503 unless set to hang.
func DefaultKnobs() Knobs { return Knobs{LatencyMs: 150, LatencyJitterMs: 100, OutageMode: "503"} }

// Validate clamps nothing: it refuses values out of range.
func (k Knobs) Validate() error {
	for _, r := range []float64{k.FailureRate, k.DeclineRate, k.TimeoutRate, k.WebhookDropRate, k.ChallengeRate} {
		if r < 0 || r > 1 {
			return errors.New("rates are between 0 and 1")
		}
	}
	if k.LatencyMs < 0 || k.LatencyMs > 30000 || k.LatencyJitterMs < 0 || k.LatencyJitterMs > 30000 || k.WebhookDelayMs < 0 || k.WebhookDelayMs > 300000 {
		return errors.New("latencies are 0 to 30000 ms (webhook delay up to 300000)")
	}
	if k.OutageMode != "hang" && k.OutageMode != "503" {
		return errors.New("outage_mode is hang or 503")
	}
	return nil
}

// Charge statuses.
const (
	Pending        = "pending"
	RequiresAction = "requires_action"
	Succeeded      = "succeeded"
	Declined       = "declined"
	Refunded       = "refunded"
)

// Methods.
const (
	HostedCard = "hosted_card"
	ApplePay   = "apple_pay"
	GooglePay  = "google_pay"
	SavedCard  = "saved_card"
)

// Saved is a card the provider keeps for a customer.
type Saved struct {
	Token    string `json:"payment_method_token"`
	Label    string `json:"label"`
	Brand    string `json:"brand"`
	Merchant string `json:"merchant"`
	Customer string `json:"customer"`
	Revoked  bool   `json:"revoked"`
}

// Charge is a simulated charge.
type Charge struct {
	ID             string            `json:"charge_id"`
	Provider       string            `json:"provider"`
	Merchant       string            `json:"merchant"`
	AmountMinor    int64             `json:"amount_minor"`
	Currency       string            `json:"currency"`
	IdempotencyKey string            `json:"idempotency_key"`
	Method         string            `json:"method"`
	Token          string            `json:"payment_method_token,omitempty"`
	CustomerRef    string            `json:"customer_ref,omitempty"`
	SaveCard       bool              `json:"save_card"`
	ReturnURL      string            `json:"return_url,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	Metadata       map[string]string `json:"metadata"`
	Status         string            `json:"status"`
	DeclineReason  string            `json:"decline_reason,omitempty"`
	ActionURL      string            `json:"action_url,omitempty"`
	Saved          *Saved            `json:"saved_card,omitempty"`
	Locale         string            `json:"locale"`
	CreatedAt      time.Time         `json:"created_at"`
}

// stored is how a charge is kept (the hidden fields included).
type stored struct {
	Charge
	NeedsChallenge bool   `json:"needs_challenge"`
	ChallengeCode  string `json:"challenge_code"`
	ChallengeTries int    `json:"challenge_tries"`
	RefundKey      string `json:"refund_key"`
}

// Provider is one simulated provider (psp-a or psp-b).
type Provider struct {
	ID            string // a | b
	PublicURL     string // base of the hosted pages, e.g. https://pay.<env>.<domain>/a
	APIKey        string // the core's key for the API
	AdminToken    string // the knobs panel's token
	WebhookSecret []byte
	Redis         *redis.Client // charges, saved cards and knobs; nil keeps them in memory
	Log           *slog.Logger
	HTTP          *http.Client // webhook sender
	// RetryBase is the first webhook retry delay (1 s; tests shorten it).
	RetryBase time.Duration
	Now       func() time.Time

	mu       sync.Mutex
	mem      map[string][]byte
	knobs    map[string]cachedKnobs
	stats    map[string]int64
	inflight sync.WaitGroup
}

type cachedKnobs struct {
	k  Knobs
	at time.Time
}

var chargesTotal = map[string]*metrics.Counter{}
var chargesMu sync.Mutex

func countCharge(provider, outcome string) {
	chargesMu.Lock()
	c, ok := chargesTotal[provider+"/"+outcome]
	if !ok {
		c = metrics.NewCounter("fakepsp_charges_total", "Simulated charges by outcome.", "provider", provider, "outcome", outcome)
		chargesTotal[provider+"/"+outcome] = c
	}
	chargesMu.Unlock()
	c.Inc()
}

func (p *Provider) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Provider) stat(name string) {
	p.mu.Lock()
	if p.stats == nil {
		p.stats = map[string]int64{}
	}
	p.stats[name]++
	p.mu.Unlock()
}

func (p *Provider) key(kind string, rest ...string) string {
	// Idempotency keys may hold colons (inst:<plan>:<part>:<try>); key parts may not.
	safe := make([]string, len(rest))
	for i, r := range rest {
		safe[i] = hex.EncodeToString([]byte(r))
	}
	k, err := redisx.PlatformKey("psp", append([]string{p.ID, kind}, safe...)...)
	if err != nil {
		panic("fakepsp: key: " + err.Error())
	}
	return k
}

func (p *Provider) put(ctx context.Context, k string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if p.Redis == nil {
		p.mu.Lock()
		if p.mem == nil {
			p.mem = map[string][]byte{}
		}
		p.mem[k] = b
		p.mu.Unlock()
		return nil
	}
	return p.Redis.Set(ctx, k, b, ttl).Err()
}

func (p *Provider) get(ctx context.Context, k string, v any) (bool, error) {
	var b []byte
	if p.Redis == nil {
		p.mu.Lock()
		b = p.mem[k]
		p.mu.Unlock()
		if b == nil {
			return false, nil
		}
	} else {
		var err error
		b, err = p.Redis.Get(ctx, k).Bytes()
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	return true, json.Unmarshal(b, v)
}

// setNX stores v only if k is free; it reports whether it stored.
func (p *Provider) setNX(ctx context.Context, k string, v string, ttl time.Duration) (bool, error) {
	if p.Redis == nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.mem == nil {
			p.mem = map[string][]byte{}
		}
		if _, ok := p.mem[k]; ok {
			return false, nil
		}
		p.mem[k] = []byte(strconv.Quote(v))
		return true, nil
	}
	return p.Redis.SetNX(ctx, k, strconv.Quote(v), ttl).Result()
}

const keep = 7 * 24 * time.Hour

// Knobs returns a merchant's knobs (cached 1 s, so a change reaches every replica within 2 s).
func (p *Provider) Knobs(ctx context.Context, merchant string) Knobs {
	p.mu.Lock()
	if c, ok := p.knobs[merchant]; ok && p.now().Sub(c.at) < time.Second {
		p.mu.Unlock()
		return c.k
	}
	p.mu.Unlock()
	k := DefaultKnobs()
	if ok, err := p.get(ctx, p.key("knobs", merchant), &k); err != nil || !ok || (k.ResetsAt != nil && !p.now().Before(*k.ResetsAt)) {
		k = DefaultKnobs()
	}
	p.mu.Lock()
	if p.knobs == nil {
		p.knobs = map[string]cachedKnobs{}
	}
	p.knobs[merchant] = cachedKnobs{k: k, at: p.now()}
	p.mu.Unlock()
	return k
}

// SetKnobs replaces a merchant's knobs.
func (p *Provider) SetKnobs(ctx context.Context, merchant string, k Knobs) error {
	if err := k.Validate(); err != nil {
		return err
	}
	at := p.now().Add(KnobLife).UTC().Truncate(time.Second)
	k.ResetsAt = &at
	if err := p.put(ctx, p.key("knobs", merchant), k, KnobLife); err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.knobs, merchant)
	p.mu.Unlock()
	return nil
}

func roll(rate float64) bool { return rate > 0 && mrand.Float64() < rate }

func randID(prefix string, n int) string {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[x.Int64()]
	}
	return prefix + string(b)
}

func code6() string {
	x, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return strconv.FormatInt(1_000_000+x.Int64(), 10)[1:]
}

func (p *Provider) latency(ctx context.Context, k Knobs) {
	d := k.LatencyMs
	if k.LatencyJitterMs > 0 {
		d += mrand.IntN(2*k.LatencyJitterMs+1) - k.LatencyJitterMs
	}
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(d) * time.Millisecond):
	}
}

func (p *Provider) loadCharge(ctx context.Context, id string) (*stored, error) {
	var s stored
	ok, err := p.get(ctx, p.key("charge", id), &s)
	if err != nil || !ok {
		return nil, err
	}
	return &s, nil
}

func (p *Provider) saveCharge(ctx context.Context, s *stored) error {
	return p.put(ctx, p.key("charge", s.ID), s, keep)
}

func (s *stored) public() Charge {
	c := s.Charge
	return c
}

// webhook is the body the provider posts to the merchant.
type webhook struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // charge.succeeded | charge.declined | charge.refunded
	Created int64  `json:"created"`
	Charge  Charge `json:"charge"`
}

// Sign is the EPS-Signature header value of a body at time t.
func Sign(secret []byte, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(ts + "." + string(body)))
	return "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

// notify sends the charge's current state to its webhook URL, after the merchant's delay, with
// retries (1, 2, 4 ... 64 s; 8 tries), unless the drop knob swallows it.
func (p *Provider) notify(s *stored, typ string) {
	if s.WebhookURL == "" {
		return
	}
	k := p.Knobs(context.Background(), s.Merchant)
	if roll(k.WebhookDropRate) {
		p.stat("webhooks_dropped")
		return
	}
	ev := webhook{ID: randID("evt_", 20), Type: typ, Created: p.now().Unix(), Charge: s.public()}
	body, _ := json.Marshal(ev)
	p.inflight.Add(1)
	go func() {
		defer p.inflight.Done()
		if k.WebhookDelayMs > 0 {
			time.Sleep(time.Duration(k.WebhookDelayMs) * time.Millisecond)
		}
		base := p.RetryBase
		if base <= 0 {
			base = time.Second
		}
		client := p.HTTP
		if client == nil {
			client = &http.Client{Timeout: 5 * time.Second}
		}
		for try := 0; try < 8; try++ {
			req, err := http.NewRequest(http.MethodPost, s.WebhookURL, bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("EPS-Signature", Sign(p.WebhookSecret, p.now(), body))
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 300 {
					p.stat("webhooks_delivered")
					return
				}
			}
			p.stat("webhook_retries")
			time.Sleep(base << try)
		}
		p.stat("webhooks_failed")
	}()
}

// Wait waits for webhooks in flight (tests).
func (p *Provider) Wait() { p.inflight.Wait() }

// Stats returns the provider's counters.
func (p *Provider) Stats() map[string]int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := map[string]int64{}
	for k, v := range p.stats {
		out[k] = v
	}
	return out
}

// finish moves a charge to a final status and notifies the merchant.
func (p *Provider) finish(ctx context.Context, s *stored, status, reason string) error {
	if s.Status == Succeeded || s.Status == Declined || s.Status == Refunded {
		return nil
	}
	s.Status = status
	s.DeclineReason = reason
	if status == Succeeded && s.SaveCard && s.Method != SavedCard {
		sv := &Saved{Token: randID("pm_"+p.ID+"_", 16), Label: "Test Visa •••• 4242", Brand: "visa", Merchant: s.Merchant, Customer: s.CustomerRef}
		if err := p.put(ctx, p.key("pm", sv.Token), sv, 0); err != nil {
			return err
		}
		s.Saved = sv
	}
	if err := p.saveCharge(ctx, s); err != nil {
		return err
	}
	countCharge(p.ID, status)
	p.stat("charges_" + status)
	p.notify(s, "charge."+status)
	return nil
}

// approve is the fan's "approve" (hosted page, wallet sheet, saved card): a decline by the knob,
// the bank challenge, or success.
func (p *Provider) approve(ctx context.Context, s *stored) error {
	k := p.Knobs(ctx, s.Merchant)
	if s.NeedsChallenge {
		s.Status = RequiresAction
		return p.saveCharge(ctx, s)
	}
	if roll(k.DeclineRate) {
		return p.finish(ctx, s, Declined, "insufficient_funds")
	}
	return p.finish(ctx, s, Succeeded, "")
}

// Challenge checks the simulated bank code; three wrong codes decline the charge (3ds_failed).
func (p *Provider) Challenge(ctx context.Context, id, code string) (*stored, error) {
	s, err := p.loadCharge(ctx, id)
	if err != nil || s == nil {
		return nil, err
	}
	if s.Status != RequiresAction {
		return s, nil
	}
	if hmac.Equal([]byte(code), []byte(s.ChallengeCode)) {
		s.NeedsChallenge = false
		if roll(p.Knobs(ctx, s.Merchant).DeclineRate) {
			return s, p.finish(ctx, s, Declined, "insufficient_funds")
		}
		return s, p.finish(ctx, s, Succeeded, "")
	}
	s.ChallengeTries++
	if s.ChallengeTries >= 3 {
		return s, p.finish(ctx, s, Declined, "3ds_failed")
	}
	return s, p.saveCharge(ctx, s)
}

// Outcome applies a test outcome chosen on the hosted page.
func (p *Provider) Outcome(ctx context.Context, id, outcome string) (*stored, error) {
	s, err := p.loadCharge(ctx, id)
	if err != nil || s == nil {
		return nil, err
	}
	if s.Status != Pending {
		return s, nil
	}
	switch outcome {
	case "approve":
		return s, p.approve(ctx, s)
	case "decline_funds":
		return s, p.finish(ctx, s, Declined, "insufficient_funds")
	case "decline_3ds":
		return s, p.finish(ctx, s, Declined, "3ds_failed")
	case "cancel":
		return s, p.finish(ctx, s, Declined, "cancelled")
	}
	return s, errors.New("unknown outcome")
}
