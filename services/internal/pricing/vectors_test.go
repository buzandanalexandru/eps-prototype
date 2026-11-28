package pricing_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/pricing"
)

type vectors struct {
	PlatformFee struct {
		PercentBps int64 `json:"percentBps"`
		FixedMinor int64 `json:"fixedMinor"`
	} `json:"platformFee"`
	Fees []struct {
		Name              string `json:"name"`
		FaceMinor         int64  `json:"faceMinor"`
		Policy            string `json:"policy"`
		FeeMinor          int64  `json:"feeMinor"`
		FanFeeMinor       int64  `json:"fanFeeMinor"`
		AllInMinor        int64  `json:"allInMinor"`
		OrganiserNetMinor int64  `json:"organiserNetMinor"`
	} `json:"fees"`
	Instalments []struct {
		Name          string  `json:"name"`
		AllInMinor    int64   `json:"allInMinor"`
		DepositMinor  int64   `json:"depositMinor"`
		MonthlyCount  int     `json:"monthlyCount"`
		DayOfMonth    int     `json:"dayOfMonth"`
		TimeZone      string  `json:"timeZone"`
		CapToFit      bool    `json:"capToFit"`
		From          string  `json:"from"`
		EventStartsAt string  `json:"eventStartsAt"`
		OK            bool    `json:"ok"`
		Amounts       []int64 `json:"amounts"`
		FirstDue      string  `json:"firstDue"`
		LastDue       string  `json:"lastDue"`
		Code          string  `json:"code"`
		MaxCount      *int    `json:"maxCount"`
	} `json:"instalments"`
}

func load(t *testing.T) vectors {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		p := filepath.Join(dir, "apps", "web", "src", "lib", "pricing", "vectors.json")
		if b, err := os.ReadFile(p); err == nil {
			var v vectors
			if err := json.Unmarshal(b, &v); err != nil {
				t.Fatal(err)
			}
			return v
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("vectors.json not found")
		}
		dir = parent
	}
}

// The shared pricing vectors (spec map): the Go fee split must equal the TypeScript one.
func TestFeeVectors(t *testing.T) {
	v := load(t)
	fee := pricing.Fee{PercentBps: v.PlatformFee.PercentBps, FixedMinor: v.PlatformFee.FixedMinor}
	for _, c := range v.Fees {
		b := pricing.FeeBreakdown(c.FaceMinor, c.Policy, fee)
		if b.FeeMinor != c.FeeMinor || b.FanFeeMinor != c.FanFeeMinor || b.AllInMinor != c.AllInMinor || b.OrganiserNetMinor != c.OrganiserNetMinor {
			t.Errorf("%s: got %+v", c.Name, b)
		}
	}
}

func TestInstalmentVectors(t *testing.T) {
	v := load(t)
	for _, c := range v.Instalments {
		from, _ := time.Parse(time.RFC3339, c.From)
		ev, _ := time.Parse(time.RFC3339, c.EventStartsAt)
		s, max, err := pricing.InstalmentSchedule(pricing.ScheduleInput{AllInMinor: c.AllInMinor, DepositMinor: c.DepositMinor, MonthlyCount: c.MonthlyCount,
			DayOfMonth: c.DayOfMonth, From: from, EventStartsAt: ev, TimeZone: c.TimeZone, CapToFit: c.CapToFit})
		if !c.OK {
			if err == nil || err.Error() != c.Code {
				t.Errorf("%s: want %s, got %v", c.Name, c.Code, err)
			}
			if c.MaxCount != nil && errors.Is(err, pricing.ErrPastEvent) && max != *c.MaxCount {
				t.Errorf("%s: max %d, want %d", c.Name, max, *c.MaxCount)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		if len(s.Payments) != len(c.Amounts) {
			t.Errorf("%s: %d payments, want %d", c.Name, len(s.Payments), len(c.Amounts))
			continue
		}
		sum := s.DepositMinor
		for i, p := range s.Payments {
			sum += p.AmountMinor
			if p.AmountMinor != c.Amounts[i] {
				t.Errorf("%s: payment %d is %d, want %d", c.Name, i+1, p.AmountMinor, c.Amounts[i])
			}
		}
		if sum != c.AllInMinor {
			t.Errorf("%s: parts add up to %d, not %d", c.Name, sum, c.AllInMinor)
		}
		if s.Payments[0].DueOn != c.FirstDue || s.Payments[len(s.Payments)-1].DueOn != c.LastDue {
			t.Errorf("%s: due %s..%s, want %s..%s", c.Name, s.Payments[0].DueOn, s.Payments[len(s.Payments)-1].DueOn, c.FirstDue, c.LastDue)
		}
	}
}
