package queue_test

import (
	"errors"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

type salesSettings = sales.Settings

func salestestGA(n int) checkout.Item { return checkout.Item{ProductID: salestest.ProductGA, Qty: n} }

func code(err error) string {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func problemData(err error) map[string]any {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Data
	}
	return nil
}
