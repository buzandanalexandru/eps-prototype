package sales

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// MaxLiveRehearsals bounds the rehearsals an organiser keeps at once (each holds its own stock and
// queue in Redis until it is cleaned up).
const MaxLiveRehearsals = 3

// RehearsalHorizon is how far ahead a new rehearsal opens: nobody can join it until a run moves its
// opening (design D-1).
const RehearsalHorizon = 360 * 24 * time.Hour

// CreateRehearsal copies a sale moment into a rehearsal under a fresh event id (design D-1): the same
// settings and the source event's live snapshot version (given by the caller, who read it), opening
// far ahead. A rehearsal of a rehearsal is refused.
func (s *Service) CreateRehearsal(ctx context.Context, org, sourceSaleID, by string, version int) (*Sale, error) {
	src, _, err := s.Get(ctx, org, sourceSaleID)
	if err != nil {
		return nil, err
	}
	if src.Rehearsal != nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "A rehearsal is made from a real sale moment.", false).With("field", "sale")
	}
	if version < 1 {
		return nil, apperr.New(http.StatusNotFound, apperr.SaleNotFound, "Publish the event before rehearsing its sale.", false)
	}
	return s.insertRehearsal(ctx, org, src, by, version, nil)
}

// insertRehearsal writes the rehearsal row and its first settings version; replacing (a reset)
// ends that rehearsal in the same transaction.
func (s *Service) insertRehearsal(ctx context.Context, org string, src *Sale, by string, version int, replacing *Sale) (*Sale, error) {
	st := src.Settings
	st.OpensAt = s.now().Add(RehearsalHorizon).UTC().Truncate(time.Second)
	st.Paused = false
	if p := st.ValidateAt(s.now()); p != nil {
		return nil, p
	}
	id, event := uuid.NewString(), uuid.NewString()
	name := "Rehearsal: " + src.Name
	if len(name) > 80 {
		name = name[:80]
	}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		if replacing != nil {
			tag, err := tx.Exec(ctx, `update sales.sales set deleted_at = now() where id = $1 and rehearsal_of_event is not null and deleted_at is null`, replacing.ID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return notFound()
			}
		}
		// The organiser's rehearsals are counted under a lock on its source sale (two creates at once).
		if _, err := tx.Exec(ctx, `select 1 from sales.sales where id = $1 for update`, src.ID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `select count(*) from sales.sales where rehearsal_of_event is not null and deleted_at is null`).Scan(&n); err != nil {
			return err
		}
		if n >= MaxLiveRehearsals {
			return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "An organiser keeps at most 3 rehearsals. Reset or delete one first.", false).
				With("field", "rehearsals").With("max", MaxLiveRehearsals)
		}
		if _, err := tx.Exec(ctx, `insert into sales.sales (id, organiser_id, event_id, name, created_by, rehearsal_of_event, rehearsal_of_sale, rehearsal_version)
			values ($1, $2, $3, $4, $5, $6, $7, $8)`, id, org, event, name, by, src.EventID, src.ID, version); err != nil {
			return err
		}
		if err := insertVersion(ctx, tx, org, id, 1, st, by); err != nil {
			return err
		}
		return writeEvent(ctx, tx, org, event, id, "sale.rehearsal_created", 1)
	})
	if err != nil {
		return nil, err
	}
	if replacing != nil {
		// The ended rehearsal leaves the Redis copies (its event lists no sale any more).
		s.Refresh(ctx, org, replacing.EventID)
		if s.Redis != nil {
			_ = s.Redis.Del(ctx, dirKey(replacing.ID)).Err()
		}
	}
	s.Refresh(ctx, org, event)
	out, _, err := s.Get(ctx, org, id)
	return out, err
}

// ResetRehearsal replaces a rehearsal with a fresh one from the same source, at the source event's
// current live version: full stock and an empty queue under a new event id. The caller checks that
// no run is active.
func (s *Service) ResetRehearsal(ctx context.Context, org, rehearsalID, by string, version int) (*Sale, error) {
	cur, _, err := s.Get(ctx, org, rehearsalID)
	if err != nil {
		return nil, err
	}
	if cur.Rehearsal == nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "Only a rehearsal can be reset.", false).With("field", "sale")
	}
	src, _, err := s.Get(ctx, org, cur.Rehearsal.SourceSaleID)
	if err != nil {
		return nil, err
	}
	if version < 1 {
		return nil, apperr.New(http.StatusNotFound, apperr.SaleNotFound, "Publish the event before rehearsing its sale.", false)
	}
	// The fresh copy keeps the rehearsal's own settings (an organiser may have turned its dial).
	src.Settings = cur.Settings
	src.Name = strings.TrimPrefix(cur.Name, "Rehearsal: ")
	return s.insertRehearsal(ctx, org, src, by, version, cur)
}

// Rehearsals lists the live rehearsals of a sale moment, newest first.
func (s *Service) Rehearsals(ctx context.Context, org, sourceSaleID string) ([]Sale, error) {
	if _, err := uuid.Parse(sourceSaleID); err != nil {
		return nil, notFound()
	}
	var out []Sale
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select `+cols+` from sales.sales s join sales.sale_settings v on v.organiser_id = s.organiser_id and v.sale_id = s.id and v.version = s.settings_version
			where s.rehearsal_of_sale = $1 and s.deleted_at is null order by s.created_at desc`, sourceSaleID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			x, err := scanSale(rows)
			if err != nil {
				return err
			}
			out = append(out, x)
		}
		return rows.Err()
	})
	return out, err
}

// OpenRehearsal moves a rehearsal's opening to `at` with a 5-minute pre-queue (open at once), as a
// new settings version. Only a rehearsal whose queue was never used may move (a run always starts on
// a fresh one), so the structural lock of real sales does not apply.
func (s *Service) OpenRehearsal(ctx context.Context, org, saleID, by string, at time.Time) (*Sale, error) {
	cur, _, err := s.Get(ctx, org, saleID)
	if err != nil {
		return nil, err
	}
	if cur.Rehearsal == nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "Only a rehearsal's opening is moved by a run.", false).With("field", "sale")
	}
	at = at.UTC().Truncate(time.Second)
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		locked, err := scanSale(tx.QueryRow(ctx, `select `+cols+` from sales.sales s join sales.sale_settings v on v.organiser_id = s.organiser_id and v.sale_id = s.id
			and v.version = s.settings_version where s.id = $1 and s.deleted_at is null for update of s`, cur.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return err
		}
		next := locked.Settings
		next.OpensAt, next.PrequeueMinutes, next.QueueOn, next.Paused = at, 5, true, false
		if p := next.Validate(); p != nil {
			return p
		}
		v := locked.Version + 1
		if _, err := tx.Exec(ctx, `update sales.sales set settings_version = $2 where id = $1`, cur.ID, v); err != nil {
			return err
		}
		if err := insertVersion(ctx, tx, org, cur.ID, v, next, by); err != nil {
			return err
		}
		return writeEvent(ctx, tx, org, cur.EventID, cur.ID, "sale.settings_changed", v)
	})
	if err != nil {
		return nil, err
	}
	s.Refresh(ctx, org, cur.EventID)
	out, _, err := s.Get(ctx, org, cur.ID)
	return out, err
}

// SetDegraded turns degraded mode on or off (design D-9); the Redis copy carries it to the gateway
// and every core within 2 seconds.
func (s *Service) SetDegraded(ctx context.Context, org, saleID, by string, on bool) (*Sale, error) {
	cur, _, err := s.Get(ctx, org, saleID)
	if err != nil {
		return nil, err
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		// Turning it off records the window that ended (passes that ran out inside it get its length).
		if _, err := tx.Exec(ctx, `update sales.sales set degraded = $2, degraded_at = clock_timestamp(), degraded_by = $3,
			degraded_window_start = case when degraded and not $2 then degraded_at else degraded_window_start end,
			degraded_window_end = case when degraded and not $2 then clock_timestamp() else degraded_window_end end
			where id = $1 and deleted_at is null`, saleID, on, by); err != nil {
			return err
		}
		typ := "sale.degraded_off"
		if on {
			typ = "sale.degraded_on"
		}
		return writeEvent(ctx, tx, org, cur.EventID, saleID, typ, cur.Version)
	})
	if err != nil {
		return nil, err
	}
	s.Refresh(ctx, org, cur.EventID)
	out, _, err := s.Get(ctx, org, saleID)
	return out, err
}

// MarkCleaned records that a rehearsal's Redis data was deleted.
func (s *Service) MarkCleaned(ctx context.Context, org, saleID string) error {
	return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update sales.sales set rehearsal_cleaned_at = now() where id = $1 and rehearsal_of_event is not null`, saleID)
		return err
	})
}

// CloseRehearsal moves a rehearsal's opening back to the horizon and pauses it once its run ended
// (review D-c): nobody can join it, and its waiting room admits nobody, until the next run.
func (s *Service) CloseRehearsal(ctx context.Context, org, saleID, by string) error {
	cur, _, err := s.Get(ctx, org, saleID)
	if err != nil || cur.Rehearsal == nil {
		return err
	}
	if err := s.forceClose(ctx, org, cur, by); err != nil {
		return err
	}
	s.Refresh(ctx, org, cur.EventID)
	return nil
}

func (s *Service) forceClose(ctx context.Context, org string, cur *Sale, by string) error {
	return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		locked, err := scanSale(tx.QueryRow(ctx, `select `+cols+` from sales.sales s join sales.sale_settings v on v.organiser_id = s.organiser_id and v.sale_id = s.id
			and v.version = s.settings_version where s.id = $1 and s.deleted_at is null for update of s`, cur.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		next := locked.Settings
		next.OpensAt, next.Paused = s.now().Add(RehearsalHorizon).UTC().Truncate(time.Second), true
		v := locked.Version + 1
		if _, err := tx.Exec(ctx, `update sales.sales set settings_version = $2 where id = $1`, cur.ID, v); err != nil {
			return err
		}
		if err := insertVersion(ctx, tx, org, cur.ID, v, next, by); err != nil {
			return err
		}
		return writeEvent(ctx, tx, org, cur.EventID, cur.ID, "sale.settings_changed", v)
	})
}
