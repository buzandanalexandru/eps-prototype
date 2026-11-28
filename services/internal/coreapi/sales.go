package coreapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsale"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// OnSale holds the step 5 services behind the sale settings and OTP operations (nil in contract
// tests without a database: they answer 503).
type OnSale struct {
	Sales *sales.Service
	Gate  *onsale.Gate
	Redis *redis.Client
}

func (r raw) VisitListSalesResponse(w http.ResponseWriter) error          { return r.write(w) }
func (r raw) VisitCreateSaleResponse(w http.ResponseWriter) error         { return r.write(w) }
func (r raw) VisitGetSaleResponse(w http.ResponseWriter) error            { return r.write(w) }
func (r raw) VisitUpdateSaleSettingsResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitSendOrderOtpResponse(w http.ResponseWriter) error       { return r.write(w) }
func (r raw) VisitVerifyOrderOtpResponse(w http.ResponseWriter) error     { return r.write(w) }
func (r raw) VisitJoinQueueResponse(w http.ResponseWriter) error          { return r.write(w) }
func (r raw) VisitQueueStatusResponse(w http.ResponseWriter) error        { return r.write(w) }

func settingsOf(b api.SaleSettings) sales.Settings {
	var st sales.Settings
	j, _ := json.Marshal(b)
	_ = json.Unmarshal(j, &st)
	return st
}

// saleView is a sale as Studio reads it.
type saleView struct {
	sales.Sale
	Phase           sales.Phase     `json:"phase"`
	PrequeueOpensAt time.Time       `json:"prequeueOpensAt"`
	History         []sales.Version `json:"history,omitempty"`
	Queue           *queue.Counts   `json:"queue,omitempty"`
}

func (s *Server) view(ctx context.Context, x *sales.Sale, hist []sales.Version, counts bool) saleView {
	v := saleView{Sale: *x, Phase: x.Settings.PhaseAt(time.Now()), PrequeueOpensAt: x.Settings.PrequeueOpensAt(), History: hist}
	if counts && s.OnSale.Redis != nil {
		if c, err := queue.CountsOf(ctx, s.OnSale.Redis, x.OrganiserID, x.EventID, x.ID, time.Now()); err == nil {
			v.Queue = &c
		}
	}
	return v
}

func by(ctx context.Context) string {
	c, _ := auth.FromContext(ctx)
	if c == nil {
		return ""
	}
	return c.Subject
}

func (s *Server) ListSales(ctx context.Context, req api.ListSalesRequestObject) (api.ListSalesResponseObject, error) {
	if s.OnSale == nil {
		return unavailable(ctx), nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin", "finance", "box_office", "staff")
	if !ok {
		return r, nil
	}
	list, err := s.OnSale.Sales.List(ctx, org, req.EventId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	out := make([]saleView, 0, len(list))
	for i := range list {
		out = append(out, s.view(ctx, &list[i], nil, true))
	}
	return okRaw(map[string]any{"sales": out}), nil
}

func (s *Server) CreateSale(ctx context.Context, req api.CreateSaleRequestObject) (api.CreateSaleResponseObject, error) {
	if s.OnSale == nil {
		return unavailable(ctx), nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin")
	if !ok {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	x, err := s.OnSale.Sales.Create(ctx, org, req.EventId.String(), req.Body.Name, by(ctx), settingsOf(req.Body.Settings))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return raw{status: http.StatusCreated, body: s.view(ctx, x, nil, false)}, nil
}

func (s *Server) GetSale(ctx context.Context, req api.GetSaleRequestObject) (api.GetSaleResponseObject, error) {
	if s.OnSale == nil {
		return unavailable(ctx), nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin", "finance", "box_office", "staff")
	if !ok {
		return r, nil
	}
	x, hist, err := s.OnSale.Sales.Get(ctx, org, req.SaleId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.view(ctx, x, hist, true)), nil
}

func (s *Server) UpdateSaleSettings(ctx context.Context, req api.UpdateSaleSettingsRequestObject) (api.UpdateSaleSettingsResponseObject, error) {
	if s.OnSale == nil {
		return unavailable(ctx), nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin")
	if !ok {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	x, err := s.OnSale.Sales.Update(ctx, org, req.SaleId.String(), by(ctx), settingsOf(*req.Body))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	_, hist, _ := s.OnSale.Sales.Get(ctx, org, x.ID)
	return okRaw(s.view(ctx, x, hist, true)), nil
}

// withPass adds the waiting room's headers to the caller as a buyer.
func withPass(b checkout.Buyer, pass, session *string) checkout.Buyer {
	if pass != nil {
		b.Pass = *pass
	}
	if session != nil {
		b.Session = *session
	}
	return b
}

// otpOrder loads an order the caller may act on and checks its pass (D-11: the OTP needs it too).
func (s *Server) otpOrder(ctx context.Context, id string, tok, pass, session *string) (*checkout.Order, string, raw, bool) {
	if s.Shop == nil || s.OnSale == nil {
		return nil, "", unavailable(ctx), false
	}
	o, access, err := s.mutable(ctx, id, tok)
	if err != nil {
		return nil, "", s.fail(ctx, err), false
	}
	b, _ := buyerOf(ctx)
	if p := s.OnSale.Gate.PassOnly(ctx, o, withPass(b, pass, session), access == accessLink); p != nil {
		return nil, "", s.fail(ctx, p), false
	}
	return o, access, raw{}, true
}

func (s *Server) SendOrderOtp(ctx context.Context, req api.SendOrderOtpRequestObject) (api.SendOrderOtpResponseObject, error) {
	o, _, r, ok := s.otpOrder(ctx, req.OrderId.String(), req.Params.XOrderToken, req.Params.XEPSPass, req.Params.XEPSSession)
	if !ok {
		return r, nil
	}
	c, err := s.OnSale.Gate.SendOTP(ctx, o)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(map[string]any{"code": c.Code, "expiresAt": c.ExpiresAt, "resendsLeft": c.ResendsLeft, "triesLeft": c.TriesLeft, "simulated": true}), nil
}

func (s *Server) VerifyOrderOtp(ctx context.Context, req api.VerifyOrderOtpRequestObject) (api.VerifyOrderOtpResponseObject, error) {
	o, access, r, ok := s.otpOrder(ctx, req.OrderId.String(), req.Params.XOrderToken, req.Params.XEPSPass, req.Params.XEPSSession)
	if !ok {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	if err := s.OnSale.Gate.VerifyOTP(ctx, o, req.Body.Code); err != nil {
		return s.fail(ctx, err), nil
	}
	cur, err := s.Shop.Checkout.GetIn(ctx, o.OrganiserID, o.ID)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.viewOf(ctx, cur, access)), nil
}

func wrongService(ctx context.Context) raw {
	return problemRaw(ctx, apperr.New(http.StatusMisdirectedRequest, "EPS-WRONG-SERVICE", "The waiting room is served by the gateway.", false))
}

// JoinQueue and QueueStatus are the gateway's operations; the core does not serve them.
func (s *Server) JoinQueue(ctx context.Context, _ api.JoinQueueRequestObject) (api.JoinQueueResponseObject, error) {
	return wrongService(ctx), nil
}

func (s *Server) QueueStatus(ctx context.Context, _ api.QueueStatusRequestObject) (api.QueueStatusResponseObject, error) {
	return wrongService(ctx), nil
}
