// Package apperr is the error every checkout, payment and wallet operation returns to a fan or an
// organiser (design D-14): an HTTP status, a quotable code (EPS-...), the key of its message in the
// shop's language files and whether trying again can help.
package apperr

import "fmt"

// Problem is a failure with a code the fan can quote to support.
type Problem struct {
	Status    int
	Code      string
	Detail    string
	Retryable bool
	// Data carries facts the screen needs (the product that sold out, the open attempt, ...).
	Data map[string]any
}

func (p *Problem) Error() string { return fmt.Sprintf("%s (%d): %s", p.Code, p.Status, p.Detail) }

// MessageKey is the shop label of the code (e.g. EPS-HOLD-SOLD-OUT → err_hold_sold_out).
func (p *Problem) MessageKey() string {
	b := []byte(p.Code)
	out := make([]byte, 0, len(b))
	for i, c := range b {
		if i < 4 { // "EPS-"
			continue
		}
		switch {
		case c == '-':
			out = append(out, '_')
		case c >= 'A' && c <= 'Z':
			out = append(out, c+'a'-'A')
		default:
			out = append(out, c)
		}
	}
	return "err_" + string(out)
}

// New builds a problem.
func New(status int, code, detail string, retryable bool) *Problem {
	return &Problem{Status: status, Code: code, Detail: detail, Retryable: retryable}
}

// With adds a fact to the problem.
func (p *Problem) With(k string, v any) *Problem {
	if p.Data == nil {
		p.Data = map[string]any{}
	}
	p.Data[k] = v
	return p
}

// Error codes (D-14).
const (
	HoldSoldOut         = "EPS-HOLD-SOLD-OUT"
	HoldExpired         = "EPS-HOLD-EXPIRED"
	HoldLimit           = "EPS-HOLD-LIMIT"
	HoldInvalid         = "EPS-HOLD-INVALID"
	EventOffSale        = "EPS-EVENT-OFF-SALE"
	OrderNotFound       = "EPS-ORDER-NOT-FOUND"
	OrderState          = "EPS-ORDER-STATE"
	DetailsMissing      = "EPS-DETAILS-MISSING"
	PayDeclined         = "EPS-PAY-DECLINED"
	Pay3DSFailed        = "EPS-PAY-3DS-FAILED"
	PayCancelled        = "EPS-PAY-CANCELLED"
	PayProvidersBusy    = "EPS-PAY-PROVIDERS-BUSY"
	PayTimeout          = "EPS-PAY-TIMEOUT"
	PayMethod           = "EPS-PAY-METHOD"
	PayInProgress       = "EPS-PAY-IN-PROGRESS"
	WalletInsufficient  = "EPS-WALLET-INSUFFICIENT"
	WalletLimit         = "EPS-WALLET-LIMIT"
	IdempotencyMismatch = "EPS-IDEMPOTENCY-MISMATCH"
	InProgress          = "EPS-IN-PROGRESS"
	LinkInvalid         = "EPS-LINK-INVALID"
	ClaimRefused        = "EPS-CLAIM-REFUSED"
	SavedCardRefused    = "EPS-SAVED-CARD-REFUSED"
	Forbidden           = "EPS-FORBIDDEN"
	Unavailable         = "EPS-UNAVAILABLE"
	PricesChanged       = "EPS-PRICES-CHANGED"
	RateLimited         = "EPS-RATE-LIMITED"
	SavedCardInUse      = "EPS-SAVED-CARD-IN-USE"
	PayCodeInvalid      = "EPS-PAY-CODE-INVALID"
)

// On-sale engine codes (step 5).
const (
	SaleInvalid  = "EPS-SALE-INVALID"
	SaleLocked   = "EPS-SALE-LOCKED"
	SaleNotFound = "EPS-SALE-NOT-FOUND"
	PassRequired = "EPS-PASS-REQUIRED"
	CapReached   = "EPS-CAP-REACHED"
	OTPRequired  = "EPS-OTP-REQUIRED"
	OTPWrong     = "EPS-OTP-WRONG"
	OTPExpired   = "EPS-OTP-EXPIRED"
	OTPLocked    = "EPS-OTP-LOCKED"
	OTPResends   = "EPS-OTP-RESENDS"
	QueueClosed  = "EPS-QUEUE-CLOSED"
)

// Step 5 review codes.
const (
	SaleStale      = "EPS-SALE-STALE"
	SignInRequired = "EPS-SIGN-IN-REQUIRED"
)
