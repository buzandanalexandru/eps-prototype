package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Question is an organiser's checkout question or consent (D-13), in the slot the selling shop's
// published storefront put it.
type Question struct {
	ID       string          `json:"id"`
	Slot     string          `json:"slot"` // afterContact | consents
	Kind     string          `json:"kind"` // text | choice | consent
	Label    snapshot.L10n   `json:"label"`
	Options  []snapshot.L10n `json:"options"`
	Required bool            `json:"required"`
}

// MaxQuestions is how many questions and consents a checkout asks (D9).
const MaxQuestions = 5

// Questions are the live version's questions (review D-a), in slot order; the order's own
// version when the event went off sale.
func (s *Service) Questions(ctx context.Context, o *Order) ([]Question, error) {
	snap, err := s.Snaps.Load(ctx, o.EventID)
	if errors.Is(err, snapshot.ErrNotFound) {
		snap, err = s.Snaps.LoadVersion(ctx, o.EventID, o.SnapshotVersion)
	}
	if err != nil {
		return nil, err
	}
	return s.QuestionsOf(ctx, snap, o.OrganiserID)
}

// QuestionsOf reads a snapshot's questions.
func (s *Service) QuestionsOf(ctx context.Context, snap *snapshot.Snapshot, org string) ([]Question, error) {
	var out []Question
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		out, err = QuestionsTx(ctx, tx, snap)
		return err
	})
	return out, err
}

// QuestionsTx reads a snapshot's questions inside a transaction of its organiser.
func QuestionsTx(ctx context.Context, tx pgx.Tx, snap *snapshot.Snapshot) ([]Question, error) {
	type slotted struct{ id, slot string }
	var ids []slotted
	for _, id := range snap.Rules.CheckoutQuestions.AfterContact {
		ids = append(ids, slotted{id, "afterContact"})
	}
	for _, id := range snap.Rules.CheckoutQuestions.Consents {
		ids = append(ids, slotted{id, "consents"})
	}
	if len(ids) == 0 {
		return []Question{}, nil
	}
	byID := map[string]Question{}
	err := func() error {
		list := make([]string, len(ids))
		for i, x := range ids {
			list[i] = x.id
		}
		rows, err := tx.Query(ctx, `select id, kind, label, options, required from control.checkout_questions where brand_id = $1 and id = any($2)`, snap.BrandID, list)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var q Question
			var label, opts []byte
			if err := rows.Scan(&q.ID, &q.Kind, &label, &opts, &q.Required); err != nil {
				return err
			}
			_ = json.Unmarshal(label, &q.Label)
			_ = json.Unmarshal(opts, &q.Options)
			byID[q.ID] = q
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}
	out := []Question{}
	for _, x := range ids {
		if q, ok := byID[x.id]; ok && len(out) < MaxQuestions {
			q.Slot = x.slot
			if q.Options == nil {
				q.Options = []snapshot.L10n{}
			}
			out = append(out, q)
		}
	}
	return out, nil
}

// Details are the fan's contact details and answers.
type Details struct {
	Name    string         `json:"name"`
	Email   string         `json:"email"`
	Answers map[string]any `json:"answers"`
}

// checkAnswers validates answers against the questions; it returns the cleaned answers and the
// ids of required questions left unanswered.
func checkAnswers(qs []Question, in map[string]any) (map[string]any, []string, error) {
	out := map[string]any{}
	var missing []string
	for _, q := range qs {
		v, ok := in[q.ID]
		switch q.Kind {
		case "text":
			sv, _ := v.(string)
			sv = strings.TrimSpace(sv)
			if len(sv) > 500 {
				return nil, nil, fmt.Errorf("answer to %s is too long", q.ID)
			}
			if sv != "" {
				out[q.ID] = sv
			} else if q.Required {
				missing = append(missing, q.ID)
			}
		case "choice":
			f, isNum := v.(float64)
			if ok && isNum && f == float64(int(f)) && int(f) >= 0 && int(f) < len(q.Options) {
				out[q.ID] = int(f)
			} else if ok && v != nil {
				return nil, nil, fmt.Errorf("answer to %s is not one of its options", q.ID)
			} else if q.Required {
				missing = append(missing, q.ID)
			}
		case "consent":
			b, _ := v.(bool)
			if b {
				out[q.ID] = true
			} else if q.Required {
				missing = append(missing, q.ID)
			}
		}
	}
	return out, missing, nil
}

// SetDetails stores the contact details and answers of a held or payment-failed order. Missing
// required answers are refused with EPS-DETAILS-MISSING naming them, so payment cannot start.
func (s *Service) SetDetails(ctx context.Context, o *Order, d Details) (*Order, error) {
	if o.State != Held && o.State != PaymentFailed {
		return nil, stateProblem(o.State)
	}
	name := strings.TrimSpace(d.Name)
	email := NormaliseEmail(d.Email)
	if name == "" || len(name) > 200 {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.DetailsMissing, "Your name is required.", false).With("missing", []string{"name"})
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 200 {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.DetailsMissing, "A valid email address is required.", false).With("missing", []string{"email"})
	}
	qs, err := s.Questions(ctx, o)
	if err != nil {
		return nil, err
	}
	answers, missing, err := checkAnswers(qs, d.Answers)
	if err != nil {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.DetailsMissing, err.Error(), false)
	}
	if len(missing) > 0 {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.DetailsMissing, "Answer the required questions.", false).With("missing", missing)
	}
	aj, _ := json.Marshal(answers)
	// Step 5 (D-7): a guest's cap moves to the normalised email it gives, checked again there.
	identity := o.CapIdentity
	if s.Gate != nil && o.BuyerKind == "guest" && o.CapIdentity != nil {
		id, err := s.Gate.BeforeDetails(ctx, o, email)
		if err != nil {
			return nil, err
		}
		if id != "" {
			identity = &id
		}
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `update sales.orders set buyer_name = $2, buyer_email = case when buyer_kind = 'guest' or buyer_email is null then $3 else buyer_email end,
			answers = $4, details_given = true, cap_identity = $5, updated_at = now() where id = $1 and state in ('held', 'payment_failed')`, o.ID, name, email, aj, identity)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return stateProblem(o.State)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetIn(ctx, o.OrganiserID, o.ID)
}

// ReadyToPay checks that payment may start: details given and every required answer present.
func (s *Service) ReadyToPay(ctx context.Context, o *Order) error {
	if !o.DetailsGiven {
		return apperr.New(http.StatusUnprocessableEntity, apperr.DetailsMissing, "Give your contact details first.", false).With("missing", []string{"name", "email"})
	}
	qs, err := s.Questions(ctx, o)
	if err != nil {
		return err
	}
	if _, missing, _ := checkAnswers(qs, o.Answers); len(missing) > 0 {
		return apperr.New(http.StatusUnprocessableEntity, apperr.DetailsMissing, "Answer the required questions.", false).With("missing", missing)
	}
	return nil
}
