package coreapi

import (
	"context"
	"net/http"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/door"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

// Tickets holds the step 7 services: the holder's tickets and codes, and the door. Nil in contract
// tests without a database: every operation then answers 503.
type Tickets struct {
	Svc  *tickets.Service
	Door *door.Service
}

func (r raw) VisitGetTicketKeysResponse(w http.ResponseWriter) error      { return r.write(w) }
func (r raw) VisitGetMyTripResponse(w http.ResponseWriter) error          { return r.write(w) }
func (r raw) VisitGetMyTicketResponse(w http.ResponseWriter) error        { return r.write(w) }
func (r raw) VisitGetMyTicketCodesResponse(w http.ResponseWriter) error   { return r.write(w) }
func (r raw) VisitNameMyTicketResponse(w http.ResponseWriter) error       { return r.write(w) }
func (r raw) VisitCheckInMyTicketResponse(w http.ResponseWriter) error    { return r.write(w) }
func (r raw) VisitPickMyTicketDayResponse(w http.ResponseWriter) error    { return r.write(w) }
func (r raw) VisitEraseMyDataResponse(w http.ResponseWriter) error        { return r.write(w) }
func (r raw) VisitGetTicketLinkResponse(w http.ResponseWriter) error      { return r.write(w) }
func (r raw) VisitGetTicketLinkCodesResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitNameTicketLinkResponse(w http.ResponseWriter) error     { return r.write(w) }
func (r raw) VisitRegisterDoorDeviceResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitDoorSyncResponse(w http.ResponseWriter) error           { return r.write(w) }
func (r raw) VisitUploadDoorScansResponse(w http.ResponseWriter) error    { return r.write(w) }

func ticketsUnavailable(ctx context.Context) raw {
	return problemRaw(ctx, apperr.New(http.StatusServiceUnavailable, apperr.Unavailable, "Tickets are not configured.", true))
}

// holderOf is the caller as a ticket holder: an account (fan or organiser token) or a guest. A
// simulator token never reaches here (simGuard).
func holderOf(ctx context.Context) (tickets.Holder, bool) {
	c, ok := auth.FromContext(ctx)
	if !ok || c.Subject == "" || c.Sim {
		return tickets.Holder{}, false
	}
	if c.Kind == auth.KindGuest {
		return tickets.Holder{ID: c.Subject, Kind: "guest"}, true
	}
	return tickets.Holder{ID: c.Subject, Kind: "account"}, true
}

// staffOf is the caller as door staff (organiser tokens only).
func staffOf(ctx context.Context) (door.Staff, bool) {
	c, ok := auth.FromContext(ctx)
	if !ok || c.Kind != auth.KindOrganiser || c.Org == "" {
		return door.Staff{}, false
	}
	return door.Staff{Org: c.Org, Role: c.Role, Sub: c.Subject}, true
}

func (s *Server) ticketsReady(ctx context.Context) (raw, bool) {
	if s.Tickets == nil || s.Tickets.Svc == nil {
		return ticketsUnavailable(ctx), false
	}
	return raw{}, true
}

func (s *Server) GetTicketKeys(ctx context.Context, _ api.GetTicketKeysRequestObject) (api.GetTicketKeysResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	keys := s.Tickets.Svc.Keys.Public()
	if keys == nil {
		keys = []tickets.PublicKey{}
	}
	return okRaw(map[string]any{"serverTime": time.Now().UTC(), "windowSeconds": int(tickets.Window / time.Second), "keys": keys}), nil
}

func (s *Server) GetMyTrip(ctx context.Context, _ api.GetMyTripRequestObject) (api.GetMyTripResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	h, ok := holderOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	trip, err := s.Tickets.Svc.TripOf(ctx, h)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(trip), nil
}

func (s *Server) GetMyTicket(ctx context.Context, req api.GetMyTicketRequestObject) (api.GetMyTicketResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	h, ok := holderOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	e, err := s.Tickets.Svc.TicketFor(ctx, h, req.TicketId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(e), nil
}

func (s *Server) GetMyTicketCodes(ctx context.Context, req api.GetMyTicketCodesRequestObject) (api.GetMyTicketCodesResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	h, ok := holderOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	c, err := s.Tickets.Svc.CodesFor(ctx, h, req.TicketId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(c), nil
}

func (s *Server) NameMyTicket(ctx context.Context, req api.NameMyTicketRequestObject) (api.NameMyTicketResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	h, ok := holderOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	e, err := s.Tickets.Svc.Personalise(ctx, h, req.TicketId.String(), req.Body.Name)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(e), nil
}

func (s *Server) CheckInMyTicket(ctx context.Context, req api.CheckInMyTicketRequestObject) (api.CheckInMyTicketResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	h, ok := holderOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	payLate := req.Body != nil && req.Body.PayLate != nil && *req.Body.PayLate
	e, err := s.Tickets.Svc.CheckIn(ctx, h, req.TicketId.String(), payLate)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(e), nil
}

func (s *Server) PickMyTicketDay(ctx context.Context, req api.PickMyTicketDayRequestObject) (api.PickMyTicketDayResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	h, ok := holderOf(ctx)
	if !ok {
		return needBuyer(ctx), nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	e, err := s.Tickets.Svc.PickDay(ctx, h, req.TicketId.String(), req.Body.Day.Format("2006-01-02"))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(e), nil
}

func (s *Server) EraseMyData(ctx context.Context, _ api.EraseMyDataRequestObject) (api.EraseMyDataResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	c, ok := auth.FromContext(ctx)
	if !ok || c.Subject == "" || c.Sim || c.Kind == auth.KindGuest {
		return problemRaw(ctx, apperr.New(http.StatusForbidden, apperr.Forbidden, "Sign in to delete your data.", false)), nil
	}
	out, err := s.Tickets.Svc.Erase(ctx, tickets.Person{Sub: c.Subject, Email: c.Email})
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(out), nil
}

func (s *Server) GetTicketLink(ctx context.Context, req api.GetTicketLinkRequestObject) (api.GetTicketLinkResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	e, err := s.Tickets.Svc.LinkTrip(ctx, req.LinkToken)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(e), nil
}

func (s *Server) GetTicketLinkCodes(ctx context.Context, req api.GetTicketLinkCodesRequestObject) (api.GetTicketLinkCodesResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	c, err := s.Tickets.Svc.LinkCodes(ctx, req.LinkToken, req.TicketId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(c), nil
}

func (s *Server) NameTicketLink(ctx context.Context, req api.NameTicketLinkRequestObject) (api.NameTicketLinkResponseObject, error) {
	if r, ok := s.ticketsReady(ctx); !ok {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	e, err := s.Tickets.Svc.LinkPersonalise(ctx, req.LinkToken, req.TicketId.String(), req.Body.Name)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(e), nil
}

func (s *Server) doorReady(ctx context.Context) (door.Staff, raw, bool) {
	if s.Tickets == nil || s.Tickets.Door == nil {
		return door.Staff{}, ticketsUnavailable(ctx), false
	}
	st, ok := staffOf(ctx)
	if !ok {
		// Not organiser staff: the same answer as another organiser's event.
		return door.Staff{}, problemRaw(ctx, apperr.New(http.StatusNotFound, "EPS-EVENT-NOT-FOUND", "Event not found.", false)), false
	}
	return st, raw{}, true
}

func (s *Server) RegisterDoorDevice(ctx context.Context, req api.RegisterDoorDeviceRequestObject) (api.RegisterDoorDeviceResponseObject, error) {
	st, r, ok := s.doorReady(ctx)
	if !ok {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	d, err := s.Tickets.Door.Register(ctx, st, req.EventId.String(), req.Body.Gate)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return raw{status: http.StatusCreated, body: d}, nil
}

func (s *Server) DoorSync(ctx context.Context, req api.DoorSyncRequestObject) (api.DoorSyncResponseObject, error) {
	st, r, ok := s.doorReady(ctx)
	if !ok {
		return r, nil
	}
	sr := door.SyncRequest{}
	if req.Params.Since != nil {
		sr.Since = *req.Params.Since
	}
	if req.Params.Manifest != nil {
		sr.Manifest = *req.Params.Manifest
	}
	if req.Params.After != nil {
		sr.ManifestAfter = *req.Params.After
	}
	if req.Params.Device != nil {
		sr.DeviceID = *req.Params.Device
	}
	out, err := s.Tickets.Door.Sync(ctx, st, req.EventId.String(), sr)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(out), nil
}

func (s *Server) UploadDoorScans(ctx context.Context, req api.UploadDoorScansRequestObject) (api.UploadDoorScansResponseObject, error) {
	st, r, ok := s.doorReady(ctx)
	if !ok {
		return r, nil
	}
	if req.Body == nil {
		return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "A body is required.", false)), nil
	}
	scans := make([]door.Scan, 0, len(req.Body.Scans))
	for _, sc := range req.Body.Scans {
		scans = append(scans, door.Scan{ID: sc.Id.String(), TicketID: sc.TicketId.String(), Generation: sc.Generation, ScannedAt: sc.ScannedAt})
	}
	res, err := s.Tickets.Door.Upload(ctx, st, req.EventId.String(), req.Body.DeviceId.String(), scans)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(map[string]any{"results": res}), nil
}
