package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/idem"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
)

// Attempt states (D-2).
const (
	Created           = "created"
	Submitted         = "submitted"
	ChallengeRequired = "challenge_required"
	Succeeded         = "succeeded"
	Declined          = "declined"
	Failed            = "failed"
	Unknown           = "unknown"
	RefundPending     = "refund_pending"
	Refunded          = "refunded"
)

// Methods.
const (
	HostedCard = "hosted_card"
	ApplePay   = "apple_pay"
	GooglePay  = "google_pay"
	SavedCard  = "saved_card"
)

// Attempt is one payment attempt.
type Attempt struct {
	ID              string          `json:"id"`
	OrganiserID     string          `json:"-"`
	Purpose         string          `json:"purpose"`
	OrderID         *string         `json:"orderId,omitempty"`
	WalletAccountID *string         `json:"-"`
	Part            *int            `json:"instalmentPart,omitempty"`
	BuyerID         string          `json:"-"`
	Provider        string          `json:"provider"`
	ProviderKey     string          `json:"-"`
	Method          string          `json:"method"`
	SavedMethodID   *string         `json:"-"`
	SaveCard        bool            `json:"saveCard"`
	AmountMinor     int64           `json:"amountMinor"`
	Currency        string          `json:"currency"`
	State           string          `json:"state"`
	ChargeID        *string         `json:"-"`
	ActionURL       *string         `json:"actionUrl,omitempty"`
	ErrorCode       *string         `json:"errorCode,omitempty"`
	FailoverOf      *string         `json:"failoverOf,omitempty"`
	Plan            string          `json:"plan"`
	Offer           *checkout.Offer `json:"-"`
	DeadlineAt      time.Time       `json:"deadlineAt"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// TopupSink credits a wallet when a top-up's charge succeeds (the wallet's ledger).
type TopupSink interface {
	CreditTx(ctx context.Context, tx pgx.Tx, a *Attempt) error
}

// Service is the payment side of the core.
type Service struct {
	Pool      *pgxpool.Pool
	Checkout  *checkout.Service
	Providers map[string]Provider
	// Preferred is the provider tried first (A by default, D-4).
	Preferred string
	Breaker   *Breaker
	// WebhookURL is where a provider posts results (the core's in-cluster address).
	WebhookURL func(provider string) string
	Topups     TopupSink
	Log        *slog.Logger
	Now        func() time.Time
}

var (
	successes = metrics.NewCounter("eps_payments_succeeded_total", "Payment attempts that succeeded.")
	dupes     = metrics.NewCounter("eps_payment_duplicate_success_total", "Second successes on one order (refunded).")
)

var (
	failoverMu       sync.Mutex
	failoverCounters = map[string]*metrics.Counter{}
)

func countFailover(from, to string) {
	failoverMu.Lock()
	defer failoverMu.Unlock()
	c, ok := failoverCounters[from+to]
	if !ok {
		c = metrics.NewCounter("eps_payment_failovers_total", "Payment attempts moved to the other provider.", "from", from, "to", to)
		failoverCounters[from+to] = c
	}
	c.Inc()
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *Service) other(p string) string {
	if p == "a" {
		return "b"
	}
	return "a"
}

func (s *Service) preferred() string {
	if s.Preferred == "b" {
		return "b"
	}
	return "a"
}

const attemptCols = `id::text, organiser_id::text, purpose, order_id::text, wallet_account_id::text, instalment_part, buyer_id, provider, provider_key, method,
	saved_method_id::text, save_card, amount_minor, currency, state, charge_id, action_url, error_code, failover_of::text, deadline_at, created_at, plan, offer`

func scanAttempt(row pgx.Row) (*Attempt, error) {
	var a Attempt
	var offer []byte
	err := row.Scan(&a.ID, &a.OrganiserID, &a.Purpose, &a.OrderID, &a.WalletAccountID, &a.Part, &a.BuyerID, &a.Provider, &a.ProviderKey, &a.Method,
		&a.SavedMethodID, &a.SaveCard, &a.AmountMinor, &a.Currency, &a.State, &a.ChargeID, &a.ActionURL, &a.ErrorCode, &a.FailoverOf, &a.DeadlineAt, &a.CreatedAt, &a.Plan, &offer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSuchCharge
	}
	if len(offer) > 0 {
		var o checkout.Offer
		if json.Unmarshal(offer, &o) == nil {
			a.Offer = &o
		}
	}
	return &a, err
}

// GetAttempt reads an attempt of a known organiser.
func (s *Service) GetAttempt(ctx context.Context, org, id string) (*Attempt, error) {
	var a *Attempt
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		a, err = scanAttempt(tx.QueryRow(ctx, "select "+attemptCols+" from sales.payment_attempts where id = $1", id))
		return err
	})
	return a, err
}

// LatestAttempt is an order's most recent attempt (nil when none).
func (s *Service) LatestAttempt(ctx context.Context, org, orderID string) (*Attempt, error) {
	var a *Attempt
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		a, err = scanAttempt(tx.QueryRow(ctx, "select "+attemptCols+" from sales.payment_attempts where order_id = $1 and purpose = 'order' order by created_at desc limit 1", orderID))
		return err
	})
	if errors.Is(err, ErrNoSuchCharge) {
		return nil, nil
	}
	return a, err
}

func insertAttempt(ctx context.Context, tx pgx.Tx, a *Attempt, offer []byte) error {
	if a.Plan == "" {
		a.Plan = "full"
	}
	_, err := tx.Exec(ctx, `insert into sales.payment_attempts (id, organiser_id, purpose, order_id, wallet_account_id, instalment_part, buyer_id, provider, provider_key,
		method, saved_method_id, save_card, amount_minor, currency, state, failover_of, deadline_at, created_at, plan, offer)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'created', $15, $16, $17, $18, $19)`,
		a.ID, a.OrganiserID, a.Purpose, a.OrderID, a.WalletAccountID, a.Part, a.BuyerID, a.Provider, a.ProviderKey, a.Method, a.SavedMethodID, a.SaveCard,
		a.AmountMinor, a.Currency, a.FailoverOf, a.DeadlineAt, a.CreatedAt, a.Plan, offer)
	return err
}

// moveAttempt changes an attempt's state, only from the given states; it reports whether it moved.
func moveAttempt(ctx context.Context, tx pgx.Tx, id, to string, from []string, set string, args ...any) (bool, error) {
	tag, err := tx.Exec(ctx, `update sales.payment_attempts set state = $2, updated_at = now(),
		resolved_at = case when $2 in ('succeeded', 'declined', 'failed', 'refunded') then now() else resolved_at end`+set+`
		where id = $1 and state = any($3)`, append([]any{id, to, from}, args...)...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func emitPayment(ctx context.Context, tx pgx.Tx, org, eventID, typ string, payload map[string]any) error {
	_, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicPayments, Type: typ, OrganiserID: org, EventID: eventID, Payload: payload})
	return err
}

// ProviderStateEvent records a breaker change (payment.provider_state).
func (s *Service) ProviderStateEvent(ctx context.Context, org, provider, from, to string) {
	_ = db.WithTx(context.WithoutCancel(ctx), s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return emitPayment(ctx, tx, org, "", "payment.provider_state", map[string]any{"provider": provider, "from": from, "to": to})
	})
}

// PayRequest is a fan's Pay press.
type PayRequest struct {
	Method      string `json:"method"`
	SavedCardID string `json:"savedCardId,omitempty"`
	SaveCard    bool   `json:"saveCard,omitempty"`
	Plan        string `json:"plan,omitempty"` // full | instalments
	ReturnURL   string `json:"returnUrl,omitempty"`
	// MerchantName is shown on the provider's page with the order number (review L7).
	MerchantName string `json:"-"`
}

// PayResult is what the fan's screen needs next.
type PayResult struct {
	Attempt *Attempt `json:"attempt"`
	// ActionURL is the provider page to open (hosted card, wallet sheet or bank challenge).
	ActionURL string `json:"actionUrl,omitempty"`
}

type payStored struct {
	AttemptID string          `json:"attemptId,omitempty"`
	Problem   *apperr.Problem `json:"problem,omitempty"`
}

func busy() *apperr.Problem {
	return apperr.New(http.StatusServiceUnavailable, apperr.PayProvidersBusy, "Your tickets are still held. The payment providers are busy: try again in a moment.", true)
}

// Pay starts a payment for an order (one Pay press). A repeat with the same idempotency key and
// body returns the same attempt without a new charge; a press while an attempt is open returns
// that attempt.
func (s *Service) Pay(ctx context.Context, b checkout.Buyer, o *checkout.Order, req PayRequest, key string) (*PayResult, error) {
	scope := "pay:" + b.ID // per buyer: the same key on another order or amount is a mismatch (review L1)
	if key == "" {
		return nil, apperr.New(http.StatusBadRequest, apperr.IdempotencyMismatch, "Idempotency-Key is required.", false)
	}
	replay, err := idem.Begin(ctx, s.Pool, o.OrganiserID, scope, key, idem.Hash(map[string]any{"order": o.ID, "amount": o.TotalMinor, "req": req}))
	switch {
	case errors.Is(err, idem.ErrMismatch):
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.IdempotencyMismatch, "This Idempotency-Key was used for another payment.", false)
	case errors.Is(err, idem.ErrInProgress):
		return nil, apperr.New(http.StatusConflict, apperr.InProgress, "The same payment is still being sent.", true)
	case errors.Is(err, idem.ErrBadKey):
		return nil, apperr.New(http.StatusBadRequest, apperr.IdempotencyMismatch, err.Error(), false)
	case err != nil:
		return nil, err
	}
	if replay != nil {
		var st payStored
		_ = json.Unmarshal(replay.Response, &st)
		if st.Problem != nil {
			return nil, st.Problem
		}
		a, err := s.GetAttempt(ctx, o.OrganiserID, st.AttemptID)
		if err != nil {
			return nil, err
		}
		return s.result(a), nil
	}
	res, err := s.pay(ctx, b, o, req)
	var p *apperr.Problem
	switch {
	case err == nil:
		_ = idem.Finish(ctx, s.Pool, o.OrganiserID, scope, key, http.StatusOK, payStored{AttemptID: res.Attempt.ID})
	case errors.As(err, &p) && !p.Retryable:
		_ = idem.Finish(ctx, s.Pool, o.OrganiserID, scope, key, p.Status, payStored{Problem: p})
	default:
		idem.Abort(ctx, s.Pool, o.OrganiserID, scope, key)
	}
	return res, err
}

func (s *Service) result(a *Attempt) *PayResult {
	r := &PayResult{Attempt: a}
	if a.ActionURL != nil && (a.State == Submitted || a.State == ChallengeRequired) {
		r.ActionURL = *a.ActionURL
	}
	return r
}

func (s *Service) pay(ctx context.Context, b checkout.Buyer, o *checkout.Order, req PayRequest) (*PayResult, error) {
	switch req.Method {
	case HostedCard, ApplePay, GooglePay, SavedCard:
	default:
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "Choose a payment method.", false)
	}
	if o.State == checkout.Paying {
		// An attempt is open: return it (no second attempt, D-2).
		if a, err := s.LatestAttempt(ctx, o.OrganiserID, o.ID); err == nil && a != nil && open(a.State) {
			return s.result(a), nil
		}
	}
	if o.State != checkout.Held && o.State != checkout.PaymentFailed {
		if o.State == checkout.Expired {
			return nil, apperr.New(http.StatusConflict, apperr.HoldExpired, "The hold ended and the tickets were released.", false)
		}
		return nil, apperr.New(http.StatusConflict, apperr.OrderState, "This order cannot be paid now.", false).With("state", o.State)
	}
	if err := s.Checkout.ReadyToPay(ctx, o); err != nil {
		return nil, err
	}
	amount := o.TotalMinor
	saveCard := req.SaveCard && b.Kind == "account"
	var offer *checkout.Offer
	if req.Plan == "instalments" {
		var err error
		offer, err = s.Checkout.Offer(ctx, o)
		if err != nil {
			return nil, err
		}
		if offer == nil {
			return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "Instalments are not offered for this order.", false)
		}
		if req.Method != HostedCard && req.Method != SavedCard {
			return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "Instalments need a card the provider can keep.", false)
		}
		amount = offer.DepositMinor
		saveCard = true
	}
	var savedID *string
	var token string
	provider := ""
	if req.Method == SavedCard {
		m, err := s.SavedMethod(ctx, o.OrganiserID, b.ID, req.SavedCardID)
		if err != nil {
			return nil, err
		}
		savedID, token, provider = &m.ID, m.Token, m.Provider
	}
	providers := []string{s.preferred(), s.other(s.preferred())}
	if provider != "" {
		providers = []string{provider} // a saved card lives at one provider
	}
	// Nothing is sent and the order is not touched when every breaker is open.
	anyOpen := false
	for _, pv := range providers {
		if _, ok := s.Providers[pv]; ok && s.Breaker.Read(ctx, o.OrganiserID, pv).State != Open {
			anyOpen = true
		}
	}
	if !anyOpen {
		return nil, busy()
	}
	plan := "full"
	var offerJSON []byte
	if offer != nil {
		plan = "instalments"
		offerJSON, _ = json.Marshal(offer)
	}
	// Whatever happens below, a press that ends without an open attempt leaves the order
	// payment_failed with its hold, never paying (review H4).
	began := false
	defer func() {
		if !began {
			return
		}
		_ = db.WithTx(context.WithoutCancel(ctx), s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
			var openN int
			if err := tx.QueryRow(ctx, `select count(*) from sales.payment_attempts where order_id = $1 and purpose = 'order'
				and state in ('created', 'submitted', 'challenge_required', 'unknown')`, o.ID).Scan(&openN); err != nil || openN > 0 {
				return err
			}
			var st string
			if err := tx.QueryRow(ctx, "select state from sales.orders where id = $1", o.ID).Scan(&st); err != nil || st != checkout.Paying {
				return err
			}
			return s.Checkout.FailPayingTx(ctx, tx, o.ID, apperr.PayProvidersBusy)
		})
	}()
	var prev *string
	for _, pv := range providers {
		if _, ok := s.Providers[pv]; !ok {
			continue
		}
		a := &Attempt{ID: uuid.NewString(), OrganiserID: o.OrganiserID, Purpose: "order", OrderID: &o.ID, BuyerID: b.ID, Provider: pv, Method: req.Method,
			SavedMethodID: savedID, SaveCard: saveCard, AmountMinor: amount, Currency: o.Currency, FailoverOf: prev, Plan: plan,
			DeadlineAt: s.now().Add(checkout.AttemptLimit), CreatedAt: s.now()}
		a.ProviderKey = a.ID
		var deadline time.Time
		err := db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
			if prev == nil {
				cur, d, err := s.Checkout.BeginPayingTx(ctx, tx, o.ID, plan)
				if err != nil {
					return err
				}
				deadline = d
				if offer != nil {
					if _, err := checkout.CreatePlanTx(ctx, tx, cur, offer); err != nil {
						return err
					}
				}
			} else {
				if err := emitPayment(ctx, tx, o.OrganiserID, o.EventID, "payment.failover", map[string]any{"orderId": o.ID, "from": s.other(pv), "to": pv, "reason": "provider_unavailable", "attemptId": a.ID, "failoverOf": *prev}); err != nil {
					return err
				}
			}
			return insertAttempt(ctx, tx, a, offerJSON)
		})
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.ConstraintName == "attempts_one_open_per_order" {
			if open, err := s.LatestAttempt(ctx, o.OrganiserID, o.ID); err == nil && open != nil {
				return s.result(open), nil
			}
		}
		if err != nil {
			return nil, err
		}
		began = true
		if prev == nil {
			s.Checkout.ExtendHold(ctx, o, deadline)
		} else {
			countFailover(s.other(pv), pv)
			s.log().Info("payment failover", "organiser", o.OrganiserID, "order", o.ID, "from", s.other(pv), "to", pv)
		}
		done, err := s.submit(ctx, a, ChargeRequest{AmountMinor: amount, Currency: o.Currency, Method: req.Method, Token: token, CustomerRef: b.ID,
			SaveCard: saveCard, ReturnURL: req.ReturnURL, Merchant: o.OrganiserID, Locale: o.Locale,
			Metadata: map[string]string{"organiser_id": o.OrganiserID, "attempt_id": a.ID, "order_id": o.ID, "purpose": "order", "merchant_name": req.MerchantName, "order_ref": o.ID[:8]}})
		if err == nil {
			return s.result(done), nil
		}
		if !errors.Is(err, errFailover) {
			return nil, err
		}
		prev = &a.ID
	}
	// Every provider failed before accepting: the deferred check leaves the order payment_failed
	// with its hold (retry allowed).
	return nil, busy()
}

var errFailover = errors.New("payments: failed before acceptance")

func open(state string) bool {
	return state == Created || state == Submitted || state == ChallengeRequired || state == Unknown
}

// submit sends an attempt's charge to its provider and applies the answer. errFailover means the
// provider did not accept it (the attempt failed; another provider may be tried). An answer that
// never came is resolved by asking the provider by the idempotency key before anything else.
func (s *Service) submit(ctx context.Context, a *Attempt, r ChargeRequest) (*Attempt, error) {
	pv := s.Providers[a.Provider]
	r.IdempotencyKey = a.ProviderKey
	if s.WebhookURL != nil {
		r.WebhookURL = s.WebhookURL(a.Provider)
	}
	// The breaker decides right before the call it guards (review H2).
	if !s.Breaker.Allow(ctx, a.OrganiserID, a.Provider) {
		if err := s.failAttempt(ctx, a, "breaker_open", false); err != nil {
			return nil, err
		}
		return nil, errFailover
	}
	ch, err := pv.Create(ctx, r)
	switch {
	case err == nil:
		s.Breaker.Record(ctx, a.OrganiserID, a.Provider, true)
		if err := s.Apply(ctx, a.OrganiserID, a.ID, ch); err != nil {
			return nil, err
		}
		return s.GetAttempt(ctx, a.OrganiserID, a.ID)
	case errors.Is(err, ErrUnavailable):
		s.Breaker.Record(ctx, a.OrganiserID, a.Provider, false)
		if err := s.failAttempt(ctx, a, "provider_unavailable", false); err != nil {
			return nil, err
		}
		return nil, errFailover
	case errors.Is(err, ErrTimeout):
		s.Breaker.Record(ctx, a.OrganiserID, a.Provider, false)
		if err := db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
			_, err := moveAttempt(ctx, tx, a.ID, Unknown, []string{Created}, "")
			return err
		}); err != nil {
			return nil, err
		}
		// Did the provider charge? Ask by the attempt's key before trying anyone else.
		found, ferr := pv.FindByKey(ctx, a.ProviderKey)
		switch {
		case ferr == nil:
			if err := s.Apply(ctx, a.OrganiserID, a.ID, found); err != nil {
				return nil, err
			}
			return s.GetAttempt(ctx, a.OrganiserID, a.ID)
		case errors.Is(ferr, ErrNoSuchCharge):
			if err := s.failAttempt(ctx, a, "no_such_charge", false); err != nil {
				return nil, err
			}
			return nil, errFailover
		default:
			// Still unknown: the reconciler resolves it; the order stays paying with its hold.
			return s.GetAttempt(ctx, a.OrganiserID, a.ID)
		}
	case errors.Is(err, ErrRefused):
		if err := s.failAttempt(ctx, a, "refused", true); err != nil {
			return nil, err
		}
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SavedCardRefused, "The provider refused this payment method.", false)
	default:
		return nil, err
	}
}

// failAttempt marks an attempt failed before acceptance; with final the order also goes to
// payment_failed (no failover follows).
func (s *Service) failAttempt(ctx context.Context, a *Attempt, reason string, final bool) error {
	return db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
		code := apperr.PayProvidersBusy
		if _, err := moveAttempt(ctx, tx, a.ID, Failed, []string{Created, Unknown}, ", error_code = $4, failure_reason = $5", code, reason); err != nil {
			return err
		}
		if final && a.OrderID != nil && a.Purpose == "order" {
			return s.Checkout.FailPayingTx(ctx, tx, *a.OrderID, code)
		}
		return nil
	})
}

// DeclineCode maps a provider's decline reason to the fan's error code.
func DeclineCode(reason string) string {
	switch reason {
	case "3ds_failed":
		return apperr.Pay3DSFailed
	case "cancelled":
		return apperr.PayCancelled
	}
	return apperr.PayDeclined
}

// Apply applies a provider's view of a charge to its attempt (create answer, webhook or status
// query). It is idempotent: every move is guarded by the attempt's current state.
func (s *Service) Apply(ctx context.Context, org, attemptID string, ch Charge) error {
	a, err := s.GetAttempt(ctx, org, attemptID)
	if err != nil {
		return err
	}
	switch ch.Status {
	case "pending":
		return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			_, err := moveAttempt(ctx, tx, a.ID, Submitted, []string{Created, Unknown}, ", charge_id = $4, action_url = $5", ch.ID, nullable(ch.ActionURL))
			return err
		})
	case "requires_action":
		return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			_, err := moveAttempt(ctx, tx, a.ID, ChallengeRequired, []string{Created, Unknown, Submitted}, ", charge_id = $4, action_url = $5", ch.ID, nullable(ch.ActionURL))
			return err
		})
	case "succeeded":
		return s.succeeded(ctx, a, ch)
	case "declined":
		return s.declined(ctx, a, ch)
	case "refunded":
		return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			moved, err := moveAttempt(ctx, tx, a.ID, Refunded, []string{RefundPending, Succeeded}, ", charge_id = coalesce(charge_id, $4)", ch.ID)
			if err != nil || !moved {
				return err
			}
			return emitPayment(ctx, tx, org, "", "payment.refunded", map[string]any{"attemptId": a.ID, "orderId": a.OrderID, "amountMinor": a.AmountMinor})
		})
	}
	return fmt.Errorf("payments: unknown charge status %q", ch.Status)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// successFrom are the states a success may arrive in (failed: a timed-out attempt whose charge
// went through after all).
var successFrom = []string{Created, Submitted, ChallengeRequired, Unknown, Failed}

func (s *Service) markSucceededTx(ctx context.Context, tx pgx.Tx, a *Attempt, ch Charge) (bool, error) {
	moved, err := moveAttempt(ctx, tx, a.ID, Succeeded, successFrom, ", charge_id = $4, error_code = null", ch.ID)
	if err != nil || !moved {
		return moved, err
	}
	successes.Inc()
	if ch.Saved != nil && ch.Saved.Token != "" && a.SaveCard {
		id, err := saveMethodTx(ctx, tx, a, ch)
		if err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, "update sales.payment_attempts set saved_method_id = $2 where id = $1", a.ID, id); err != nil {
			return false, err
		}
		a.SavedMethodID = &id
	}
	return true, emitPayment(ctx, tx, a.OrganiserID, "", "payment.succeeded", map[string]any{"attemptId": a.ID, "orderId": a.OrderID, "provider": a.Provider, "method": a.Method, "amountMinor": a.AmountMinor, "purpose": a.Purpose})
}

func (s *Service) succeeded(ctx context.Context, a *Attempt, ch Charge) error {
	switch a.Purpose {
	case "topup":
		return db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
			moved, err := s.markSucceededTx(ctx, tx, a, ch)
			if err != nil || !moved {
				return err
			}
			if s.Topups == nil {
				return errors.New("payments: no wallet for top-ups")
			}
			return s.Topups.CreditTx(ctx, tx, a)
		})
	case "instalment":
		return db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
			moved, err := s.markSucceededTx(ctx, tx, a, ch)
			if err != nil || !moved {
				return err
			}
			return s.partResult(ctx, tx, a, true)
		})
	}
	if a.State == Succeeded || a.State == RefundPending || a.State == Refunded {
		return nil // already applied (a repeated webhook)
	}
	out, err := s.Checkout.Complete(ctx, a.OrganiserID, *a.OrderID, "payment confirmed by provider "+a.Provider, func(tx pgx.Tx, o *checkout.Order) error {
		moved, err := s.markSucceededTx(ctx, tx, a, ch)
		if err != nil {
			return err
		}
		if !moved {
			return errors.New("payments: attempt changed state")
		}
		if o.PayPlan == "instalments" {
			sm := ""
			if a.SavedMethodID != nil {
				sm = *a.SavedMethodID
			}
			return checkout.ActivatePlanTx(ctx, tx, o.ID, sm)
		}
		return nil
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.ConstraintName == "attempts_one_success_per_order" {
		out = checkout.Outcome{AlreadyPaid: true}
		err = nil
	}
	if err != nil {
		return err
	}
	if out.Refund || out.AlreadyPaid {
		cur, err := s.GetAttempt(ctx, a.OrganiserID, a.ID)
		if err != nil {
			return err
		}
		if cur.State == Succeeded && !out.Refund {
			return nil // this attempt is the one that paid
		}
		reason := "late"
		if out.AlreadyPaid {
			reason = "duplicate"
			dupes.Inc()
		}
		return s.refund(ctx, cur, ch.ID, reason)
	}
	return nil
}

// refund refunds a charge through the provider that took it, under its own idempotency key.
func (s *Service) refund(ctx context.Context, a *Attempt, chargeID, reason string) error {
	err := db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
		moved, err := moveAttempt(ctx, tx, a.ID, RefundPending, append([]string{Succeeded}, successFrom...), ", charge_id = coalesce(charge_id, $4)", chargeID)
		if err != nil || !moved {
			return err
		}
		typ := "payment.refund_started"
		if reason == "duplicate" {
			typ = "payment.duplicate_success" // an alert: should never happen
		}
		return emitPayment(ctx, tx, a.OrganiserID, "", typ, map[string]any{"attemptId": a.ID, "orderId": a.OrderID, "reason": reason, "provider": a.Provider})
	})
	if err != nil {
		return err
	}
	return s.sendRefund(ctx, a.OrganiserID, a.ID)
}

// sendRefund asks the provider to refund a refund_pending attempt (retried by the reconciler).
func (s *Service) sendRefund(ctx context.Context, org, attemptID string) error {
	a, err := s.GetAttempt(ctx, org, attemptID)
	if err != nil || a.State != RefundPending || a.ChargeID == nil {
		return err
	}
	ch, err := s.Providers[a.Provider].Refund(ctx, *a.ChargeID, "refund-"+a.ID)
	if err != nil {
		s.log().Warn("refund not sent yet", "attempt", a.ID, "error", err.Error())
		return nil
	}
	return s.Apply(ctx, org, a.ID, ch)
}

func (s *Service) declined(ctx context.Context, a *Attempt, ch Charge) error {
	code := DeclineCode(ch.DeclineReason)
	return db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error {
		moved, err := moveAttempt(ctx, tx, a.ID, Declined, []string{Created, Submitted, ChallengeRequired, Unknown}, ", charge_id = $4, error_code = $5", ch.ID, code)
		if err != nil || !moved {
			return err
		}
		if err := emitPayment(ctx, tx, a.OrganiserID, "", "payment.declined", map[string]any{"attemptId": a.ID, "orderId": a.OrderID, "code": code, "purpose": a.Purpose}); err != nil {
			return err
		}
		switch a.Purpose {
		case "order":
			return s.Checkout.FailPayingTx(ctx, tx, *a.OrderID, code)
		case "instalment":
			return s.partResult(ctx, tx, a, false)
		}
		return nil
	})
}
