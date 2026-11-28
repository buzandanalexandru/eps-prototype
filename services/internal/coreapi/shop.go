package coreapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

// Shop holds the checkout, payment and wallet services behind the /v1/shop and organiser payment
// operations (step 4). Nil in contract tests without a database: every operation then answers 503.
type Shop struct {
	Checkout *checkout.Service
	Payments *payments.Service
	Wallet   *wallet.Service
	Knobs    map[string]*payments.Knobs
	// ReturnOrigins are the web origins a provider page may send the fan back to (the web app's
	// exact origins); localhost is allowed when the list is empty (local development).
	ReturnOrigins []string
	Log           *slog.Logger
}

// raw answers any strict operation with a status and a JSON body (or a problem document).
type raw struct {
	status  int
	body    any
	problem bool
}

func (r raw) write(w http.ResponseWriter) error {
	if r.status == http.StatusNoContent {
		w.WriteHeader(r.status)
		return nil
	}
	ct := "application/json"
	if r.problem {
		ct = "application/problem+json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(r.status)
	return json.NewEncoder(w).Encode(r.body)
}

func (r raw) VisitCreateOrderResponse(w http.ResponseWriter) error            { return r.write(w) }
func (r raw) VisitListMyOrdersResponse(w http.ResponseWriter) error           { return r.write(w) }
func (r raw) VisitGetOrderResponse(w http.ResponseWriter) error               { return r.write(w) }
func (r raw) VisitSetOrderDetailsResponse(w http.ResponseWriter) error        { return r.write(w) }
func (r raw) VisitPayOrderResponse(w http.ResponseWriter) error               { return r.write(w) }
func (r raw) VisitCancelOrderResponse(w http.ResponseWriter) error            { return r.write(w) }
func (r raw) VisitSimulateNextInstalmentResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitPayOverdueInstalmentResponse(w http.ResponseWriter) error   { return r.write(w) }
func (r raw) VisitClaimOrdersResponse(w http.ResponseWriter) error            { return r.write(w) }
func (r raw) VisitListSavedCardsResponse(w http.ResponseWriter) error         { return r.write(w) }
func (r raw) VisitDeleteSavedCardResponse(w http.ResponseWriter) error        { return r.write(w) }
func (r raw) VisitListWalletsResponse(w http.ResponseWriter) error            { return r.write(w) }
func (r raw) VisitGetWalletResponse(w http.ResponseWriter) error              { return r.write(w) }
func (r raw) VisitTopUpWalletResponse(w http.ResponseWriter) error            { return r.write(w) }
func (r raw) VisitGetTopupResponse(w http.ResponseWriter) error               { return r.write(w) }
func (r raw) VisitGetPaymentProvidersResponse(w http.ResponseWriter) error    { return r.write(w) }
func (r raw) VisitSetProviderKnobsResponse(w http.ResponseWriter) error       { return r.write(w) }
func (r raw) VisitStandPaymentResponse(w http.ResponseWriter) error           { return r.write(w) }
func (r raw) VisitCloseSeasonResponse(w http.ResponseWriter) error            { return r.write(w) }

func okRaw(v any) raw { return raw{status: http.StatusOK, body: v} }

// fail turns a service error into a problem answer (logging unexpected ones).
func (s *Server) fail(ctx context.Context, err error) raw {
	var p *apperr.Problem
	switch {
	case errors.As(err, &p):
		return problemRaw(ctx, p)
	case errors.Is(err, checkout.ErrNotFound):
		return problemRaw(ctx, apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found.", false))
	}
	if s.Shop != nil && s.Shop.Log != nil {
		s.Shop.Log.Error("shop operation failed", "error", err.Error(), "requestId", httpx.RequestID(ctx))
	}
	return problemRaw(ctx, apperr.New(http.StatusInternalServerError, "internal", "", true))
}

func problemRaw(ctx context.Context, p *apperr.Problem) raw {
	doc := problemFor(ctx, p.Status, p.Code, p.Detail)
	if strings.HasPrefix(p.Code, "EPS-") {
		mk := p.MessageKey()
		doc.MessageKey = &mk
	}
	r := p.Retryable
	doc.Retryable = &r
	if len(p.Data) > 0 {
		d := map[string]any(p.Data)
		doc.Data = &d
	}
	return raw{status: p.Status, body: doc, problem: true}
}

func unavailable(ctx context.Context) raw {
	return problemRaw(ctx, apperr.New(http.StatusServiceUnavailable, apperr.Unavailable, "Checkout is not configured.", true))
}

// buyerOf is the caller as a buyer: an account (fan or organiser token, with its verified email)
// or a guest.
func buyerOf(ctx context.Context) (checkout.Buyer, bool) {
	c, ok := auth.FromContext(ctx)
	if !ok || c.Subject == "" {
		return checkout.Buyer{}, false
	}
	if c.Kind == auth.KindGuest {
		return checkout.Buyer{ID: c.Subject, Kind: "guest"}, true
	}
	return checkout.Buyer{ID: c.Subject, Kind: "account", Email: c.Email}, true
}

func needBuyer(ctx context.Context) raw {
	return problemRaw(ctx, apperr.New(http.StatusUnauthorized, "unauthenticated", "A core token is required.", false))
}

// OrderView is an order as the fan's screens read it.
type OrderView struct {
	*checkout.Order
	ServerTime      time.Time              `json:"serverTime"`
	EventName       any                    `json:"eventName,omitempty"`
	Questions       []checkout.Question    `json:"questions"`
	InstalmentOffer *checkout.Offer        `json:"instalmentOffer,omitempty"`
	Plan            *planView              `json:"plan,omitempty"`
	Payment         *payments.Attempt      `json:"payment,omitempty"`
	CanPay          bool                   `json:"canPay"`
	Access          string                 `json:"access"`
	ResumeToken     string                 `json:"resumeToken,omitempty"`
	ViewToken       string                 `json:"viewToken,omitempty"`
	Wallet          *walletOption          `json:"wallet,omitempty"`
	SavedCards      []payments.SavedMethod `json:"savedCards"`
}

type planView struct {
	*checkout.Plan
	SavedCardLabel string `json:"savedCardLabel,omitempty"`
}

type walletOption struct {
	BalanceMinor int64 `json:"balanceMinor"`
	CreditMinor  int64 `json:"creditMinor"`
}

// Access modes.
const (
	accessBuyer       = "buyer"
	accessLink        = "link"
	accessExpiredLink = "expired_link"
)

// orderFor loads an order the caller may see: its buyer, or a valid order link. Anything else is
// "Order not found" (an order's existence is never confirmed to a stranger); a changed link is
// EPS-LINK-INVALID.
func (s *Server) orderFor(ctx context.Context, id, token string) (*checkout.Order, string, error) {
	o, err := s.Shop.Checkout.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if b, ok := buyerOf(ctx); ok && b.ID == o.BuyerID {
		return o, accessBuyer, nil
	}
	if token == "" {
		return nil, "", checkout.ErrNotFound
	}
	links := s.Shop.Checkout.Links
	_, err = links.Check(token, o.ID, checkout.LinkResume)
	switch {
	case err == nil:
		return o, accessLink, nil
	case errors.Is(err, checkout.ErrLinkExpired):
		return o, accessExpiredLink, nil
	}
	if _, err := links.Check(token, o.ID, checkout.LinkView); err == nil {
		return o, accessExpiredLink, nil // a view link shows the order; paying needs the buyer or a resume link
	} else if errors.Is(err, checkout.ErrLinkExpired) {
		return nil, "", checkout.ErrNotFound
	}
	return nil, "", apperr.New(http.StatusForbidden, apperr.LinkInvalid, "Link not valid.", false)
}

func (s *Server) viewOf(ctx context.Context, o *checkout.Order, access string) OrderView {
	sh := s.Shop
	v := OrderView{Order: o, ServerTime: time.Now().UTC(), Access: access, Questions: []checkout.Question{}, SavedCards: []payments.SavedMethod{}}
	if qs, err := sh.Checkout.Questions(ctx, o); err == nil {
		v.Questions = qs
	}
	if snap, err := s.Snapshots.LoadVersion(ctx, o.EventID, o.SnapshotVersion); err == nil {
		v.EventName = snap.Event.Name
	}
	live := o.State == checkout.Held || o.State == checkout.PaymentFailed || o.State == checkout.Paying
	holdLive := o.HoldExpiresAt.After(time.Now())
	v.CanPay = (o.State == checkout.Held || o.State == checkout.PaymentFailed) && holdLive && (access == accessBuyer || access == accessLink)
	if v.CanPay {
		if off, err := sh.Checkout.Offer(ctx, o); err == nil {
			v.InstalmentOffer = off
		}
	}
	if p, err := sh.Checkout.PlanOf(ctx, o.OrganiserID, o.ID); err == nil && p != nil {
		pv := &planView{Plan: p}
		if p.SavedMethodID != nil {
			if m, err := sh.Payments.SavedMethod(ctx, o.OrganiserID, o.BuyerID, *p.SavedMethodID); err == nil {
				pv.SavedCardLabel = m.Label
			}
		}
		v.Plan = pv
	}
	if a, err := sh.Payments.LatestAttempt(ctx, o.OrganiserID, o.ID); err == nil && a != nil {
		v.Payment = a
	}
	if live && holdLive && (access == accessBuyer || access == accessLink) {
		v.ResumeToken = sh.Checkout.Links.ResumeLink(o)
	}
	if access == accessBuyer && o.BuyerKind == "guest" {
		v.ViewToken = sh.Checkout.Links.ViewLink(o)
	}
	if b, ok := buyerOf(ctx); ok && b.Kind == "account" && b.ID == o.BuyerID {
		if w, err := sh.Wallet.Wallet(ctx, o.OrganiserID, b.ID, o.Currency); err == nil {
			v.Wallet = &walletOption{BalanceMinor: w.BalanceMinor, CreditMinor: w.CreditMinor}
		}
		if cards, err := sh.Payments.SavedMethods(ctx, o.OrganiserID, b.ID); err == nil {
			v.SavedCards = cards
		}
	}
	return v
}

func token(t *string) string {
	if t == nil {
		return ""
	}
	return *t
}

func (s *Server) CreateOrder(ctx context.Context, req api.CreateOrderRequestObject) (api.CreateOrderResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok := buyerOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	key := ""
	if req.Params.IdempotencyKey != nil {
		key = *req.Params.IdempotencyKey
	}
	o, existing, err := s.Shop.Checkout.CreateOrder(ctx, b, checkout.CreateRequest(*req.Body), key)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	st := http.StatusCreated
	if existing {
		st = http.StatusOK
	}
	return raw{status: st, body: s.viewOf(ctx, o, accessBuyer)}, nil
}

func (s *Server) ListMyOrders(ctx context.Context, _ api.ListMyOrdersRequestObject) (api.ListMyOrdersResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok := buyerOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	list, err := s.Shop.Checkout.OrdersOf(ctx, b.ID)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	out := make([]OrderView, 0, len(list))
	for _, o := range list {
		out = append(out, s.viewOf(ctx, o, accessBuyer))
	}
	return okRaw(map[string]any{"orders": out}), nil
}

func (s *Server) GetOrder(ctx context.Context, req api.GetOrderRequestObject) (api.GetOrderResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	o, access, err := s.orderFor(ctx, req.OrderId.String(), token(req.Params.XOrderToken))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.viewOf(ctx, o, access)), nil
}

// mutable loads an order the caller may change: its buyer, or a live resume link.
func (s *Server) mutable(ctx context.Context, id string, tok *string) (*checkout.Order, string, error) {
	o, access, err := s.orderFor(ctx, id, token(tok))
	if err != nil {
		return nil, "", err
	}
	if access != accessBuyer && access != accessLink {
		if o.State == checkout.Held || o.State == checkout.PaymentFailed {
			return nil, "", apperr.New(http.StatusConflict, apperr.HoldExpired, "The hold ended and the tickets were released.", false)
		}
		return nil, "", apperr.New(http.StatusConflict, apperr.OrderState, "This order cannot be changed now.", false).With("state", o.State)
	}
	return o, access, nil
}

func (s *Server) SetOrderDetails(ctx context.Context, req api.SetOrderDetailsRequestObject) (api.SetOrderDetailsResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	o, access, err := s.mutable(ctx, req.OrderId.String(), req.Params.XOrderToken)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	o, err = s.Shop.Checkout.SetDetails(ctx, o, checkout.Details(*req.Body))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.viewOf(ctx, o, access)), nil
}

// returnURL keeps a provider return address only on an allowed web origin.
func (s *Server) returnURL(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(raw) > 2000 {
		return "", false
	}
	origin := u.Scheme + "://" + u.Host
	if len(s.Shop.ReturnOrigins) == 0 {
		return raw, u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	}
	for _, o := range s.Shop.ReturnOrigins {
		if o == origin {
			return raw, true
		}
	}
	return "", false
}

func (s *Server) PayOrder(ctx context.Context, req api.PayOrderRequestObject) (api.PayOrderResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, _ := buyerOf(ctx)
	o, access, err := s.mutable(ctx, req.OrderId.String(), req.Params.XOrderToken)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	body := req.Body
	if access == accessLink {
		// Paying through a resume link acts for the order's buyer (the link is the authority).
		b = checkout.Buyer{ID: o.BuyerID, Kind: o.BuyerKind}
	}
	if string(body.Method) == "wallet" {
		paid, err := s.Shop.Wallet.PayOrder(ctx, b, o, req.Params.IdempotencyKey)
		if err != nil {
			return s.fail(ctx, err), nil
		}
		return okRaw(map[string]any{"order": s.viewOf(ctx, paid, access)}), nil
	}
	ret, allowed := s.returnURL(token(body.ReturnUrl))
	if !allowed {
		return problemRaw(ctx, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "returnUrl is not on an allowed origin.", false)), nil
	}
	pr := payments.PayRequest{Method: string(body.Method), ReturnURL: ret}
	if body.SavedCardId != nil {
		pr.SavedCardID = *body.SavedCardId
	}
	if body.SaveCard != nil {
		pr.SaveCard = *body.SaveCard
	}
	if body.Plan != nil {
		pr.Plan = string(*body.Plan)
	}
	res, err := s.Shop.Payments.Pay(ctx, b, o, pr, req.Params.IdempotencyKey)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	cur, err := s.Shop.Checkout.GetIn(ctx, o.OrganiserID, o.ID)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	out := map[string]any{"order": s.viewOf(ctx, cur, access)}
	if res.ActionURL != "" {
		out["actionUrl"] = res.ActionURL
	}
	return okRaw(out), nil
}

func (s *Server) CancelOrder(ctx context.Context, req api.CancelOrderRequestObject) (api.CancelOrderResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	o, access, err := s.mutable(ctx, req.OrderId.String(), req.Params.XOrderToken)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	o, err = s.Shop.Checkout.Cancel(ctx, o)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.viewOf(ctx, o, access)), nil
}

func (s *Server) SimulateNextInstalment(ctx context.Context, req api.SimulateNextInstalmentRequestObject) (api.SimulateNextInstalmentResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	o, access, err := s.orderFor(ctx, req.OrderId.String(), token(req.Params.XOrderToken))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if access != accessBuyer {
		return s.fail(ctx, checkout.ErrNotFound), nil
	}
	if _, err := s.Shop.Checkout.SimulateNextDue(ctx, o.OrganiserID, o.ID); err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.viewOf(ctx, o, access)), nil
}

func (s *Server) PayOverdueInstalment(ctx context.Context, req api.PayOverdueInstalmentRequestObject) (api.PayOverdueInstalmentResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	o, access, err := s.orderFor(ctx, req.OrderId.String(), token(req.Params.XOrderToken))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if access != accessBuyer {
		return s.fail(ctx, checkout.ErrNotFound), nil
	}
	if _, err := s.Shop.Payments.PayOverdue(ctx, o.OrganiserID, o.ID); err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.viewOf(ctx, o, access)), nil
}

func (s *Server) ClaimOrders(ctx context.Context, req api.ClaimOrdersRequestObject) (api.ClaimOrdersResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 {
		return needBuyer(ctx), nil
	}
	id := ""
	if req.Body != nil && req.Body.OrderId != nil {
		id = req.Body.OrderId.String()
	}
	claimed, err := s.Shop.Checkout.Claim(ctx, b, id)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if claimed == nil {
		claimed = []string{}
	}
	return okRaw(map[string]any{"claimed": claimed}), nil
}

func (s *Server) ListSavedCards(ctx context.Context, req api.ListSavedCardsRequestObject) (api.ListSavedCardsResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 || b.Kind != "account" {
		return okRaw(map[string]any{"cards": []any{}}), nil
	}
	cards, err := s.Shop.Payments.SavedMethods(ctx, req.Params.OrganiserId.String(), b.ID)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(map[string]any{"cards": cards}), nil
}

func (s *Server) DeleteSavedCard(ctx context.Context, req api.DeleteSavedCardRequestObject) (api.DeleteSavedCardResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 {
		return needBuyer(ctx), nil
	}
	if err := s.Shop.Payments.DeleteSavedMethod(ctx, req.Params.OrganiserId.String(), b.ID, req.CardId.String()); err != nil {
		return s.fail(ctx, err), nil
	}
	return raw{status: http.StatusNoContent}, nil
}

func (s *Server) ListWallets(ctx context.Context, _ api.ListWalletsRequestObject) (api.ListWalletsResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 || b.Kind != "account" {
		return okRaw(map[string]any{"wallets": []any{}}), nil
	}
	ws, err := s.Shop.Wallet.Wallets(ctx, b.ID)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(map[string]any{"wallets": ws}), nil
}

func (s *Server) GetWallet(ctx context.Context, req api.GetWalletRequestObject) (api.GetWalletResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 || b.Kind != "account" {
		return problemRaw(ctx, apperr.New(http.StatusForbidden, apperr.Forbidden, "Sign in to use the wallet.", false)), nil
	}
	cur := "RON"
	if req.Params.Currency != nil {
		cur = string(*req.Params.Currency)
	}
	w, err := s.Shop.Wallet.Wallet(ctx, req.OrganiserId.String(), b.ID, cur)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(w), nil
}

func (s *Server) TopUpWallet(ctx context.Context, req api.TopUpWalletRequestObject) (api.TopUpWalletResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 {
		return needBuyer(ctx), nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	ret, allowed := s.returnURL(token(req.Body.ReturnUrl))
	if !allowed {
		return problemRaw(ctx, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "returnUrl is not on an allowed origin.", false)), nil
	}
	tr := wallet.TopupRequest{AmountMinor: req.Body.AmountMinor, Currency: string(req.Body.Currency), Method: string(req.Body.Method), ReturnURL: ret}
	if req.Body.Locale != nil {
		tr.Locale = string(*req.Body.Locale)
	}
	res, err := s.Shop.Wallet.Topup(ctx, b, req.OrganiserId.String(), tr, req.Params.IdempotencyKey)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	out := map[string]any{"attempt": res.Attempt}
	if res.ActionURL != "" {
		out["actionUrl"] = res.ActionURL
	}
	return okRaw(out), nil
}

func (s *Server) GetTopup(ctx context.Context, req api.GetTopupRequestObject) (api.GetTopupResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	b, ok2 := buyerOf(ctx)
	if !ok2 {
		return needBuyer(ctx), nil
	}
	org := req.OrganiserId.String()
	a, err := s.Shop.Payments.GetAttempt(ctx, org, req.AttemptId.String())
	if err != nil || a.Purpose != "topup" || a.BuyerID != b.ID {
		return problemRaw(ctx, apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Top-up not found.", false)), nil
	}
	out := map[string]any{"attempt": a}
	if w, err := s.Shop.Wallet.Wallet(ctx, org, b.ID, a.Currency); err == nil {
		out["wallet"] = w
	}
	return okRaw(out), nil
}

// organiserRole returns the caller's organiser when its role is one of roles.
func organiserRole(ctx context.Context, roles ...string) (string, raw, bool) {
	c, okc := auth.FromContext(ctx)
	if !okc {
		return "", needBuyer(ctx), false
	}
	if c.Kind != auth.KindOrganiser || c.Org == "" {
		return "", problemRaw(ctx, apperr.New(http.StatusForbidden, "forbidden", "An organiser token is required.", false)), false
	}
	for _, r := range roles {
		if c.Role == r {
			return c.Org, raw{}, true
		}
	}
	return "", problemRaw(ctx, apperr.New(http.StatusForbidden, "forbidden", "Your role cannot do this.", false)), false
}

func (s *Server) providers(ctx context.Context, org string) map[string]any {
	list := []map[string]any{}
	for _, p := range []string{"a", "b"} {
		st := s.Shop.Payments.Breaker.Read(ctx, org, p)
		item := map[string]any{"provider": p, "state": st.State, "calls": st.Calls, "failures": st.Failures, "consecutiveFailures": st.Consecutive, "reachable": false}
		if !st.OpenedAt.IsZero() {
			item["openedAt"] = st.OpenedAt.UTC()
		}
		if k, ok := s.Shop.Knobs[p]; ok && k != nil {
			if body, err := k.Get(ctx, org); err == nil {
				item["knobs"] = json.RawMessage(body)
				item["reachable"] = true
			}
		}
		list = append(list, item)
	}
	pref := s.Shop.Payments.Preferred
	if pref != "b" {
		pref = "a"
	}
	return map[string]any{"preferred": pref, "providers": list}
}

func (s *Server) GetPaymentProviders(ctx context.Context, _ api.GetPaymentProvidersRequestObject) (api.GetPaymentProvidersResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	org, r, okr := organiserRole(ctx, "owner", "admin", "finance", "box_office", "staff")
	if !okr {
		return r, nil
	}
	return okRaw(s.providers(ctx, org)), nil
}

func (s *Server) SetProviderKnobs(ctx context.Context, req api.SetProviderKnobsRequestObject) (api.SetProviderKnobsResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	org, r, okr := organiserRole(ctx, "owner", "admin")
	if !okr {
		return r, nil
	}
	k, okk := s.Shop.Knobs[string(req.Provider)]
	if !okk || k == nil || req.Body == nil {
		return unavailable(ctx), nil
	}
	// The generated body (float32 rates) re-read as the provider's own knobs, then validated.
	b, _ := json.Marshal(req.Body)
	knobs := fakepsp.DefaultKnobs()
	if err := json.Unmarshal(b, &knobs); err != nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", err.Error(), false)), nil
	}
	for _, f := range []*float64{&knobs.FailureRate, &knobs.DeclineRate, &knobs.TimeoutRate, &knobs.WebhookDropRate, &knobs.ChallengeRate} {
		*f = float64(int(*f*1000+0.5)) / 1000 // float32 noise off
	}
	if err := knobs.Validate(); err != nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", err.Error(), false)), nil
	}
	body, _ := json.Marshal(knobs)
	if _, err := k.Put(ctx, org, body); err != nil {
		return problemRaw(ctx, apperr.New(http.StatusBadGateway, apperr.Unavailable, "The simulated provider did not answer.", true)), nil
	}
	return okRaw(s.providers(ctx, org)), nil
}

func (s *Server) StandPayment(ctx context.Context, req api.StandPaymentRequestObject) (api.StandPaymentResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	org, r, okr := organiserRole(ctx, "owner", "admin", "box_office", "staff")
	if !okr {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	label := token(req.Body.Label)
	w, err := s.Shop.Wallet.Stand(ctx, org, req.Body.Code, req.Body.AmountMinor, label)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(w), nil
}

func (s *Server) CloseSeason(ctx context.Context, req api.CloseSeasonRequestObject) (api.CloseSeasonResponseObject, error) {
	if s.Shop == nil {
		return unavailable(ctx), nil
	}
	org, r, okr := organiserRole(ctx, "owner", "admin")
	if !okr {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	n, total, err := s.Shop.Wallet.CloseSeason(ctx, org, req.Body.ExpiresAt)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(map[string]any{"fans": n, "totalMinor": total}), nil
}
