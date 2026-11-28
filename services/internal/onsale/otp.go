package onsale

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

func (g *Gate) record(ctx context.Context, o *checkout.Order, outcome string) {
	_ = db.WithTx(ctx, g.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into sales.otp_challenges (organiser_id, order_id, outcome) values ($1, $2, $3)`, o.OrganiserID, o.ID, outcome)
		return err
	})
}

func otpState(o *checkout.Order) error {
	if o.State != checkout.Held && o.State != checkout.PaymentFailed {
		return apperr.New(http.StatusConflict, apperr.OrderState, "This order cannot take a code now.", false).With("state", o.State)
	}
	return nil
}

// SendOTP sends (simulates) a code for an order that needs one: the first code or a resend.
func (g *Gate) SendOTP(ctx context.Context, o *checkout.Order) (*botdefence.Challenge, error) {
	if err := otpState(o); err != nil {
		return nil, err
	}
	if !g.OTPNeeded(ctx, o) {
		return nil, apperr.New(http.StatusConflict, apperr.OrderState, "This order needs no code.", false)
	}
	c, err := g.OTP.Send(ctx, o.OrganiserID, o.EventID, o.ID)
	if errors.Is(err, botdefence.ErrNoResends) {
		return nil, apperr.New(http.StatusConflict, apperr.OTPResends, "No more codes can be sent for this order.", false)
	}
	if err != nil {
		return nil, err
	}
	g.record(ctx, o, "sent")
	return c, nil
}

// VerifyOTP checks a code (D-8). When every try of every code is used, the order is cancelled, its
// stock released and the buyer's score raised.
func (g *Gate) VerifyOTP(ctx context.Context, o *checkout.Order, code string) error {
	if err := otpState(o); err != nil {
		return err
	}
	if !sixDigits.MatchString(code) {
		code = "x" // never matches; still counts as a try
	}
	r, err := g.OTP.Verify(ctx, o.OrganiserID, o.EventID, o.ID, code)
	if err != nil {
		return err
	}
	switch r.Outcome {
	case botdefence.OTPVerified:
		g.record(ctx, o, "verified")
		return db.WithTx(ctx, g.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `update sales.orders set otp_verified_at = coalesce(otp_verified_at, now()), updated_at = now() where id = $1`, o.ID)
			return err
		})
	case botdefence.OTPExpired:
		g.record(ctx, o, "expired")
		return apperr.New(http.StatusUnprocessableEntity, apperr.OTPExpired, "This code has expired. Ask for a new one.", false).With("resendsLeft", r.ResendsLeft)
	case botdefence.OTPLocked:
		g.record(ctx, o, "locked")
		if g.Checkout != nil {
			if _, err := g.Checkout.Cancel(ctx, o); err != nil {
				return err
			}
		}
		if sale, _ := g.saleOf(ctx, o); sale != nil && g.Scorer != nil && o.PassQID != nil {
			if k, err := queue.KeysOf(o.OrganiserID, o.EventID, sale.ID); err == nil {
				_, _ = g.Scorer.Add(ctx, k.Bot(*o.PassQID), botdefence.OTPExhausted)
			}
		}
		return apperr.New(http.StatusConflict, apperr.OTPLocked, "Too many wrong codes: the order was cancelled and the tickets released.", false)
	}
	g.record(ctx, o, "wrong")
	return apperr.New(http.StatusUnprocessableEntity, apperr.OTPWrong, "That code is not right.", false).With("triesLeft", r.TriesLeft).With("resendsLeft", r.ResendsLeft)
}
