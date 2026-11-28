package wallet

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// Movement is one line of a fan's wallet history.
type Movement struct {
	Kind        string    `json:"kind"` // topup | order_payment | stand_payment | season_close | reversal
	Account     string    `json:"account"`
	AmountMinor int64     `json:"amountMinor"`
	Memo        string    `json:"memo"`
	At          time.Time `json:"at"`
}

// Wallet is a fan's wallet at one organiser.
type Wallet struct {
	OrganiserID     string     `json:"organiserId"`
	Currency        string     `json:"currency"`
	BalanceMinor    int64      `json:"balanceMinor"`
	CreditMinor     int64      `json:"creditMinor"`
	CreditExpiresAt *time.Time `json:"creditExpiresAt,omitempty"`
	Code            string     `json:"code"`
	History         []Movement `json:"history"`
}

// Wallet reads a fan's wallet at an organiser (an empty one if none exists yet).
func (s *Service) Wallet(ctx context.Context, org, owner, currency string) (*Wallet, error) {
	w := &Wallet{OrganiserID: org, Currency: currency, History: []Movement{}}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id::text, kind, balance_minor, coalesce(code, ''), expires_at from sales.wallet_accounts
			where owner = $1 and currency = $2 and kind in ('fan_balance', 'fan_season_credit')`, owner, currency)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id, kind, code string
			var bal int64
			var exp *time.Time
			if err := rows.Scan(&id, &kind, &bal, &code, &exp); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
			if kind == FanBalance {
				w.BalanceMinor, w.Code = bal, code
			} else {
				w.CreditMinor, w.CreditExpiresAt = bal, exp
			}
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		hrows, err := tx.Query(ctx, `select e.kind, a.kind, l.amount_minor, e.memo, e.created_at from sales.journal_lines l
			join sales.journal_entries e on e.organiser_id = l.organiser_id and e.id = l.entry_id
			join sales.wallet_accounts a on a.organiser_id = l.organiser_id and a.id = l.account_id
			where l.account_id = any($1::uuid[]) order by l.id desc limit 100`, ids)
		if err != nil {
			return err
		}
		defer hrows.Close()
		for hrows.Next() {
			var m Movement
			if err := hrows.Scan(&m.Kind, &m.Account, &m.AmountMinor, &m.Memo, &m.At); err != nil {
				return err
			}
			w.History = append(w.History, m)
		}
		return hrows.Err()
	})
	return w, err
}

// Wallets lists a fan's wallets across organisers.
func (s *Service) Wallets(ctx context.Context, owner string) ([]*Wallet, error) {
	var orgs []string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text from sales.wallets_of_fan($1)", owner)
		if err != nil {
			return err
		}
		orgs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, err
	}
	out := []*Wallet{}
	for _, org := range orgs {
		var currencies []string
		err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, "select distinct currency from sales.wallet_accounts where owner = $1 and kind = 'fan_balance'", owner)
			if err != nil {
				return err
			}
			currencies, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, c := range currencies {
			w, err := s.Wallet(ctx, org, owner, c)
			if err != nil {
				return nil, err
			}
			out = append(out, w)
		}
	}
	return out, nil
}

// Audit checks an organiser's ledger: every entry adds up to zero per currency and every cached
// balance equals the sum of its lines. It returns the problems found.
func (s *Service) Audit(ctx context.Context, org string) ([]string, error) {
	var out []string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select entry_id::text, currency, sum(amount_minor) from sales.journal_lines group by entry_id, currency having sum(amount_minor) <> 0`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, cur string
			var sum int64
			if err := rows.Scan(&id, &cur, &sum); err != nil {
				rows.Close()
				return err
			}
			out = append(out, fmt.Sprintf("entry %s does not add up in %s (%d)", id, cur, sum))
		}
		rows.Close()
		rows, err = tx.Query(ctx, `select a.id::text, a.kind, a.balance_minor, coalesce(sum(l.amount_minor), 0) from sales.wallet_accounts a
			left join sales.journal_lines l on l.organiser_id = a.organiser_id and l.account_id = a.id group by a.id
			having a.balance_minor <> coalesce(sum(l.amount_minor), 0)`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, kind string
			var bal, sum int64
			if err := rows.Scan(&id, &kind, &bal, &sum); err != nil {
				return err
			}
			out = append(out, fmt.Sprintf("account %s (%s) caches %d but its lines add up to %d", id, kind, bal, sum))
		}
		return rows.Err()
	})
	return out, err
}
