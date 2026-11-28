package coreapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
)

// Step 6: rehearsals, simulation runs, degraded mode (design D-1, D-2, D-9).

func (r raw) VisitSetSaleDegradedResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitListRehearsalsResponse(w http.ResponseWriter) error  { return r.write(w) }
func (r raw) VisitCreateRehearsalResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitResetRehearsalResponse(w http.ResponseWriter) error  { return r.write(w) }
func (r raw) VisitListSimulationsResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitStartSimulationResponse(w http.ResponseWriter) error { return r.write(w) }
func (r raw) VisitGetSimulationResponse(w http.ResponseWriter) error   { return r.write(w) }
func (r raw) VisitStopSimulationResponse(w http.ResponseWriter) error  { return r.write(w) }
func (r raw) VisitLiveStreamResponse(w http.ResponseWriter) error      { return r.write(w) }

func (s *Server) simReady(ctx context.Context) (raw, bool) {
	if s.OnSale == nil || s.Sim == nil {
		return unavailable(ctx), false
	}
	return raw{}, true
}

var readers = []string{"owner", "admin", "finance", "box_office", "staff"}

func (s *Server) SetSaleDegraded(ctx context.Context, req api.SetSaleDegradedRequestObject) (api.SetSaleDegradedResponseObject, error) {
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
	x, err := s.OnSale.Sales.SetDegraded(ctx, org, req.SaleId.String(), by(ctx), req.Body.Degraded)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(s.view(ctx, x, nil, true)), nil
}

func (s *Server) ListRehearsals(ctx context.Context, req api.ListRehearsalsRequestObject) (api.ListRehearsalsResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, readers...)
	if !ok {
		return r, nil
	}
	list, err := s.OnSale.Sales.Rehearsals(ctx, org, req.SaleId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	out := make([]saleView, 0, len(list))
	for i := range list {
		out = append(out, s.view(ctx, &list[i], nil, true))
	}
	l := s.Sim.Limits
	if l.MaxFans <= 0 {
		l = simulation.DefaultLimits
	}
	return okRaw(map[string]any{"rehearsals": out, "limits": l}), nil
}

func (s *Server) CreateRehearsal(ctx context.Context, req api.CreateRehearsalRequestObject) (api.CreateRehearsalResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin")
	if !ok {
		return r, nil
	}
	x, err := s.Sim.CreateRehearsal(ctx, org, req.SaleId.String(), by(ctx))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return raw{status: http.StatusCreated, body: s.view(ctx, x, nil, false)}, nil
}

func (s *Server) ResetRehearsal(ctx context.Context, req api.ResetRehearsalRequestObject) (api.ResetRehearsalResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin")
	if !ok {
		return r, nil
	}
	x, err := s.Sim.ResetRehearsal(ctx, org, req.SaleId.String(), by(ctx))
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return raw{status: http.StatusCreated, body: s.view(ctx, x, nil, false)}, nil
}

func (s *Server) ListSimulations(ctx context.Context, req api.ListSimulationsRequestObject) (api.ListSimulationsResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, readers...)
	if !ok {
		return r, nil
	}
	runs, err := s.Sim.List(ctx, org, req.SaleId.String(), 10)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	if runs == nil {
		runs = []simulation.Run{}
	}
	return okRaw(map[string]any{"runs": runs}), nil
}

func (s *Server) StartSimulation(ctx context.Context, req api.StartSimulationRequestObject) (api.StartSimulationResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin")
	if !ok {
		return r, nil
	}
	var k simulation.Knobs
	if req.Body != nil {
		b, _ := json.Marshal(req.Body)
		if err := json.Unmarshal(b, &k); err != nil {
			return problemRaw(ctx, apperr.New(http.StatusBadRequest, "invalid_request", "Unreadable knobs.", false)), nil
		}
	}
	run, err := s.Sim.Start(ctx, org, req.SaleId.String(), by(ctx), k)
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return raw{status: http.StatusCreated, body: run}, nil
}

func (s *Server) GetSimulation(ctx context.Context, req api.GetSimulationRequestObject) (api.GetSimulationResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, readers...)
	if !ok {
		return r, nil
	}
	run, err := s.Sim.Get(ctx, org, req.RunId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(run), nil
}

func (s *Server) StopSimulation(ctx context.Context, req api.StopSimulationRequestObject) (api.StopSimulationResponseObject, error) {
	if r, ok := s.simReady(ctx); !ok {
		return r, nil
	}
	org, r, ok := organiserRole(ctx, "owner", "admin")
	if !ok {
		return r, nil
	}
	run, err := s.Sim.Stop(ctx, org, req.RunId.String())
	if err != nil {
		return s.fail(ctx, err), nil
	}
	return okRaw(run), nil
}

// LiveStream is the gateway's operation; the core does not serve it.
func (s *Server) LiveStream(ctx context.Context, _ api.LiveStreamRequestObject) (api.LiveStreamResponseObject, error) {
	return problemRaw(ctx, apperr.New(http.StatusMisdirectedRequest, "EPS-WRONG-SERVICE", "The live stream is served by the gateway.", false)), nil
}
