// Package pricing is the Go side of the shared money module (design D1.3; apps/web/src/lib/pricing):
// integer minor units, the platform fee and fee policies, and instalment schedules. It must pass
// the same vectors (apps/web/src/lib/pricing/vectors.json) as the TypeScript module.
package pricing

import (
	"errors"
	"time"
	_ "time/tzdata" // the distroless images have no zoneinfo; schedules need the event's zone
)

// Fee is the platform fee: basis points of the face price, rounded half up, plus a fixed part.
type Fee struct {
	PercentBps int64
	FixedMinor int64
}

// Breakdown is one unit's price split.
type Breakdown struct {
	FaceMinor         int64
	FeeMinor          int64
	FanFeeMinor       int64
	AllInMinor        int64
	OrganiserNetMinor int64
}

// DivRoundHalfUp divides non-negative integers, rounding half up.
func DivRoundHalfUp(n, d int64) int64 {
	if n < 0 || d <= 0 {
		panic("pricing: DivRoundHalfUp takes non-negative integers")
	}
	return (2*n + d) / (2 * d)
}

// PlatformFeeFor is the fee for one unit; a free unit pays none.
func PlatformFeeFor(faceMinor int64, fee Fee) int64 {
	if faceMinor <= 0 {
		return 0
	}
	return DivRoundHalfUp(faceMinor*fee.PercentBps, 10_000) + fee.FixedMinor
}

// FeeBreakdown splits a face price by policy: absorb, pass_on or split (fan's half rounded down).
func FeeBreakdown(faceMinor int64, policy string, fee Fee) Breakdown {
	f := PlatformFeeFor(faceMinor, fee)
	var fan int64
	switch policy {
	case "pass_on":
		fan = f
	case "split":
		fan = f / 2
	}
	all := faceMinor + fan
	return Breakdown{FaceMinor: faceMinor, FeeMinor: f, FanFeeMinor: fan, AllInMinor: all, OrganiserNetMinor: all - f}
}

// LastPaymentDaysBeforeEvent and MaxMonthlyPayments are the instalment rules (C9).
const (
	LastPaymentDaysBeforeEvent = 7
	MaxMonthlyPayments         = 24
	DefaultTimeZone            = "Europe/Bucharest"
)

// DepositFor is a plan's deposit for an all-in amount: fixed, or basis points rounded half up.
func DepositFor(depositMinor, depositBps *int64, allInMinor int64) int64 {
	if depositMinor != nil {
		return *depositMinor
	}
	if depositBps == nil {
		return 0
	}
	return DivRoundHalfUp(allInMinor**depositBps, 10_000)
}

// LocalDate is the calendar date (YYYY-MM-DD) of an instant in a zone.
func LocalDate(t time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation(DefaultTimeZone)
	}
	return t.In(loc).Format(time.DateOnly)
}

// DueDate is the k-th monthly due date (k ≥ 1) after a local purchase date.
func DueDate(fromLocal string, k, dayOfMonth int) string {
	d, err := time.Parse(time.DateOnly, fromLocal)
	if err != nil {
		return ""
	}
	return time.Date(d.Year(), d.Month()+time.Month(k), dayOfMonth, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

// LastAllowedDay is the last local date a monthly payment may fall on.
func LastAllowedDay(eventStartsAt time.Time, tz string) string {
	d, _ := time.Parse(time.DateOnly, LocalDate(eventStartsAt, tz))
	return d.AddDate(0, 0, -LastPaymentDaysBeforeEvent).Format(time.DateOnly)
}

// MaxMonthly is the most monthly payments a buyer purchasing at `from` can get (0 if none fit).
func MaxMonthly(from time.Time, dayOfMonth int, eventStartsAt time.Time, tz string) int {
	fromLocal := LocalDate(from, tz)
	limit := LastAllowedDay(eventStartsAt, tz)
	n := 0
	for n < MaxMonthlyPayments && DueDate(fromLocal, n+1, dayOfMonth) <= limit {
		n++
	}
	return n
}

// Payment is one monthly payment of a schedule.
type Payment struct {
	N           int    `json:"n"`
	DueOn       string `json:"dueOn"`
	AmountMinor int64  `json:"amountMinor"`
}

// Schedule is a deposit and its monthly payments; they add up to TotalMinor.
type Schedule struct {
	DepositMinor int64     `json:"depositMinor"`
	Payments     []Payment `json:"payments"`
	TotalMinor   int64     `json:"totalMinor"`
}

// Schedule errors, as in the TypeScript module.
var (
	ErrInvalidPlan     = errors.New("invalid_plan")
	ErrDepositTooLarge = errors.New("deposit_too_large")
	ErrPastEvent       = errors.New("past_event")
)

// ScheduleInput is what a schedule is computed from.
type ScheduleInput struct {
	AllInMinor    int64
	DepositMinor  int64
	MonthlyCount  int
	DayOfMonth    int
	From          time.Time
	EventStartsAt time.Time
	TimeZone      string
	CapToFit      bool
}

// InstalmentSchedule computes a schedule: equal parts, the rounding remainder on the last one.
// With ErrPastEvent, the int is the most payments that still fit.
func InstalmentSchedule(p ScheduleInput) (Schedule, int, error) {
	if p.AllInMinor < 0 || p.DepositMinor < 0 || p.MonthlyCount < 1 || p.MonthlyCount > MaxMonthlyPayments || p.DayOfMonth < 1 || p.DayOfMonth > 28 {
		return Schedule{}, 0, ErrInvalidPlan
	}
	if p.DepositMinor >= p.AllInMinor {
		return Schedule{}, 0, ErrDepositTooLarge
	}
	tz := p.TimeZone
	if tz == "" {
		tz = DefaultTimeZone
	}
	max := MaxMonthly(p.From, p.DayOfMonth, p.EventStartsAt, tz)
	count := p.MonthlyCount
	if p.CapToFit && max < count {
		count = max
	}
	if count < 1 || count > max {
		return Schedule{}, max, ErrPastEvent
	}
	fromLocal := LocalDate(p.From, tz)
	rest := p.AllInMinor - p.DepositMinor
	each := rest / int64(count)
	out := Schedule{DepositMinor: p.DepositMinor, TotalMinor: p.AllInMinor}
	for k := 1; k <= count; k++ {
		amt := each
		if k == count {
			amt = rest - each*int64(count-1)
		}
		out.Payments = append(out.Payments, Payment{N: k, DueOn: DueDate(fromLocal, k, p.DayOfMonth), AmountMinor: amt})
	}
	return out, max, nil
}
