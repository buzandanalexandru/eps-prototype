package coreapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
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
	// Hold limits per client (review D-h): new orders per minute and live holds at once; 0 off.
	Redis        *redis.Client
	HoldsPerMin  int
	LiveHoldsPer int
}

// clientKey is the rate-limit identity of the caller: the browser's address the web app put in
// the token, else the token's subject.
func clientKey(ctx context.Context) string {
	c, ok := auth.FromContext(ctx)
	if !ok {
		return ""
	}
	if c.ClientIP != "" && len(c.ClientIP) <= 64 && !strings.ContainsAny(c.ClientIP, " {}") {
		if c.Sim {
			// A simulator's synthetic address never shares a real client's limits (step 6).
			return "sim-" + strings.ReplaceAll(c.ClientIP, ":", "_")
		}
		return "ip-" + strings.ReplaceAll(c.ClientIP, ":", "_")
	}
	return "sub-" + strings.ReplaceAll(c.Subject, ":", "_")
}

var rateLimited = metrics.NewCounter("eps_holds_rate_limited_total", "Hold requests refused by the per-client limits.")

// holdAllowed applies the per-client limits before a new hold.
func (s *Server) holdAllowed(ctx context.Context) bool {
	sh := s.Shop
	who := clientKey(ctx)
	if sh.Redis == nil || who == "" {
		return true
	}
	now := time.Now()
	if sh.HoldsPerMin > 0 {
		k, _ := redisx.PlatformKey("rl", "holds", who, strconv.FormatInt(now.Unix()/60, 10))
		n, err := sh.Redis.Incr(ctx, k).Result()
		if err == nil {
			_ = sh.Redis.Expire(ctx, k, 70*time.Second).Err()
			if n > int64(sh.HoldsPerMin) {
				return false
			}
		}
	}
	if sh.LiveHoldsPer > 0 {
		k, _ := redisx.PlatformKey("rl", "live", who)
		_ = sh.Redis.ZRemRangeByScore(ctx, k, "-inf", strconv.FormatInt(now.UnixMilli(), 10)).Err()
		if n, err := sh.Redis.ZCard(ctx, k).Result(); err == nil && n >= int64(sh.LiveHoldsPer) {
			return false
		}
	}
	return true
}

func (s *Server) holdTaken(ctx context.Context, o *checkout.Order) {
	sh := s.Shop
	who := clientKey(ctx)
	if sh.Redis == nil || who == "" || sh.LiveHoldsPer <= 0 {
		return
	}
	k, _ := redisx.PlatformKey("rl", "live", who)
	_ = sh.Redis.ZAdd(ctx, k, redis.Z{Score: float64(o.HoldExpiresAt.UnixMilli()), Member: o.ID}).Err()
	_ = sh.Redis.Expire(ctx, k, time.Hour).Err()
}

// raw answers any strict operation with a status and a JSON body (or a problem document).
type raw struct {
	status  int
	body    any
	problem bool
	// retryAfter sets Retry-After (seconds) on a problem (EPS-BUSY: step 6).
	retryAfter int
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
	if r.retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(r.retryAfter))
	}
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
func (r raw) VisitCreatePayCodeResponse(w http.ResponseWriter) error          { return r.write(w) }

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
	out := raw{status: p.Status, body: doc, problem: true}
	if n, ok := p.Data["retryAfter"].(int); ok && n > 0 {
		out.retryAfter = n
	}
	return out
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
		b := checkout.Buyer{ID: c.Subject, Kind: "guest"}
		if c.Sim {
			b.Sim = &checkout.SimBuyer{Rehearsal: c.Rehearsal, Run: c.Run}
		}
		return b, true
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
	TicketsToken    string                 `json:"ticketsToken,omitempty"`
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
	accessLink        = "link"      // a live resume link
	accessViewLink    = "view_link" // a guest's 30-day order link
	accessExpiredLink = "expired_link"
)

// ExpiredLinkLife is how long an expired resume link still shows the order's state.
const ExpiredLinkLife = 30 * 24 * time.Hour

// orderFor loads an order the caller may see: its buyer, or a valid order link. Anything else,
// a changed link included, is "Order not found" (review D-f: an order's existence is never
// confirmed to a stranger).
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
	until, err := links.Check(token, o.ID, checkout.LinkResume)
	switch {
	case err == nil:
		return o, accessLink, nil
	case errors.Is(err, checkout.ErrLinkExpired) && time.Since(until) < ExpiredLinkLife:
		return o, accessExpiredLink, nil
	}
	if _, err := links.Check(token, o.ID, checkout.LinkView); err == nil {
		return o, accessViewLink, nil
	}
	return nil, "", checkout.ErrNotFound
}

// viewOf builds the order view in one read transaction (review D-k). An expired resume link sees
// the state only: no name, email, answers or history (review D-f).
func (s *Server) viewOf(ctx context.Context, o *checkout.Order, access string) OrderView {
	sh := s.Shop
	v := OrderView{Order: o, ServerTime: time.Now().UTC(), Access: access, Questions: []checkout.Question{}, SavedCards: []payments.SavedMethod{}}
	if access == accessExpiredLink {
		red := *o
		red.BuyerEmail, red.BuyerName, red.Answers, red.History = nil, nil, map[string]any{}, []checkout.HistoryEntry{}
		v.Order = &red
		return v
	}
	if o.OTPRequired && s.OnSale != nil && s.OnSale.Gate != nil && o.OTPVerified == nil {
		o.OTPRequired = s.OnSale.Gate.OTPNeeded(ctx, o) // the organiser may have switched OTP off
	}
	live := o.State == checkout.Held || o.State == checkout.PaymentFailed || o.State == checkout.Paying
	holdLive := o.HoldExpiresAt.After(time.Now())
	v.CanPay = (o.State == checkout.Held || o.State == checkout.PaymentFailed) && holdLive && (access == accessBuyer || access == accessLink)
	snap, err := s.Snapshots.Load(ctx, o.EventID)
	if err != nil {
		snap, _ = s.Snapshots.LoadVersion(ctx, o.EventID, o.SnapshotVersion)
	}
	b, isBuyer := buyerOf(ctx)
	account := isBuyer && b.Kind == "account" && b.ID == o.BuyerID
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		if snap != nil {
			v.EventName = snap.Event.Name
			if qs, err := checkout.QuestionsTx(ctx, tx, snap); err == nil {
				v.Questions = qs
			}
			if v.CanPay && !o.Rehearsal {
				v.InstalmentOffer = checkout.OfferFor(snap, o, time.Now())
			}
		}
		if p, err := checkout.PlanTx(ctx, tx, o.ID); err == nil && p != nil {
			pv := &planView{Plan: p}
			if p.SavedMethodID != nil {
				pv.SavedCardLabel = payments.SavedLabelTx(ctx, tx, *p.SavedMethodID)
			}
			v.Plan = pv
		}
		if a, err := payments.LatestAttemptTx(ctx, tx, o.ID); err == nil && a != nil {
			v.Payment = a
		}
		if account && !o.Rehearsal { // a rehearsal offers no wallet or saved card (review D-a)
			if w, err := wallet.WalletTx(ctx, tx, o.OrganiserID, b.ID, o.Currency); err == nil {
				v.Wallet = &walletOption{BalanceMinor: w.BalanceMinor, CreditMinor: w.CreditMinor}
			}
			if cards, err := payments.SavedMethodsTx(ctx, tx, b.ID); err == nil {
				v.SavedCards = cards
			}
		}
		return nil
	})
	if live && holdLive && (access == accessBuyer || access == accessLink) {
		v.ResumeToken = sh.Checkout.Links.ResumeLink(o)
	}
	if access == accessBuyer && o.BuyerKind == "guest" {
		v.ViewToken = sh.Checkout.Links.ViewLink(o)
	}
	// A guest's tickets open from a signed link once issued (step 7).
	if (access == accessBuyer || access == accessViewLink) && s.Tickets != nil && s.Tickets.Svc != nil {
		v.TicketsToken = s.Tickets.Svc.LinkFor(ctx, o)
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
	if !s.holdAllowed(ctx) {
		rateLimited.Inc()
		return problemRaw(ctx, apperr.New(http.StatusTooManyRequests, apperr.RateLimited, "Too many holds from this connection. Wait a minute.", true)), nil
	}
	b = withPass(b, req.Params.XEPSPass, req.Params.XEPSSession)
	o, existing, err := s.Shop.Checkout.CreateOrder(ctx, b, checkout.CreateRequest(*req.Body), key)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if !existing {
		s.holdTaken(ctx, o)
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
		if access == accessViewLink {
			return nil, "", checkout.ErrNotFound
		}
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

// returnURL keeps a provider return address only on an allowed web origin: the web app's exact
// origins, or https on one of the organiser's configured brand hostnames (review D-i).
func (s *Server) returnURL(ctx context.Context, org, raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(raw) > 2000 || u.User != nil {
		return "", false
	}
	origin := u.Scheme + "://" + u.Host
	for _, o := range s.Shop.ReturnOrigins {
		if o == origin {
			return raw, true
		}
	}
	if len(s.Shop.ReturnOrigins) == 0 && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1") {
		return raw, true
	}
	if u.Scheme != "https" || u.Port() != "" || s.Pool == nil {
		return "", false
	}
	var n int
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from control.brand_hostnames where hostname = $1", strings.ToLower(u.Hostname())).Scan(&n)
	})
	if n > 0 {
		return raw, true
	}
	return "", false
}

// brandName names the selling brand on the provider's page (review L7).
func (s *Server) brandName(ctx context.Context, o *checkout.Order) string {
	snap, err := s.Snapshots.LoadVersion(ctx, o.EventID, o.SnapshotVersion)
	if err != nil || s.Pool == nil {
		return ""
	}
	var name string
	_ = db.WithTx(ctx, s.Pool, db.RoleCore, o.OrganiserID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select name from control.brands where id = $1", snap.BrandID).Scan(&name)
	})
	return name
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
	// Step 6 (review D-a): a rehearsal never touches real money or a person's saved cards.
	if o.Rehearsal {
		if p := payments.RehearsalMethod(string(body.Method), body.SaveCard != nil && *body.SaveCard, body.Plan != nil && string(*body.Plan) == "instalments"); p != nil {
			return problemRaw(ctx, p), nil
		}
	}
	// Step 5 (D-11): the order's pass from the same session, then the one-time code if it needs one.
	if s.OnSale != nil && s.OnSale.Gate != nil {
		if err := s.OnSale.Gate.BeforePay(ctx, o, withPass(b, req.Params.XEPSPass, req.Params.XEPSSession), access == accessLink); err != nil {
			return s.fail(ctx, err), nil
		}
	}
	if access == accessLink {
		// A link holder pays by card, Apple Pay or Google Pay only; the wallet and saved cards need
		// the buyer's own session (review D-e). The payment acts for the order's buyer.
		switch string(body.Method) {
		case payments.HostedCard, payments.ApplePay, payments.GooglePay:
		default:
			return problemRaw(ctx, apperr.New(http.StatusUnprocessableEntity, apperr.PayMethod, "Sign in as the buyer to pay with the wallet or a saved card.", false)), nil
		}
		b = checkout.Buyer{ID: o.BuyerID, Kind: o.BuyerKind}
	}
	if string(body.Method) == "wallet" {
		paid, err := s.Shop.Wallet.PayOrder(ctx, b, o, req.Params.IdempotencyKey)
		if err != nil {
			return s.fail(ctx, err), nil
		}
		return okRaw(map[string]any{"order": s.viewOf(ctx, paid, access)}), nil
	}
	ret, allowed := s.returnURL(ctx, o.OrganiserID, token(body.ReturnUrl))
	if !allowed {
		return problemRaw(ctx, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "returnUrl is not on an allowed origin.", false)), nil
	}
	pr := payments.PayRequest{Method: string(body.Method), ReturnURL: ret, MerchantName: s.brandName(ctx, o)}
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
	if err := s.Shop.Wallet.CheckOrganiser(ctx, req.OrganiserId.String(), cur); err != nil {
		return s.fail(ctx, err), nil
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
	ret, allowed := s.returnURL(ctx, req.OrganiserId.String(), token(req.Body.ReturnUrl))
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
	w, err := s.Shop.Wallet.Stand(ctx, org, req.Body.Code, req.Body.PayCode, req.Body.AmountMinor, label, req.Params.IdempotencyKey)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(w), nil
}

func (s *Server) CreatePayCode(ctx context.Context, req api.CreatePayCodeRequestObject) (api.CreatePayCodeResponseObject, error) {
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
	wc, pc, exp, err := s.Shop.Wallet.PayCode(ctx, req.OrganiserId.String(), b.ID, cur)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(map[string]any{"walletCode": wc, "payCode": pc, "expiresAt": exp.UTC()}), nil
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

// ReturnURLAllowed reports whether a provider return address is allowed (tests).
func (s *Server) ReturnURLAllowed(ctx context.Context, org, raw string) bool {
	_, ok := s.returnURL(ctx, org, raw)
	return ok && raw != ""
}
