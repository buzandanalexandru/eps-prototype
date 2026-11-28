package fakepsp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
)

// CreateRequest is POST /v1/charges.
type CreateRequest struct {
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

var (
	keyPattern   = regexp.MustCompile(`^[A-Za-z0-9:_-]{8,120}$`)
	tokenPattern = regexp.MustCompile(`^pm_[ab]_[A-Za-z0-9]{8,64}$`)
	idPattern    = regexp.MustCompile(`^ch_[A-Za-z0-9]{20,40}$`)
)

func writeErr(w http.ResponseWriter, status int, code string) {
	httpx.WriteJSON(w, status, map[string]string{"error": code})
}

func bearer(r *http.Request, want string) bool {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func okURL(raw string, needed bool) bool {
	if raw == "" {
		return !needed
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && len(raw) <= 2000
}

// Routes mounts the provider's API (in-cluster: the core's API key), the hosted pages (public,
// under /<id>/pay/) and the admin knobs (admin token) on mux.
func (p *Provider) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/charges", p.api(p.create))
	mux.HandleFunc("GET /v1/charges", p.api(p.findByKey))
	mux.HandleFunc("GET /v1/charges/{id}", p.api(p.getCharge))
	mux.HandleFunc("POST /v1/charges/{id}/challenge", p.api(p.challengeAPI))
	mux.HandleFunc("POST /v1/refunds", p.api(p.refund))
	mux.HandleFunc("DELETE /v1/payment-methods/{token}", p.api(p.revoke))
	mux.HandleFunc("GET /admin/knobs", p.admin(p.getKnobs))
	mux.HandleFunc("PUT /admin/knobs", p.admin(p.putKnobs))
	mux.HandleFunc("GET /admin/stats", p.admin(func(w http.ResponseWriter, r *http.Request) { httpx.WriteJSON(w, http.StatusOK, p.Stats()) }))
	base := "/" + p.ID + "/pay/{id}"
	mux.HandleFunc("GET "+base, p.page)
	mux.HandleFunc("POST "+base, p.pagePost)
	mux.HandleFunc("POST "+base+"/challenge", p.pageChallenge)
}

func (p *Provider) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !bearer(r, p.APIKey) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
}

func (p *Provider) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !bearer(r, p.AdminToken) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
}

// outage applies the merchant's outage, latency and failure knobs to an API call; it reports
// whether the call may go on.
func (p *Provider) gate(w http.ResponseWriter, r *http.Request, merchant string) (Knobs, bool) {
	k := p.Knobs(r.Context(), merchant)
	if k.Outage {
		p.stat("outage_calls")
		if k.OutageMode == "503" {
			writeErr(w, http.StatusServiceUnavailable, "provider_unavailable")
			return k, false
		}
		<-r.Context().Done() // hang until the client gives up
		return k, false
	}
	p.latency(r.Context(), k)
	return k, r.Context().Err() == nil
}

func (p *Provider) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if req.Merchant == "" || len(req.Merchant) > 64 || !keyPattern.MatchString(req.IdempotencyKey) || req.AmountMinor <= 0 || req.AmountMinor > 100_000_000 ||
		(req.Currency != "RON" && req.Currency != "EUR") || !okURL(req.WebhookURL, false) || !okURL(req.ReturnURL, req.Method != SavedCard) {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	switch req.Method {
	case HostedCard, ApplePay, GooglePay, SavedCard:
	default:
		writeErr(w, http.StatusBadRequest, "invalid_method")
		return
	}
	k, ok := p.gate(w, r, req.Merchant)
	if !ok {
		return
	}
	ctx := r.Context()
	// A repeated idempotency key returns the original charge, never a second one.
	if s, err := p.byKey(ctx, req.IdempotencyKey); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	} else if s != nil {
		if s.AmountMinor != req.AmountMinor || s.Merchant != req.Merchant || s.Method != req.Method {
			writeErr(w, http.StatusConflict, "idempotency_key_reused")
			return
		}
		p.stat("idempotent_replays")
		httpx.WriteJSON(w, http.StatusOK, s.public())
		return
	}
	if roll(k.FailureRate) {
		p.stat("failures")
		writeErr(w, http.StatusServiceUnavailable, "provider_unavailable")
		return
	}
	var saved *Saved
	if req.Method == SavedCard {
		var sv Saved
		found, err := p.get(ctx, p.key("pm", req.Token), &sv)
		if !tokenPattern.MatchString(req.Token) || err != nil || !found || sv.Revoked || sv.Merchant != req.Merchant || sv.Customer != req.CustomerRef {
			writeErr(w, http.StatusPaymentRequired, "invalid_payment_method")
			return
		}
		saved = &sv
	}
	locale := req.Locale
	if locale != "en" {
		locale = "ro"
	}
	s := &stored{Charge: Charge{ID: randID("ch_", 24), Provider: p.ID, Merchant: req.Merchant, AmountMinor: req.AmountMinor, Currency: req.Currency,
		IdempotencyKey: req.IdempotencyKey, Method: req.Method, Token: req.Token, CustomerRef: req.CustomerRef, SaveCard: req.SaveCard,
		ReturnURL: req.ReturnURL, WebhookURL: req.WebhookURL, Metadata: req.Metadata, Status: Pending, Locale: locale, CreatedAt: p.now().UTC()},
		NeedsChallenge: roll(k.ChallengeRate), ChallengeCode: code6()}
	if s.Metadata == nil {
		s.Metadata = map[string]string{}
	}
	if saved != nil {
		s.Saved = saved
	}
	s.ActionURL = p.PublicURL + "/pay/" + s.ID
	won, err := p.setNX(ctx, p.key("idem", req.IdempotencyKey), s.ID, keep)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if !won { // a concurrent call with the same key won
		if other, _ := p.byKey(ctx, req.IdempotencyKey); other != nil {
			httpx.WriteJSON(w, http.StatusOK, other.public())
			return
		}
	}
	if err := p.saveCharge(ctx, s); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	p.stat("charges_created")
	if req.Method == SavedCard {
		// One tap: the provider charges the saved card at once (or asks for the bank code).
		if err := p.approve(context.WithoutCancel(ctx), s); err != nil {
			writeErr(w, http.StatusInternalServerError, "store")
			return
		}
	}
	if roll(k.TimeoutRate) {
		// Accepted, but the answer is lost: the client times out; a status check finds the charge.
		p.stat("timeouts")
		<-ctx.Done()
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, s.public())
}

func (p *Provider) byKey(ctx context.Context, key string) (*stored, error) {
	var id string
	ok, err := p.get(ctx, p.key("idem", key), &id)
	if err != nil || !ok {
		return nil, err
	}
	return p.loadCharge(ctx, id)
}

func (p *Provider) findByKey(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("idempotency_key")
	if !keyPattern.MatchString(key) {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	// The merchant's outage applies to lookups too (a provider that is down answers nothing).
	if m := r.URL.Query().Get("merchant"); m != "" {
		if _, ok := p.gate(w, r, m); !ok {
			return
		}
	}
	s, err := p.byKey(r.Context(), key)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if s == nil {
		writeErr(w, http.StatusNotFound, "no_such_charge")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s.public())
}

func (p *Provider) getCharge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		writeErr(w, http.StatusNotFound, "no_such_charge")
		return
	}
	s, err := p.loadCharge(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if s == nil {
		writeErr(w, http.StatusNotFound, "no_such_charge")
		return
	}
	if _, ok := p.gate(w, r, s.Merchant); !ok {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s.public())
}

func (p *Provider) challengeAPI(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	s, err := p.Challenge(r.Context(), r.PathValue("id"), body.Code)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if s == nil {
		writeErr(w, http.StatusNotFound, "no_such_charge")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s.public())
}

func (p *Provider) refund(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChargeID       string `json:"charge_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || !keyPattern.MatchString(body.IdempotencyKey) {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	ctx := r.Context()
	s, err := p.loadCharge(ctx, body.ChargeID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	if s == nil {
		writeErr(w, http.StatusNotFound, "no_such_charge")
		return
	}
	if _, ok := p.gate(w, r, s.Merchant); !ok {
		return
	}
	switch {
	case s.Status == Refunded:
		httpx.WriteJSON(w, http.StatusOK, s.public())
		return
	case s.Status != Succeeded:
		writeErr(w, http.StatusConflict, "not_refundable")
		return
	}
	s.RefundKey = body.IdempotencyKey
	s.Status = Refunded
	if err := p.saveCharge(ctx, s); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	countCharge(p.ID, Refunded)
	p.stat("refunds")
	p.notify(s, "charge.refunded")
	httpx.WriteJSON(w, http.StatusOK, s.public())
}

func (p *Provider) revoke(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	var sv Saved
	found, err := p.get(r.Context(), p.key("pm", tok), &sv)
	if !tokenPattern.MatchString(tok) || err != nil || !found {
		writeErr(w, http.StatusNotFound, "no_such_payment_method")
		return
	}
	sv.Revoked = true
	if err := p.put(r.Context(), p.key("pm", tok), sv, 0); err != nil {
		writeErr(w, http.StatusInternalServerError, "store")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

func merchantParam(r *http.Request) (string, error) {
	m := r.URL.Query().Get("merchant")
	if m == "" || len(m) > 64 || strings.ContainsAny(m, ":{} ") {
		return "", errors.New("merchant is required")
	}
	return m, nil
}

func (p *Provider) getKnobs(w http.ResponseWriter, r *http.Request) {
	m, err := merchantParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p.Knobs(r.Context(), m))
}

func (p *Provider) putKnobs(w http.ResponseWriter, r *http.Request) {
	m, err := merchantParam(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request")
		return
	}
	k := DefaultKnobs()
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&k); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := p.SetKnobs(r.Context(), m, k); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, k)
}
