// Package snapshot reads the immutable sale snapshots the control plane compiles at publish time
// (P1, design D3.2 to D3.4): the Go mirror of apps/web/src/lib/snapshot/types.ts, a loader that
// reads only published snapshots through control.live_snapshot(), caches them in Redis and in
// process and checks their content hash, and the public availability answer per event.
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is the snapshot document version this package reads.
const SchemaVersion = 1

// L10n is fan-facing text in each shop language.
type L10n struct {
	RO string `json:"ro"`
	EN string `json:"en"`
}

// PriceStep is one rung of a product's price ladder with its precomputed amounts.
type PriceStep struct {
	Index             int     `json:"index"`
	Mode              string  `json:"mode"` // by_date | by_sales
	FromAt            *string `json:"fromAt"`
	AfterSold         *int64  `json:"afterSold"`
	FaceMinor         int64   `json:"faceMinor"`
	AllInMinor        int64   `json:"allInMinor"`
	FanFeeMinor       int64   `json:"fanFeeMinor"`
	OrganiserNetMinor int64   `json:"organiserNetMinor"`
}

// Instalments are a product's instalment settings (amounts derive from the all-in price).
type Instalments struct {
	DepositMinor *int64 `json:"depositMinor"`
	DepositBps   *int64 `json:"depositBps"`
	MonthlyCount int    `json:"monthlyCount"`
	DayOfMonth   int    `json:"dayOfMonth"`
	LastDueBy    string `json:"lastDueBy"`
}

// Product is a sellable product as published.
type Product struct {
	ProductID   string          `json:"productId"`
	Type        string          `json:"type"`
	Name        L10n            `json:"name"`
	Perks       []L10n          `json:"perks"`
	Zone        *string         `json:"zone"`
	Config      json.RawMessage `json:"config"`
	Named       bool            `json:"named"`
	SalesFrom   *string         `json:"salesFrom"`
	SalesUntil  *string         `json:"salesUntil"`
	Visible     bool            `json:"visible"`
	MaxPerOrder int             `json:"maxPerOrder"`
	SortOrder   int             `json:"sortOrder"`
	PoolIDs     []string        `json:"poolIds"`
	Requires    []string        `json:"requires"`
	PriceSteps  []PriceStep     `json:"priceSteps"`
	Instalments *Instalments    `json:"instalments"`
	// Step 7 (optional; absent in snapshots published before it): the personalisation and online
	// check-in deadlines and the late check-in fee with its all-in amounts.
	PersonaliseBy *string  `json:"personaliseBy,omitempty"`
	CheckinBy     *string  `json:"checkinBy,omitempty"`
	LateCheckin   *LateFee `json:"lateCheckin,omitempty"`
}

// LateFee is a product's late check-in fee, priced like a price step.
type LateFee struct {
	FaceMinor         int64 `json:"faceMinor"`
	AllInMinor        int64 `json:"allInMinor"`
	FanFeeMinor       int64 `json:"fanFeeMinor"`
	OrganiserNetMinor int64 `json:"organiserNetMinor"`
}

// PoolInstance is one capacity counter (event, day or slot).
type PoolInstance struct {
	InstanceID string  `json:"instanceId"`
	Date       *string `json:"date"`
	SlotStart  *string `json:"slotStart"`
	Capacity   int64   `json:"capacity"`
}

// Pool is a capacity pool expanded into its instances.
type Pool struct {
	PoolID    string         `json:"poolId"`
	Name      string         `json:"name"`
	Scope     string         `json:"scope"`
	Instances []PoolInstance `json:"instances"`
}

// Venue is where the event takes place.
type Venue struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Lat     *float64 `json:"lat"`
	Lng     *float64 `json:"lng"`
}

// Event holds what the shop shows about the event.
type Event struct {
	Slug        string `json:"slug"`
	Name        L10n   `json:"name"`
	Description L10n   `json:"description"`
	StartsAt    string `json:"startsAt"`
	EndsAt      string `json:"endsAt"`
	Timezone    string `json:"timezone"`
	Venue       *Venue `json:"venue"`
	Days        []struct {
		Date   string `json:"date"`
		Opens  string `json:"opens"`
		Closes string `json:"closes"`
	} `json:"days"`
	Lineup []struct {
		Date   string `json:"date"`
		Artist string `json:"artist"`
		Stage  string `json:"stage"`
		Starts string `json:"starts"`
		Sample bool   `json:"sample"`
	} `json:"lineup"`
}

// Rules are the event's buying rules.
type Rules struct {
	MinAge          *int `json:"minAge"`
	ConsentUnderAge *int `json:"consentUnderAge"`
	MaxPerAccount   *int `json:"maxPerAccount"`
	// HoldMinutes is how long a checkout hold lasts (5 to 15; step 4).
	HoldMinutes int `json:"holdMinutes"`
	// CheckoutQuestions are the shop's checkout slots at publish (step 4 asks them).
	CheckoutQuestions struct {
		AfterContact []string `json:"afterContact"`
		Consents     []string `json:"consents"`
	} `json:"checkoutQuestions"`
}

// Fee is the fee policy and platform fee the prices were computed with.
type Fee struct {
	Policy     string `json:"policy"`
	Bps        int64  `json:"bps"`
	FixedMinor int64  `json:"fixedMinor"`
}

// Snapshot is a sale snapshot document (schemaVersion 1).
type Snapshot struct {
	SchemaVersion int    `json:"schemaVersion"`
	SnapshotID    string `json:"snapshotId"`
	OrganiserID   string `json:"organiserId"`
	BrandID       string `json:"brandId"`
	EventID       string `json:"eventId"`
	Version       int    `json:"version"`
	// Rehearsal is set only on the core's copy for a rehearsal event (step 6, RehearsalSource).
	Rehearsal   bool      `json:"rehearsal,omitempty"`
	PublishedAt string    `json:"publishedAt"`
	Currency    string    `json:"currency"`
	Timezone    string    `json:"timezone"`
	IsSample    bool      `json:"isSample"`
	Fee         Fee       `json:"fee"`
	Event       Event     `json:"event"`
	Rules       Rules     `json:"rules"`
	Pools       []Pool    `json:"pools"`
	Products    []Product `json:"products"`
}

// Parse decodes a snapshot document and checks the invariants the core relies on: the schema
// version, ids present and every price step's all-in = face + fan fee.
func Parse(doc []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(doc, &s); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	if err := s.Check(); err != nil {
		return nil, err
	}
	return &s, nil
}

// ErrInvalid is returned for a document that breaks an invariant.
var ErrInvalid = errors.New("snapshot: invalid document")

// Check verifies the invariants of a decoded snapshot.
func (s *Snapshot) Check() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schemaVersion %d", ErrInvalid, s.SchemaVersion)
	}
	if s.SnapshotID == "" || s.OrganiserID == "" || s.EventID == "" || s.Version < 1 {
		return fmt.Errorf("%w: missing ids or version", ErrInvalid)
	}
	for _, p := range s.Products {
		if len(p.PriceSteps) == 0 {
			return fmt.Errorf("%w: product %s has no price", ErrInvalid, p.ProductID)
		}
		for _, st := range p.PriceSteps {
			if st.AllInMinor != st.FaceMinor+st.FanFeeMinor {
				return fmt.Errorf("%w: product %s step %d: all-in %d is not face %d + fan fee %d", ErrInvalid, p.ProductID, st.Index, st.AllInMinor, st.FaceMinor, st.FanFeeMinor)
			}
		}
	}
	return nil
}

// Product returns the product with the id, or nil.
func (s *Snapshot) Product(id string) *Product {
	for i := range s.Products {
		if s.Products[i].ProductID == id {
			return &s.Products[i]
		}
	}
	return nil
}

func parseTime(v *string) (time.Time, bool) {
	if v == nil || *v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, *v)
	return t, err == nil
}
