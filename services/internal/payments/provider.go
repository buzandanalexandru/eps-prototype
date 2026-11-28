// Package payments takes payment for orders, wallet top-ups and instalment parts through the
// simulated providers (O3, design D-2 to D-6): one adapter per provider, attempts as a persisted
// state machine, idempotency keys towards the client and the provider, a circuit breaker shared
// by every core through Redis with failover from A to B, signed webhooks and a reconciler for
// outcomes nobody reported. EPS never sees card data: only a provider's charge id, a saved-card
// token and its label.
package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// Charge is a provider's view of a charge (the fields EPS reads).
type Charge struct {
	ID             string            `json:"charge_id"`
	Provider       string            `json:"provider"`
	Merchant       string            `json:"merchant"`
	AmountMinor    int64             `json:"amount_minor"`
	Currency       string            `json:"currency"`
	IdempotencyKey string            `json:"idempotency_key"`
	Method         string            `json:"method"`
	Status         string            `json:"status"` // pending | requires_action | succeeded | declined | refunded
	DeclineReason  string            `json:"decline_reason,omitempty"`
	ActionURL      string            `json:"action_url,omitempty"`
	Metadata       map[string]string `json:"metadata"`
	Saved          *struct {
		Token string `json:"payment_method_token"`
		Label string `json:"label"`
		Brand string `json:"brand"`
	} `json:"saved_card,omitempty"`
}

// ChargeRequest asks a provider for a charge.
type ChargeRequest struct {
	AmountMinor    int64             `json:"amount_minor"`
	Currency       string            `json:"currency"`
	IdempotencyKey string            `json:"idempotency_key"`
	Method         string            `json:"method"`
	Token          string            `json:"payment_method_token,omitempty"`
	CustomerRef    string            `json:"customer_ref,omitempty"`
	SaveCard       bool              `json:"save_card"`
	ReturnURL      string            `json:"return_url,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	Merchant       string            `json:"merchant"`
	Locale         string            `json:"locale,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Adapter errors.
var (
	// ErrUnavailable: the provider certainly did not take the call: the connection could not be
	// made, or it answered 503 (its "not accepted" answer). Another provider may be tried.
	ErrUnavailable = errors.New("payments: provider unavailable")
	// ErrTimeout: no usable answer (timeout, connection reset, 502/504 or another 5xx); the provider
	// may have charged, so the attempt is unknown until asked by its key (review H3).
	ErrTimeout = errors.New("payments: provider outcome unknown")
	// ErrNoSuchCharge: the provider has no charge for the id or key.
	ErrNoSuchCharge = errors.New("payments: no such charge")
	// ErrRefused: the provider refused the request itself (an invalid saved card, ...).
	ErrRefused = errors.New("payments: refused by the provider")
)

// Provider is one payment provider behind the adapter interface (a real PSP is a swap).
type Provider interface {
	Name() string
	Create(ctx context.Context, r ChargeRequest) (Charge, error)
	Get(ctx context.Context, chargeID string) (Charge, error)
	FindByKey(ctx context.Context, key string) (Charge, error)
	Refund(ctx context.Context, chargeID, key string) (Charge, error)
	Revoke(ctx context.Context, token string) error
}

// HTTPProvider talks to a simulated provider over the network.
type HTTPProvider struct {
	ID      string
	BaseURL string // in-cluster URL, e.g. http://eps-psp-a
	APIKey  string
	// Timeout per call (3 s, D-2).
	Timeout time.Duration
	Client  *http.Client
}

// Name is the provider id (a or b).
func (p *HTTPProvider) Name() string { return p.ID }

// SharedTransport is the connection pool for every call to the providers (review L3).
var SharedTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConns:        256,
	MaxIdleConnsPerHost: 128,
	IdleConnTimeout:     90 * time.Second,
}

var callLatency = map[string]*metrics.Histogram{}

var latencyMu sync.Mutex

func observe(provider, op string, d time.Duration) {
	k := provider + "/" + op
	latencyMu.Lock()
	defer latencyMu.Unlock()
	h, ok := callLatency[k]
	if !ok {
		h = metrics.NewHistogram("eps_provider_call_seconds", "Latency of calls to the payment providers.", metrics.DefaultLatencyBuckets, "provider", provider, "op", op)
		callLatency[k] = h
	}
	h.Observe(d.Seconds())
}

// dialFailed reports a connection that was never made (nothing reached the provider).
func dialFailed(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

func (p *HTTPProvider) call(ctx context.Context, method, path string, body any, out any) error {
	start := time.Now()
	defer func() { observe(p.ID, method+" "+strings.SplitN(strings.TrimPrefix(path, "/v1/"), "/", 2)[0], time.Since(start)) }()
	t := p.Timeout
	if t <= 0 {
		t = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	c := p.Client
	if c == nil {
		c = &http.Client{Transport: SharedTransport}
	}
	resp, err := c.Do(req)
	if err != nil {
		if dialFailed(err) {
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		// Timeout, reset, broken connection: the request may have reached the provider.
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusServiceUnavailable:
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: status %d", ErrTimeout, resp.StatusCode)
	case resp.StatusCode == http.StatusNotFound:
		return ErrNoSuchCharge
	case resp.StatusCode >= 400:
		return fmt.Errorf("%w: status %d %s", ErrRefused, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (p *HTTPProvider) Create(ctx context.Context, r ChargeRequest) (Charge, error) {
	var c Charge
	err := p.call(ctx, http.MethodPost, "/v1/charges", r, &c)
	return c, err
}

func (p *HTTPProvider) Get(ctx context.Context, id string) (Charge, error) {
	var c Charge
	err := p.call(ctx, http.MethodGet, "/v1/charges/"+url.PathEscape(id), nil, &c)
	return c, err
}

func (p *HTTPProvider) FindByKey(ctx context.Context, key string) (Charge, error) {
	var c Charge
	err := p.call(ctx, http.MethodGet, "/v1/charges?idempotency_key="+url.QueryEscape(key), nil, &c)
	return c, err
}

func (p *HTTPProvider) Refund(ctx context.Context, chargeID, key string) (Charge, error) {
	var c Charge
	err := p.call(ctx, http.MethodPost, "/v1/refunds", map[string]string{"charge_id": chargeID, "idempotency_key": key}, &c)
	return c, err
}

func (p *HTTPProvider) Revoke(ctx context.Context, token string) error {
	return p.call(ctx, http.MethodDelete, "/v1/payment-methods/"+url.PathEscape(token), nil, nil)
}

// Knobs is the provider admin API the Studio panel reaches through the core.
type Knobs struct {
	BaseURL    string
	AdminToken string
	Client     *http.Client
}

// Get reads a merchant's knobs as raw JSON.
func (k *Knobs) Get(ctx context.Context, merchant string) (json.RawMessage, error) {
	return k.do(ctx, http.MethodGet, merchant, nil)
}

// Put replaces a merchant's knobs.
func (k *Knobs) Put(ctx context.Context, merchant string, body json.RawMessage) (json.RawMessage, error) {
	return k.do(ctx, http.MethodPut, merchant, body)
}

func (k *Knobs) do(ctx context.Context, method, merchant string, body json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, k.BaseURL+"/admin/knobs?merchant="+url.QueryEscape(merchant), rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+k.AdminToken)
	c := k.Client
	if c == nil {
		c = &http.Client{Transport: SharedTransport}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: status %d %s", ErrRefused, resp.StatusCode, string(b))
	}
	return b, nil
}
