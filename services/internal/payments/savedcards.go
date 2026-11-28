package payments

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// SavedMethod is a provider-saved card as EPS knows it: provider, token and label only (D13).
type SavedMethod struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Token     string    `json:"-"`
	Label     string    `json:"label"`
	Brand     string    `json:"brand"`
	CreatedAt time.Time `json:"createdAt"`
}

func refused() *apperr.Problem {
	return apperr.New(http.StatusForbidden, apperr.SavedCardRefused, "This saved card cannot be used here.", false)
}

func saveMethodTx(ctx context.Context, tx pgx.Tx, a *Attempt, ch Charge) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `insert into sales.saved_payment_methods (organiser_id, owner_id, provider, token, label, brand)
		values ($1, $2, $3, $4, $5, $6) on conflict (organiser_id, provider, token) do update set label = excluded.label returning id::text`,
		a.OrganiserID, a.BuyerID, a.Provider, ch.Saved.Token, ch.Saved.Label, ch.Saved.Brand).Scan(&id)
	return id, err
}

// SavedMethods lists an owner's cards for one organiser (a card saved for one organiser is never
// offered at another's shop).
func (s *Service) SavedMethods(ctx context.Context, org, owner string) ([]SavedMethod, error) {
	out := []SavedMethod{}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id::text, provider, token, label, brand, created_at from sales.saved_payment_methods
			where owner_id = $1 and deleted_at is null order by created_at`, owner)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m SavedMethod
			if err := rows.Scan(&m.ID, &m.Provider, &m.Token, &m.Label, &m.Brand, &m.CreatedAt); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// SavedMethod returns one of an owner's cards for an organiser, or EPS-SAVED-CARD-REFUSED.
func (s *Service) SavedMethod(ctx context.Context, org, owner, id string) (*SavedMethod, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, refused()
	}
	var m SavedMethod
	var ownerID string
	var deleted *time.Time
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select id::text, provider, token, label, brand, created_at, owner_id, deleted_at from sales.saved_payment_methods where id = $1`, id).
			Scan(&m.ID, &m.Provider, &m.Token, &m.Label, &m.Brand, &m.CreatedAt, &ownerID, &deleted)
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (ownerID != owner || deleted != nil)) {
		return nil, refused()
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteSavedMethod removes an owner's card and revokes its token at the provider.
func (s *Service) DeleteSavedMethod(ctx context.Context, org, owner, id string) error {
	m, err := s.SavedMethod(ctx, org, owner, id)
	if err != nil {
		return err
	}
	if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update sales.saved_payment_methods set deleted_at = now() where id = $1 and owner_id = $2`, id, owner)
		return err
	}); err != nil {
		return err
	}
	if pv, ok := s.Providers[m.Provider]; ok {
		if err := pv.Revoke(ctx, m.Token); err != nil && !errors.Is(err, ErrNoSuchCharge) {
			s.log().Warn("token revoke failed", "provider", m.Provider, "error", err.Error())
		}
	}
	return nil
}

// ChargePart charges one instalment part with the plan's saved card (checkout.PartCharger).
func (s *Service) ChargePart(ctx context.Context, p *checkout.Plan, n int) error {
	if p.SavedMethodID == nil {
		return errors.New("payments: plan has no saved card")
	}
	var a *Attempt
	var token string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, p.OrganiserID, func(tx pgx.Tx) error {
		var amount int64
		var tries int
		var state string
		if err := tx.QueryRow(ctx, `select amount_minor, tries, state from sales.instalment_parts where plan_id = $1 and n = $2 for update`, p.ID, n).Scan(&amount, &tries, &state); err != nil {
			return err
		}
		if state != "scheduled" && state != "overdue" {
			return errors.New("payments: part is not due")
		}
		var provider, owner, currency, orderBuyer string
		if err := tx.QueryRow(ctx, `select m.provider, m.token, m.owner_id, o.currency, o.buyer_id from sales.saved_payment_methods m
			join sales.orders o on o.id = $2 where m.id = $1 and m.deleted_at is null`, *p.SavedMethodID, p.OrderID).Scan(&provider, &token, &owner, &currency, &orderBuyer); err != nil {
			return err
		}
		tries++
		part := n
		a = &Attempt{ID: uuid.NewString(), OrganiserID: p.OrganiserID, Purpose: "instalment", OrderID: &p.OrderID, Part: &part, BuyerID: owner, Provider: provider,
			ProviderKey: "inst:" + p.ID + ":" + strconv.Itoa(n) + ":" + strconv.Itoa(tries), Method: SavedCard, SavedMethodID: p.SavedMethodID,
			AmountMinor: amount, Currency: currency, DeadlineAt: s.now().Add(checkout.AttemptLimit), CreatedAt: s.now()}
		if _, err := tx.Exec(ctx, `update sales.instalment_parts set state = 'charging', tries = $3, attempt_id = $4 where plan_id = $1 and n = $2`, p.ID, n, tries, a.ID); err != nil {
			return err
		}
		return insertAttempt(ctx, tx, a)
	})
	if err != nil {
		return err
	}
	_, err = s.submit(ctx, a, ChargeRequest{AmountMinor: a.AmountMinor, Currency: a.Currency, Method: SavedCard, Token: token, CustomerRef: a.BuyerID,
		Merchant: a.OrganiserID, Metadata: map[string]string{"organiser_id": a.OrganiserID, "attempt_id": a.ID, "order_id": p.OrderID, "purpose": "instalment"}})
	if errors.Is(err, errFailover) || isProblem(err) {
		// Not charged: the part is overdue (retried with "Pay now").
		return db.WithTx(ctx, s.Pool, db.RoleCore, a.OrganiserID, func(tx pgx.Tx) error { return s.partResult(ctx, tx, a, false) })
	}
	return err
}

func isProblem(err error) bool {
	var p *apperr.Problem
	return errors.As(err, &p)
}

func (s *Service) partResult(ctx context.Context, tx pgx.Tx, a *Attempt, ok bool) error {
	var planID string
	if err := tx.QueryRow(ctx, `select id::text from sales.instalment_plans where order_id = $1`, *a.OrderID).Scan(&planID); err != nil {
		return err
	}
	return checkout.PartResultTx(ctx, tx, a.OrganiserID, planID, *a.Part, ok, s.Checkout.Today())
}

// PayOverdue retries an overdue part with the plan's saved card ("Pay now").
func (s *Service) PayOverdue(ctx context.Context, org, orderID string) (*checkout.Plan, error) {
	p, err := s.Checkout.PlanOf(ctx, org, orderID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperr.New(http.StatusConflict, apperr.OrderState, "This order has no instalment plan.", false)
	}
	for _, pt := range p.Parts {
		if pt.State == "overdue" {
			if err := s.ChargePart(ctx, p, pt.N); err != nil {
				return nil, err
			}
			return s.Checkout.PlanOf(ctx, org, orderID)
		}
	}
	return nil, apperr.New(http.StatusConflict, apperr.OrderState, "Nothing is overdue.", false)
}

// StartTopup charges a wallet top-up (the wallet credits it when the charge succeeds).
func (s *Service) StartTopup(ctx context.Context, b checkout.Buyer, org, accountID, currency, locale string, amount int64, req PayRequest) (*PayResult, error) {
	switch req.Method {
	case HostedCard, ApplePay, GooglePay:
	default:
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "Choose a payment method.", false)
	}
	var prev *string
	for _, pv := range []string{s.preferred(), s.other(s.preferred())} {
		if _, ok := s.Providers[pv]; !ok || !s.Breaker.Allow(ctx, org, pv) {
			continue
		}
		a := &Attempt{ID: uuid.NewString(), OrganiserID: org, Purpose: "topup", WalletAccountID: &accountID, BuyerID: b.ID, Provider: pv, Method: req.Method,
			AmountMinor: amount, Currency: currency, FailoverOf: prev, DeadlineAt: s.now().Add(checkout.AttemptLimit), CreatedAt: s.now()}
		a.ProviderKey = a.ID
		if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			if prev != nil {
				if err := emitPayment(ctx, tx, org, "", "payment.failover", map[string]any{"from": s.other(pv), "to": pv, "reason": "provider_unavailable", "attemptId": a.ID, "failoverOf": *prev, "purpose": "topup"}); err != nil {
					return err
				}
			}
			return insertAttempt(ctx, tx, a)
		}); err != nil {
			return nil, err
		}
		done, err := s.submit(ctx, a, ChargeRequest{AmountMinor: amount, Currency: currency, Method: req.Method, CustomerRef: b.ID, ReturnURL: req.ReturnURL,
			Merchant: org, Locale: locale, Metadata: map[string]string{"organiser_id": org, "attempt_id": a.ID, "purpose": "topup"}})
		if err == nil {
			return s.result(done), nil
		}
		if !errors.Is(err, errFailover) {
			return nil, err
		}
		prev = &a.ID
	}
	return nil, busy()
}
