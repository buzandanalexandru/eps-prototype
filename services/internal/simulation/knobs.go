// Package simulation holds the on-sale simulator (step 6, E7, R3): rehearsals and runs in the
// core (design D-1, D-2), the virtual fans and bots of the simulator service (D-3, D-4) and the
// internal API between them.
package simulation

import (
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
)

// ProviderPlan is the timed change of provider A during a run.
type ProviderPlan struct {
	Mode        string  `json:"mode"` // none | outage | failure_rate
	AtSeconds   int     `json:"atSeconds"`
	ForSeconds  int     `json:"forSeconds"`
	FailureRate float64 `json:"failureRate"`
}

// Knobs are a run's settings (design D-2). Zero values take the defaults.
type Knobs struct {
	Fans            int                `json:"fans"`
	BotShare        *float64           `json:"botShare,omitempty"`
	PrequeueShare   *float64           `json:"prequeueShare,omitempty"`
	Arrival         string             `json:"arrival"`
	OpensInSeconds  int                `json:"opensInSeconds"`
	QtyMix          map[string]float64 `json:"qtyMix"`
	PaymentMix      map[string]float64 `json:"paymentMix"`
	AbandonRate     *float64           `json:"abandonRate,omitempty"`
	DeclineRate     *float64           `json:"declineRate,omitempty"`
	PollSeconds     int                `json:"pollSeconds"`
	ProviderA       *ProviderPlan      `json:"providerA,omitempty"`
	DurationSeconds int                `json:"durationSeconds"`
}

// Limits bound a run in this environment (SIM_MAX_FANS; previews 2,000, production 10,000).
type Limits struct {
	MaxFans            int `json:"maxFans"`
	MaxDurationSeconds int `json:"maxDurationSeconds"`
}

// DefaultLimits are the limits without configuration.
var DefaultLimits = Limits{MaxFans: 2000, MaxDurationSeconds: 600}

// Payment methods a simulated fan can use (no wallet or saved card: D-2).
var PaymentMethods = []string{"hosted_card", "apple_pay", "google_pay"}

func f(v float64) *float64 { return &v }

// Defaults fills every unset knob with its default (design D-2).
func (k Knobs) Defaults() Knobs {
	if k.Fans == 0 {
		k.Fans = 1000
	}
	if k.BotShare == nil {
		k.BotShare = f(0.10)
	}
	if k.PrequeueShare == nil {
		k.PrequeueShare = f(0.6)
	}
	if k.Arrival == "" {
		k.Arrival = "spike"
	}
	if k.OpensInSeconds == 0 {
		k.OpensInSeconds = 60
	}
	if len(k.QtyMix) == 0 {
		k.QtyMix = map[string]float64{"1": 35, "2": 40, "3": 10, "4": 15}
	}
	if len(k.PaymentMix) == 0 {
		k.PaymentMix = map[string]float64{"hosted_card": 60, "apple_pay": 20, "google_pay": 20}
	}
	if k.AbandonRate == nil {
		k.AbandonRate = f(0.08)
	}
	if k.DeclineRate == nil {
		k.DeclineRate = f(0.03)
	}
	if k.PollSeconds == 0 {
		k.PollSeconds = 10
	}
	if k.ProviderA == nil {
		k.ProviderA = &ProviderPlan{Mode: "outage", AtSeconds: 60, ForSeconds: 90}
	}
	if k.ProviderA.Mode == "" {
		k.ProviderA.Mode = "none"
	}
	if k.ProviderA.Mode == "failure_rate" && k.ProviderA.FailureRate == 0 {
		k.ProviderA.FailureRate = 0.5
	}
	if k.ProviderA.ForSeconds == 0 {
		k.ProviderA.ForSeconds = 90
	}
	if k.DurationSeconds == 0 {
		k.DurationSeconds = 300
	}
	return k
}

func bad(field string, min, max any) *apperr.Problem {
	return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, fmt.Sprintf("%s must be between %v and %v.", field, min, max), false).
		With("field", field).With("min", min).With("max", max)
}

func rate(field string, v *float64, max float64) *apperr.Problem {
	if v == nil || math.IsNaN(*v) || *v < 0 || *v > max {
		return bad(field, 0, max)
	}
	return nil
}

// Validate refuses a value outside its range, naming the field and the range (after Defaults).
func (k Knobs) Validate(l Limits) *apperr.Problem {
	if l.MaxFans <= 0 {
		l = DefaultLimits
	}
	if k.Fans < 10 || k.Fans > l.MaxFans {
		return bad("fans", 10, l.MaxFans)
	}
	for _, p := range []*apperr.Problem{rate("botShare", k.BotShare, 0.5), rate("prequeueShare", k.PrequeueShare, 1),
		rate("abandonRate", k.AbandonRate, 0.5), rate("declineRate", k.DeclineRate, 0.5)} {
		if p != nil {
			return p
		}
	}
	if k.Arrival != "spike" && k.Arrival != "flat" {
		return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "arrival is spike or flat.", false).With("field", "arrival")
	}
	if k.OpensInSeconds < 30 || k.OpensInSeconds > 600 {
		return bad("opensInSeconds", 30, 600)
	}
	if k.PollSeconds < 5 || k.PollSeconds > 30 {
		return bad("pollSeconds", 5, 30)
	}
	maxDur := l.MaxDurationSeconds
	if maxDur <= 0 || maxDur > 600 {
		maxDur = 600
	}
	if k.DurationSeconds < 60 || k.DurationSeconds > maxDur {
		return bad("durationSeconds", 60, maxDur)
	}
	if err := mix("qtyMix", k.QtyMix, []string{"1", "2", "3", "4"}); err != nil {
		return err
	}
	if err := mix("paymentMix", k.PaymentMix, PaymentMethods); err != nil {
		return err
	}
	a := k.ProviderA
	switch a.Mode {
	case "none":
	case "outage", "failure_rate":
		if a.AtSeconds < 0 || a.AtSeconds > 600 {
			return bad("providerA.atSeconds", 0, 600)
		}
		if a.ForSeconds < 10 || a.ForSeconds > 600 {
			return bad("providerA.forSeconds", 10, 600)
		}
		if a.Mode == "failure_rate" && (math.IsNaN(a.FailureRate) || a.FailureRate <= 0 || a.FailureRate > 1) {
			return bad("providerA.failureRate", 0, 1)
		}
	default:
		return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, "providerA.mode is none, outage or failure_rate.", false).With("field", "providerA.mode")
	}
	return nil
}

// mix checks a weights map: known keys only, weights 0 to 100, at least one above zero.
func mix(field string, m map[string]float64, keys []string) *apperr.Problem {
	known := map[string]bool{}
	for _, k := range keys {
		known[k] = true
	}
	total := 0.0
	for k, v := range m {
		if !known[k] {
			return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, fmt.Sprintf("%s has an unknown key %q.", field, trim(k)), false).With("field", field)
		}
		if math.IsNaN(v) || v < 0 || v > 100 {
			return bad(field, 0, 100)
		}
		total += v
	}
	if total <= 0 {
		return apperr.New(http.StatusUnprocessableEntity, apperr.SimInvalid, field+" needs a weight above zero.", false).With("field", field)
	}
	return nil
}

func trim(s string) string {
	if len(s) > 20 {
		return s[:20]
	}
	return s
}

// Weighted picks keys by weight from a uniform u in [0, 1).
type Weighted struct {
	keys []string
	cum  []float64
}

// NewWeighted builds a picker over a weights map (keys sorted, so a seed gives the same picks).
func NewWeighted(m map[string]float64) Weighted {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w := Weighted{}
	total := 0.0
	for _, k := range keys {
		if m[k] <= 0 {
			continue
		}
		total += m[k]
		w.keys = append(w.keys, k)
		w.cum = append(w.cum, total)
	}
	for i := range w.cum {
		w.cum[i] /= total
	}
	return w
}

// Pick returns the key at u.
func (w Weighted) Pick(u float64) string {
	for i, c := range w.cum {
		if u < c {
			return w.keys[i]
		}
	}
	if len(w.keys) == 0 {
		return ""
	}
	return w.keys[len(w.keys)-1]
}
