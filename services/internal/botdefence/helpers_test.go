package botdefence_test

import (
	"errors"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func problem(err error) (string, map[string]any) {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code, p.Data
	}
	if err != nil {
		return err.Error(), nil
	}
	return "", nil
}

func ga(n int) checkout.Item { return checkout.Item{ProductID: salestest.ProductGA, Qty: n} }

type salesSettings = sales.Settings
