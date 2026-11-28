package checkout

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// NormaliseEmail is how emails are compared for claims.
func NormaliseEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// Claim moves guest orders placed with the account's verified email into the account (D-9). With
// orderID it claims that order only, and refuses one placed with another email. It returns the
// claimed order ids.
func (s *Service) Claim(ctx context.Context, b Buyer, orderID string) ([]string, error) {
	if b.Kind != "account" {
		return nil, apperr.New(http.StatusForbidden, apperr.Forbidden, "Sign in to add orders to your account.", false)
	}
	if b.Email == "" {
		return nil, apperr.New(http.StatusForbidden, apperr.ClaimRefused, "Verify your email to add orders to your account.", false)
	}
	email := NormaliseEmail(b.Email)
	type ref struct{ org, id string }
	var refs []ref
	if orderID != "" {
		org, err := s.OrganiserOf(ctx, orderID)
		if err != nil {
			return nil, err
		}
		if org == "" {
			return nil, notFound()
		}
		refs = []ref{{org, orderID}}
	} else {
		err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, "select organiser_id::text, order_id::text from sales.guest_orders_by_email($1, 200)", email)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var r ref
				if err := rows.Scan(&r.org, &r.id); err != nil {
					return err
				}
				refs = append(refs, r)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
	}
	var claimed []string
	for _, r := range refs {
		err := db.WithTx(ctx, s.Pool, db.RoleCore, r.org, func(tx pgx.Tx) error {
			o, err := LoadTx(ctx, tx, r.id, true)
			if err != nil {
				return err
			}
			if o.BuyerKind != "guest" || o.BuyerEmail == nil || *o.BuyerEmail != email {
				return apperr.New(http.StatusForbidden, apperr.ClaimRefused, "This order was placed with another email.", false)
			}
			if _, err := tx.Exec(ctx, `update sales.orders set buyer_kind = 'account', buyer_id = $2, updated_at = now() where id = $1`, r.id, b.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `insert into sales.order_history (organiser_id, order_id, from_state, to_state, reason) values ($1, $2, $3, $3, 'claimed into an account')`, r.org, r.id, o.State); err != nil {
				return err
			}
			// Saved cards of the guest (instalment plans) follow the order.
			if _, err := tx.Exec(ctx, `update sales.saved_payment_methods set owner_id = $2 where owner_id = $1`, o.BuyerID, b.ID); err != nil {
				return err
			}
			return emit(ctx, tx, o, "order.claimed", map[string]any{"from": "guest"})
		})
		var p *apperr.Problem
		var pe *pgconn.PgError
		switch {
		case err == nil:
			claimed = append(claimed, r.id)
		case errors.As(err, &p) && orderID != "":
			return nil, p
		case errors.As(err, &pe) && pe.Code == "23505":
			// The account already has a live order for that sale: leave this one with the guest.
		case errors.As(err, &p), errors.Is(err, ErrNotFound):
		default:
			return claimed, err
		}
	}
	return claimed, nil
}

// OrdersOf lists a buyer's orders across organisers, newest first.
func (s *Service) OrdersOf(ctx context.Context, buyerID string) ([]*Order, error) {
	type ref struct{ org, id string }
	var refs []ref
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, order_id::text from sales.orders_of_buyer($1, 100)", buyerID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ref
			if err := rows.Scan(&r.org, &r.id); err != nil {
				return err
			}
			refs = append(refs, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	out := make([]*Order, 0, len(refs))
	for _, r := range refs {
		if o, err := s.GetIn(ctx, r.org, r.id); err == nil && o.BuyerID == buyerID {
			out = append(out, o)
		}
	}
	return out, nil
}
