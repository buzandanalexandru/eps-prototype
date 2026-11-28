package tickets

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Person is whose sales data an erasure removes: the account and its verified email (guest orders
// placed with that email are theirs too).
type Person struct {
	Sub   string
	Email string
}

// Erased counts what an erasure changed.
type Erased struct {
	Organisers   int `json:"organisers"`
	Orders       int `json:"orders"`
	Tickets      int `json:"tickets"`
	Wallets      int `json:"wallets"`
	SavedCards   int `json:"savedCards"`
	QueueEntries int `json:"queueEntries"`
}

// Erase deletes a person's personal data from the sales schema (GDPR; fan-account capability),
// keeping what the law requires for money in anonymised form: orders keep their lines, totals and
// states, payment attempts and the wallet ledger keep every amount, but no row names the person any
// more. Orders lose the buyer's id, name, email, answers and cap identity; their cap-ledger rows go;
// open instalment payments are cancelled; tickets the person holds are cancelled (the door feed
// revokes them and drops the name) and lose the holder; wallets lose their owner and stand code;
// saved cards are removed; waiting-room audit rows (a keyed hash of sale and subject) are deleted.
// Refused while the person has an order in progress (held or paying).
func (s *Service) Erase(ctx context.Context, p Person) (*Erased, error) {
	if p.Sub == "" || strings.HasPrefix(p.Sub, "erased:") {
		return nil, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "Nothing to erase.", false)
	}
	email := strings.ToLower(strings.TrimSpace(p.Email))
	var orgs []string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text from sales.person_organisers($1, $2)", p.Sub, email)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o string
			if err := rows.Scan(&o); err != nil {
				return err
			}
			orgs = append(orgs, o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	// Nothing changes while an order is in progress anywhere.
	for _, org := range orgs {
		var live int
		if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select count(*) from sales.orders where state in ('held', 'paying', 'payment_failed')
				and (buyer_id = $1 or ($2 <> '' and buyer_kind = 'guest' and buyer_email = $2))`, p.Sub, email).Scan(&live)
		}); err != nil {
			return nil, err
		}
		if live > 0 {
			return nil, apperr.New(http.StatusConflict, CodeLiveOrder, "Finish or cancel your order in progress first.", true)
		}
	}
	out := &Erased{Organisers: len(orgs)}
	subs := map[string]bool{p.Sub: true}
	for _, org := range orgs {
		if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			return s.eraseIn(ctx, tx, org, p.Sub, email, subs, out)
		}); err != nil {
			return nil, err
		}
	}
	// Waiting-room audit rows are stored under a keyed hash of sale and subject.
	if len(s.IdentitySecret) > 0 {
		type sale struct{ org, id string }
		var sales []sale
		if err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, "select organiser_id::text, sale_id::text from sales.recent_sales(5000)")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var x sale
				if err := rows.Scan(&x.org, &x.id); err != nil {
					return err
				}
				sales = append(sales, x)
			}
			return rows.Err()
		}); err != nil {
			return nil, err
		}
		byOrg := map[string][]string{}
		for _, x := range sales {
			for sub := range subs {
				byOrg[x.org] = append(byOrg[x.org], queue.IdentityHash(s.IdentitySecret, x.id, sub))
			}
		}
		for org, hashes := range byOrg {
			if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, "delete from sales.queue_entries where identity_hash = any($1)", hashes)
				out.QueueEntries += int(tag.RowsAffected())
				return err
			}); err != nil {
				return nil, err
			}
		}
	}
	if s.Log != nil {
		s.Log.Info("personal sales data erased", "organisers", out.Organisers, "orders", out.Orders, "tickets", out.Tickets, "wallets", out.Wallets, "savedCards", out.SavedCards, "queueEntries", out.QueueEntries)
	}
	return out, nil
}

func (s *Service) eraseIn(ctx context.Context, tx pgx.Tx, org, sub, email string, subs map[string]bool, out *Erased) error {
	// The person's orders: bought with the account, or as a guest with their email.
	rows, err := tx.Query(ctx, `select id::text, buyer_id from sales.orders where buyer_id = $1 or ($2 <> '' and buyer_kind = 'guest' and buyer_email = $2)`, sub, email)
	if err != nil {
		return err
	}
	var orders []string
	for rows.Next() {
		var id, buyer string
		if err := rows.Scan(&id, &buyer); err != nil {
			rows.Close()
			return err
		}
		orders = append(orders, id)
		subs[buyer] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	ids := make([]string, 0, len(subs))
	for k := range subs {
		ids = append(ids, k)
	}
	// Tickets they hold: cancelled through the feed (the door drops the name), then without holder.
	list, err := queryTickets(ctx, tx, "holder_id = any($1) and state <> 'cancelled'", ids)
	if err != nil {
		return err
	}
	for _, t := range list {
		if _, err := ReissueTx(ctx, tx, t.ID, t.Generation, Change{Reason: "erased", Cancel: true, ClearName: true}); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `update sales.tickets set holder_kind = 'erased', holder_id = 'erased:' || id::text, prev_holder_id = null, holder_name = null, updated_at = now()
		where holder_id = any($1) or prev_holder_id = any($1)`, ids)
	if err != nil {
		return err
	}
	out.Tickets += int(tag.RowsAffected())
	if len(orders) > 0 {
		if _, err := tx.Exec(ctx, `update sales.instalment_parts set state = 'cancelled' where state in ('scheduled', 'overdue')
			and plan_id in (select id from sales.instalment_plans where order_id = any($1::uuid[]) and state in ('pending', 'active'))`, orders); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update sales.instalment_plans set state = 'cancelled' where order_id = any($1::uuid[]) and state in ('pending', 'active')`, orders); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update sales.payment_attempts set buyer_id = 'erased:' || order_id::text where order_id = any($1::uuid[])`, orders); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `delete from sales.cap_ledger where order_id = any($1::uuid[])`, orders); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `update sales.orders set buyer_id = 'erased:' || id::text, buyer_email = null, buyer_name = null, answers = '{}'::jsonb,
			cap_identity = null, updated_at = now() where id = any($1::uuid[])`, orders)
		if err != nil {
			return err
		}
		out.Orders += int(tag.RowsAffected())
	}
	if _, err := tx.Exec(ctx, `update sales.payment_attempts set buyer_id = 'erased:' || id::text where purpose = 'topup' and buyer_id = any($1)`, ids); err != nil {
		return err
	}
	tag, err = tx.Exec(ctx, `update sales.saved_payment_methods set owner_id = 'erased:' || id::text, label = 'Removed card', deleted_at = coalesce(deleted_at, now()) where owner_id = any($1)`, ids)
	if err != nil {
		return err
	}
	out.SavedCards += int(tag.RowsAffected())
	tag, err = tx.Exec(ctx, `update sales.wallet_accounts set owner = 'erased:' || id::text, code = null where owner = any($1) and kind in ('fan_balance', 'fan_season_credit')`, ids)
	if err != nil {
		return err
	}
	out.Wallets += int(tag.RowsAffected())
	return nil
}
