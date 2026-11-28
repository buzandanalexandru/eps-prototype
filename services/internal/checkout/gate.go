package checkout

import (
	"context"
	"time"
)

// Gate is the on-sale engine's check before a hold (step 5, design D-11): the sale's pass, the cap
// per order and the cap per person. Nil (step 4 alone, or an event without sale moments) means no
// pass and only the snapshot's buying rules.
type Gate interface {
	// BeforeHold runs before stock is taken for a new order and returns what the order records.
	BeforeHold(ctx context.Context, h GateHold) (*GateResult, error)
	// BeforeDetails runs when a guest gives an email: the cap moves to the email (D-7).
	BeforeDetails(ctx context.Context, o *Order, email string) (identity string, err error)
}

// GateHold is a hold the gate checks.
type GateHold struct {
	Buyer   Buyer
	Org     string
	EventID string
	OrderID string
	Tickets int
	// MaxPerAccount is the snapshot's per-person maximum (0 when unset): the stricter cap applies.
	MaxPerAccount int
	// HoldMinutes is the snapshot's hold length.
	HoldMinutes int
}

// GateResult is what the gate decided for a hold.
type GateResult struct {
	SaleID       string
	PassQID      string
	CapIdentity  string
	CapPerPerson int
	CapTickets   int
	OTPRequired  bool
	// HoldMinutes is the sale's hold length (0: the snapshot's).
	HoldMinutes int
	// Taken reserves the person's cap for the order until the deadline; Undo gives it back when the
	// hold fails after it.
	Taken func(ctx context.Context, deadline time.Time) error
	Undo  func(ctx context.Context)
}
