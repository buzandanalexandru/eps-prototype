// Package wallet is the EPS Wallet (O6, design D-11): a simulated prepaid balance per fan per
// organiser, kept as a double-entry ledger in the sales schema. Every money move is one journal
// entry whose lines add up to zero per currency (a deferred trigger checks it at commit), lines are
// never changed (corrections are reversing entries), and cached balances move in the same
// transaction with a CHECK that a fan's balance never goes below zero. In production a stored
// balance is e-money: it needs an e-money licence or a regulated partner holding the funds; the
// prototype holds no money at all.
package wallet

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/idem"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Account kinds.
const (
	FanBalance     = "fan_balance"
	FanCredit      = "fan_season_credit"
	PSPClearing    = "psp_clearing"
	OrganiserSales = "organiser_sales"
	StandSales     = "stand_sales"
)

// Top-up limits (minor units) until organisers set their own.
const (
	TopupMinMinor = 1_000
	TopupMaxMinor = 200_000
)

// Service is the wallet.
type Service struct {
	// Redis keeps the fans' short-lived pay codes for stands.
	Redis    *redis.Client
	Pool     *pgxpool.Pool
	Checkout *checkout.Service
	Payments *payments.Service
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Line is one journal line: a signed amount on an account (positive raises its balance).
type Line struct {
	AccountID   string
	AmountMinor int64
}

func newCode() string {
	const a = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 9)
	for i := range b {
		if i == 4 {
			b[i] = '-'
			continue
		}
		x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(a))))
		b[i] = a[x.Int64()]
	}
	return string(b)
}

// AccountTx returns (creating it if needed) an account's id inside tx.
func AccountTx(ctx context.Context, tx pgx.Tx, org, kind, owner, currency string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `select id::text from sales.wallet_accounts where kind = $1 and owner = $2 and currency = $3`, kind, owner, currency).Scan(&id)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	for i := 0; i < 5; i++ {
		var code *string
		if kind == FanBalance {
			c := newCode()
			code = &c
		}
		err = tx.QueryRow(ctx, `insert into sales.wallet_accounts (organiser_id, kind, owner, currency, code) values ($1, $2, $3, $4, $5)
			on conflict (organiser_id, kind, owner, currency) do update set kind = excluded.kind returning id::text`, org, kind, owner, currency, code).Scan(&id)
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" { // a code clash: draw another
			continue
		}
		return id, err
	}
	return id, err
}

// PostTx writes one journal entry with its lines. The lines must add up to zero (checked by the
// database at commit; an unbalanced entry rolls the whole transaction back).
func PostTx(ctx context.Context, tx pgx.Tx, org, kind, ref, memo, currency string, lines []Line) (string, error) {
	var id string
	if err := tx.QueryRow(ctx, `insert into sales.journal_entries (organiser_id, kind, ref, memo) values ($1, $2, $3, $4) returning id::text`, org, kind, ref, memo).Scan(&id); err != nil {
		return "", err
	}
	for _, l := range lines {
		if l.AmountMinor == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `insert into sales.journal_lines (organiser_id, entry_id, account_id, amount_minor, currency) values ($1, $2, $3, $4, $5)`,
			org, id, l.AccountID, l.AmountMinor, currency); err != nil {
			return "", err
		}
	}
	_, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicWallet, Type: "wallet." + kind, OrganiserID: org, Payload: map[string]any{"entryId": id, "ref": ref, "currency": currency}})
	return id, err
}

// ReverseTx writes the reversing entry of an entry (the correction of a ledger mistake).
func ReverseTx(ctx context.Context, tx pgx.Tx, org, entryID, memo string) (string, error) {
	rows, err := tx.Query(ctx, `select account_id::text, amount_minor, currency from sales.journal_lines where entry_id = $1`, entryID)
	if err != nil {
		return "", err
	}
	var lines []Line
	currency := ""
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.AccountID, &l.AmountMinor, &currency); err != nil {
			rows.Close()
			return "", err
		}
		l.AmountMinor = -l.AmountMinor
		lines = append(lines, l)
	}
	rows.Close()
	if len(lines) == 0 {
		return "", errors.New("wallet: no such entry")
	}
	return PostTx(ctx, tx, org, "reversal", entryID, memo, currency, lines)
}

func insufficient() *apperr.Problem {
	return apperr.New(http.StatusConflict, apperr.WalletInsufficient, "Your wallet balance does not cover this.", false)
}

func isOverspend(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23514" && pe.ConstraintName == "fan_balance_not_negative"
}

// fanAccounts locks a fan's balance and (unexpired) season credit accounts in tx.
func fanAccounts(ctx context.Context, tx pgx.Tx, org, owner, currency string, now time.Time) (balID string, bal int64, credID string, cred int64, err error) {
	if balID, err = AccountTx(ctx, tx, org, FanBalance, owner, currency); err != nil {
		return
	}
	if credID, err = AccountTx(ctx, tx, org, FanCredit, owner, currency); err != nil {
		return
	}
	var exp *time.Time
	if err = tx.QueryRow(ctx, `select balance_minor from sales.wallet_accounts where id = $1 for update`, balID).Scan(&bal); err != nil {
		return
	}
	if err = tx.QueryRow(ctx, `select balance_minor, expires_at from sales.wallet_accounts where id = $1 for update`, credID).Scan(&cred, &exp); err != nil {
		return
	}
	if exp != nil && !now.Before(*exp) {
		cred = 0 // expired credit is not spendable
	}
	return
}

// spend splits an amount over season credit first (it expires) and then the balance.
func spend(amount, cred, bal int64) (fromCred, fromBal int64, ok bool) {
	if cred+bal < amount {
		return 0, 0, false
	}
	fromCred = min(cred, amount)
	return fromCred, amount - fromCred, true
}

// PayOrder pays a held order from the fan's wallet in one transaction: ledger entry, stock commit
// and the order paid, with no provider involved (D-11). A balance that does not cover the total
// is refused with EPS-WALLET-INSUFFICIENT and the hold stays.
func (s *Service) PayOrder(ctx context.Context, b checkout.Buyer, o *checkout.Order, key string) (*checkout.Order, error) {
	if b.Kind != "account" || o.BuyerID != b.ID {
		return nil, apperr.New(http.StatusForbidden, apperr.Forbidden, "Sign in to pay from your wallet.", false)
	}
	if o.PayPlan == "instalments" {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "An instalment order cannot be paid from the wallet.", false)
	}
	if o.Rehearsal { // step 6, review D-a: no ledger postings for a rehearsal
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "A rehearsal pays by card, Apple Pay or Google Pay only, in one payment (simulated).", false).With("rehearsal", true)
	}
	if err := s.Checkout.ReadyToPay(ctx, o); err != nil {
		return nil, err
	}
	scope := "wallet-pay:" + o.ID
	if key != "" {
		replay, err := idem.Begin(ctx, s.Pool, o.OrganiserID, scope, key, idem.Hash(map[string]any{"order": o.ID, "total": o.TotalMinor}))
		switch {
		case errors.Is(err, idem.ErrMismatch):
			return nil, apperr.New(http.StatusUnprocessableEntity, apperr.IdempotencyMismatch, "This Idempotency-Key was used for another payment.", false)
		case errors.Is(err, idem.ErrInProgress):
			return nil, apperr.New(http.StatusConflict, apperr.InProgress, "The same payment is still running.", true)
		case err != nil && !errors.Is(err, idem.ErrBadKey):
			return nil, err
		}
		if replay != nil {
			return s.Checkout.GetIn(ctx, o.OrganiserID, o.ID)
		}
	}
	// Ledger entry and order paid in one transaction, then the stock (review M4). If the stock
	// cannot be committed, the entry is reversed and the order refunded (the balance comes back);
	// a crash in between is settled by the sweeper's orphan pass, which commits the stock of a paid
	// order whose hold is still held.
	var entryID string
	var paid *checkout.Order
	err := db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		balID, bal, credID, cred, err := fanAccounts(ctx, tx, o.OrganiserID, b.ID, o.Currency, s.now())
		if err != nil {
			return err
		}
		fromCred, fromBal, ok := spend(o.TotalMinor, cred, bal)
		if !ok {
			return insufficient()
		}
		salesID, err := AccountTx(ctx, tx, o.OrganiserID, OrganiserSales, o.OrganiserID, o.Currency)
		if err != nil {
			return err
		}
		if entryID, err = PostTx(ctx, tx, o.OrganiserID, "order_payment", o.ID, "order paid from wallet", o.Currency,
			[]Line{{credID, -fromCred}, {balID, -fromBal}, {salesID, o.TotalMinor}}); err != nil {
			return err
		}
		paid, err = s.Checkout.MarkPaidTx(ctx, tx, o.ID, "paid from the EPS Wallet")
		return err
	})
	if isOverspend(err) {
		err = insufficient()
	}
	if err == nil {
		if cerr := s.Checkout.CommitStock(ctx, paid, "wallet"); cerr != nil {
			var p *apperr.Problem
			if errors.As(cerr, &p) {
				// Compensation: the money goes back to the wallet and the order is refunded.
				if rerr := db.WithTx(context.WithoutCancel(ctx), s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
					if _, err := ReverseTx(ctx, tx, o.OrganiserID, entryID, "stock gone: payment returned"); err != nil {
						return err
					}
					return checkout.RefundPaidTx(ctx, tx, paid, p.Code)
				}); rerr != nil {
					return nil, rerr
				}
			}
			err = cerr
		}
	}
	if key != "" {
		var p *apperr.Problem
		switch {
		case err == nil:
			_ = idem.Finish(ctx, s.Pool, o.OrganiserID, scope, key, 200, map[string]string{"orderId": o.ID})
		case errors.As(err, &p) && !p.Retryable && p.Code != apperr.WalletInsufficient:
			_ = idem.Finish(ctx, s.Pool, o.OrganiserID, scope, key, p.Status, map[string]string{"code": p.Code})
		default:
			idem.Abort(ctx, s.Pool, o.OrganiserID, scope, key)
		}
	}
	if err != nil {
		return nil, err
	}
	return s.Checkout.GetIn(ctx, o.OrganiserID, o.ID)
}

// CreditTx credits a confirmed top-up (payments.TopupSink): clearing account of the provider to
// the fan's balance.
func (s *Service) CreditTx(ctx context.Context, tx pgx.Tx, a *payments.Attempt) error {
	var owner string
	if err := tx.QueryRow(ctx, `select owner from sales.wallet_accounts where id = $1 and kind = 'fan_balance'`, *a.WalletAccountID).Scan(&owner); err != nil {
		return err
	}
	clearing, err := AccountTx(ctx, tx, a.OrganiserID, PSPClearing, a.Provider, a.Currency)
	if err != nil {
		return err
	}
	_, err = PostTx(ctx, tx, a.OrganiserID, "topup", a.ID, "top-up through provider "+a.Provider, a.Currency,
		[]Line{{clearing, -a.AmountMinor}, {*a.WalletAccountID, a.AmountMinor}})
	return err
}

// TopupRequest is a fan's top-up.
type TopupRequest struct {
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
	Method      string `json:"method"`
	ReturnURL   string `json:"returnUrl"`
	Locale      string `json:"locale"`
}

// Topup starts a top-up through the simulated providers; the balance rises only when the provider
// confirms the charge, and a repeated request with the same key credits once.
func (s *Service) Topup(ctx context.Context, b checkout.Buyer, org string, req TopupRequest, key string) (*payments.PayResult, error) {
	if b.Kind != "account" {
		return nil, apperr.New(http.StatusForbidden, apperr.Forbidden, "Sign in to use the wallet.", false)
	}
	if req.AmountMinor < TopupMinMinor || req.AmountMinor > TopupMaxMinor || (req.Currency != "RON" && req.Currency != "EUR") {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.WalletLimit, "Top up between 10 and 2,000.", false).With("min", TopupMinMinor).With("max", TopupMaxMinor)
	}
	if err := s.CheckOrganiser(ctx, org, req.Currency); err != nil {
		return nil, err
	}
	scope := "topup:" + b.ID
	replay, err := idem.Begin(ctx, s.Pool, org, scope, key, idem.Hash(req))
	switch {
	case errors.Is(err, idem.ErrMismatch):
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.IdempotencyMismatch, "This Idempotency-Key was used for another top-up.", false)
	case errors.Is(err, idem.ErrInProgress):
		return nil, apperr.New(http.StatusConflict, apperr.InProgress, "The same top-up is still running.", true)
	case errors.Is(err, idem.ErrBadKey):
		return nil, apperr.New(http.StatusBadRequest, apperr.IdempotencyMismatch, err.Error(), false)
	case err != nil:
		return nil, err
	}
	if replay != nil {
		var st struct {
			AttemptID string `json:"attemptId"`
		}
		_ = json.Unmarshal(replay.Response, &st)
		a, err := s.Payments.GetAttempt(ctx, org, st.AttemptID)
		if err != nil {
			return nil, err
		}
		r := &payments.PayResult{Attempt: a}
		if a.ActionURL != nil && (a.State == payments.Submitted || a.State == payments.ChallengeRequired) {
			r.ActionURL = *a.ActionURL
		}
		return r, nil
	}
	var accountID string
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		accountID, err = AccountTx(ctx, tx, org, FanBalance, b.ID, req.Currency)
		if err != nil {
			return err
		}
		_, err = AccountTx(ctx, tx, org, FanCredit, b.ID, req.Currency)
		return err
	})
	if err != nil {
		idem.Abort(ctx, s.Pool, org, scope, key)
		return nil, err
	}
	res, err := s.Payments.StartTopup(ctx, b, org, accountID, req.Currency, req.Locale, req.AmountMinor, payments.PayRequest{Method: req.Method, ReturnURL: req.ReturnURL})
	if err != nil {
		idem.Abort(ctx, s.Pool, org, scope, key)
		return nil, err
	}
	_ = idem.Finish(ctx, s.Pool, org, scope, key, 200, map[string]string{"attemptId": res.Attempt.ID})
	return res, nil
}

// PayCodeLife is how long a fan's stand pay code works (single use, review D-d).
const PayCodeLife = 2 * time.Minute

func payCodeKey(org, walletCode string) string {
	k, _ := redisx.Key(org, "", "paycode", walletCode)
	return k
}

func hashCode(c string) string {
	h := sha256.Sum256([]byte("eps-pay-code:" + c))
	return hex.EncodeToString(h[:])
}

// CheckOrganiser reports whether the organiser sells in the currency (its brands' currencies);
// a wallet exists only for an organiser and currency that are real (review sec L4).
func (s *Service) CheckOrganiser(ctx context.Context, org, currency string) error {
	var n int
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select count(*) from control.brands where currency = $1`, currency).Scan(&n)
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.New(http.StatusNotFound, apperr.OrderNotFound, "No wallet for this organiser and currency.", false)
	}
	return nil
}

// PayCode gives the fan a 6-digit code for one stand payment within 2 minutes.
func (s *Service) PayCode(ctx context.Context, org, owner, currency string) (string, string, time.Time, error) {
	if err := s.CheckOrganiser(ctx, org, currency); err != nil {
		return "", "", time.Time{}, err
	}
	var walletCode string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		id, err := AccountTx(ctx, tx, org, FanBalance, owner, currency)
		if err != nil {
			return err
		}
		if _, err := AccountTx(ctx, tx, org, FanCredit, owner, currency); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "select code from sales.wallet_accounts where id = $1", id).Scan(&walletCode)
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	x, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	code := strconv.FormatInt(1_000_000+x.Int64(), 10)[1:]
	exp := s.now().Add(PayCodeLife)
	if err := s.Redis.Set(ctx, payCodeKey(org, walletCode), hashCode(code), PayCodeLife).Err(); err != nil {
		return "", "", time.Time{}, err
	}
	return walletCode, code, exp, nil
}

// KEYS: pay code. ARGV: hash. Deletes and returns 1 only on a match (single use).
var useCodeScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then redis.call('DEL', KEYS[1]); return 1 end
return 0
`)

// Stand charges a fan's wallet at an organiser's stand by the fan's wallet code and the fan's
// current pay code (simulated, review D-d). The same Idempotency-Key charges once.
func (s *Service) Stand(ctx context.Context, org, code, payCode string, amount int64, label, key string) (*Wallet, error) {
	if amount <= 0 || amount > TopupMaxMinor || len(label) > 80 {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.WalletLimit, "Charge between 0.01 and 2,000.", false)
	}
	scope := "stand:" + org
	replay, err := idem.Begin(ctx, s.Pool, org, scope, key, idem.Hash(map[string]any{"code": code, "amount": amount, "label": label}))
	switch {
	case errors.Is(err, idem.ErrMismatch):
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.IdempotencyMismatch, "This Idempotency-Key was used for another charge.", false)
	case errors.Is(err, idem.ErrInProgress):
		return nil, apperr.New(http.StatusConflict, apperr.InProgress, "The same charge is still running.", true)
	case errors.Is(err, idem.ErrBadKey):
		return nil, apperr.New(http.StatusBadRequest, apperr.IdempotencyMismatch, err.Error(), false)
	case err != nil:
		return nil, err
	}
	if replay != nil {
		var w Wallet
		_ = json.Unmarshal(replay.Response, &w)
		return &w, nil
	}
	w, err := s.stand(ctx, org, code, payCode, amount, label)
	if err != nil {
		idem.Abort(ctx, s.Pool, org, scope, key)
		return nil, err
	}
	_ = idem.Finish(ctx, s.Pool, org, scope, key, 200, w)
	return w, nil
}

func (s *Service) stand(ctx context.Context, org, code, payCode string, amount int64, label string) (*Wallet, error) {
	ok, err := useCodeScript.Run(ctx, s.Redis, []string{payCodeKey(org, code)}, hashCode(payCode)).Int()
	if err != nil {
		return nil, err
	}
	if ok != 1 {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.PayCodeInvalid, "The fan's pay code is wrong or has expired.", false)
	}
	var owner, currency string
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `select owner, currency from sales.wallet_accounts where kind = 'fan_balance' and code = $1`, code).Scan(&owner, &currency); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.New(http.StatusNotFound, apperr.OrderNotFound, "No wallet with this code.", false)
			}
			return err
		}
		balID, bal, credID, cred, err := fanAccounts(ctx, tx, org, owner, currency, s.now())
		if err != nil {
			return err
		}
		fromCred, fromBal, ok := spend(amount, cred, bal)
		if !ok {
			return insufficient()
		}
		standID, err := AccountTx(ctx, tx, org, StandSales, org, currency)
		if err != nil {
			return err
		}
		if label == "" {
			label = "stand"
		}
		_, err = PostTx(ctx, tx, org, "stand_payment", code, label, currency, []Line{{credID, -fromCred}, {balID, -fromBal}, {standID, amount}})
		return err
	})
	if isOverspend(err) {
		err = insufficient()
	}
	if err != nil {
		return nil, err
	}
	return s.Wallet(ctx, org, owner, currency)
}

// CloseSeason moves every fan's unspent balance to next-season credit expiring at `expires`.
// MinCreditLife is the least time a new season credit lasts (review D-d).
const MinCreditLife = 30 * 24 * time.Hour

func (s *Service) CloseSeason(ctx context.Context, org string, expires time.Time) (int, int64, error) {
	if expires.Before(s.now().Add(MinCreditLife)) {
		return 0, 0, apperr.New(http.StatusUnprocessableEntity, apperr.WalletLimit, "Season credit must last at least 30 days.", false)
	}
	n, total := 0, int64(0)
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		// Credit already past its expiry is written off first, with its own ledger entry, so an
		// old season's credit never comes back with the new expiry date.
		erows, err := tx.Query(ctx, `select id::text, owner, currency, balance_minor from sales.wallet_accounts
			where kind = 'fan_season_credit' and balance_minor > 0 and expires_at is not null and expires_at <= now() for update`)
		if err != nil {
			return err
		}
		type exp struct {
			id, owner, currency string
			bal                 int64
		}
		var olds []exp
		for erows.Next() {
			var e exp
			if err := erows.Scan(&e.id, &e.owner, &e.currency, &e.bal); err != nil {
				erows.Close()
				return err
			}
			olds = append(olds, e)
		}
		erows.Close()
		for _, e := range olds {
			brk, err := AccountTx(ctx, tx, org, "credit_breakage", org, e.currency)
			if err != nil {
				return err
			}
			if _, err := PostTx(ctx, tx, org, "credit_expiry", e.owner, "season credit expired", e.currency, []Line{{e.id, -e.bal}, {brk, e.bal}}); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `select id::text, owner, currency, balance_minor from sales.wallet_accounts where kind = 'fan_balance' and balance_minor > 0 for update`)
		if err != nil {
			return err
		}
		type acct struct {
			id, owner, currency string
			bal                 int64
		}
		var list []acct
		for rows.Next() {
			var a acct
			if err := rows.Scan(&a.id, &a.owner, &a.currency, &a.bal); err != nil {
				rows.Close()
				return err
			}
			list = append(list, a)
		}
		rows.Close()
		for _, a := range list {
			credID, err := AccountTx(ctx, tx, org, FanCredit, a.owner, a.currency)
			if err != nil {
				return err
			}
			if _, err := PostTx(ctx, tx, org, "season_close", a.owner, "season closed", a.currency, []Line{{a.id, -a.bal}, {credID, a.bal}}); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update sales.wallet_accounts set expires_at = $2 where id = $1`, credID, expires); err != nil {
				return err
			}
			n++
			total += a.bal
		}
		return nil
	})
	return n, total, err
}
