package coreapi

import (
	"context"
	"errors"
	"net/http"
	"sort"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

func problemFor(ctx context.Context, status int, code, detail string) api.Problem {
	p := api.Problem{Type: "https://eps.dev/problems/" + code, Title: http.StatusText(status), Status: status, Code: code, RequestId: httpx.RequestID(ctx)}
	if detail != "" {
		p.Detail = &detail
	}
	return p
}

// organiser returns the organiser of an organiser token, or a problem status.
func organiser(ctx context.Context) (string, int) {
	c, ok := auth.FromContext(ctx)
	if !ok {
		return "", http.StatusUnauthorized
	}
	if c.Kind != "organiser" || c.Org == "" {
		return "", http.StatusForbidden
	}
	return c.Org, 0
}

var errUnavailable = errors.New("snapshots not configured")

func (s *Server) ActivateSnapshot(ctx context.Context, req api.ActivateSnapshotRequestObject) (api.ActivateSnapshotResponseObject, error) {
	org, st := organiser(ctx)
	switch st {
	case http.StatusUnauthorized:
		return api.ActivateSnapshot401ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: api.ProblemApplicationProblemPlusJSONResponse(problemFor(ctx, st, "unauthenticated", ""))}, nil
	case http.StatusForbidden:
		return api.ActivateSnapshot403ApplicationProblemPlusJSONResponse(problemFor(ctx, st, "forbidden", "An organiser token is required.")), nil
	}
	if s.Snapshots == nil {
		return api.ActivateSnapshotdefaultApplicationProblemPlusJSONResponse{Body: problemFor(ctx, http.StatusServiceUnavailable, "unavailable", ""), StatusCode: http.StatusServiceUnavailable}, nil
	}
	sha, err := s.Snapshots.Activate(ctx, org, req.EventId.String(), req.Version)
	if errors.Is(err, snapshot.ErrNotFound) {
		return api.ActivateSnapshot404ApplicationProblemPlusJSONResponse(problemFor(ctx, http.StatusNotFound, "not_found", "")), nil
	}
	if err != nil {
		return nil, err
	}
	return api.ActivateSnapshot200JSONResponse{EventId: req.EventId, Version: req.Version, Sha256: sha}, nil
}

func (s *Server) GetPoolUsage(ctx context.Context, req api.GetPoolUsageRequestObject) (api.GetPoolUsageResponseObject, error) {
	org, st := organiser(ctx)
	switch st {
	case http.StatusUnauthorized:
		return api.GetPoolUsage401ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: api.ProblemApplicationProblemPlusJSONResponse(problemFor(ctx, st, "unauthenticated", ""))}, nil
	case http.StatusForbidden:
		return api.GetPoolUsage403ApplicationProblemPlusJSONResponse(problemFor(ctx, st, "forbidden", "An organiser token is required.")), nil
	}
	if s.Snapshots == nil {
		return api.GetPoolUsagedefaultApplicationProblemPlusJSONResponse{Body: problemFor(ctx, http.StatusServiceUnavailable, "unavailable", ""), StatusCode: http.StatusServiceUnavailable}, nil
	}
	// An event that was published belongs to the organiser in its snapshot; another organiser's
	// event is 404. A never-published event has no usage anywhere, so the answer is empty.
	owner, err := s.Snapshots.OrganiserOf(ctx, req.EventId.String())
	if err != nil {
		return nil, err
	}
	if owner != "" && owner != org {
		return api.GetPoolUsage404ApplicationProblemPlusJSONResponse(problemFor(ctx, http.StatusNotFound, "not_found", "")), nil
	}
	usage := s.Usage
	if usage == nil {
		usage = snapshot.NoUsage{}
	}
	used, err := usage.PoolUsage(ctx, org, req.EventId.String())
	if err != nil {
		return nil, err
	}
	out := api.PoolUsage{EventId: req.EventId, Instances: []struct {
		InstanceId string `json:"instanceId"`
		Used       int64  `json:"used"`
	}{}}
	ids := make([]string, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if used[id] > 0 {
			out.Instances = append(out.Instances, struct {
				InstanceId string `json:"instanceId"`
				Used       int64  `json:"used"`
			}{id, used[id]})
		}
	}
	return api.GetPoolUsage200JSONResponse(out), nil
}

func (s *Server) GetAvailability(ctx context.Context, req api.GetAvailabilityRequestObject) (api.GetAvailabilityResponseObject, error) {
	if s.Availability == nil {
		return api.GetAvailabilitydefaultApplicationProblemPlusJSONResponse{Body: problemFor(ctx, http.StatusServiceUnavailable, "unavailable", ""), StatusCode: http.StatusServiceUnavailable}, nil
	}
	a, err := s.Availability.Get(ctx, req.EventId.String())
	if errors.Is(err, snapshot.ErrNotFound) {
		return api.GetAvailability404ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: api.ProblemApplicationProblemPlusJSONResponse(problemFor(ctx, http.StatusNotFound, "not_found", ""))}, nil
	}
	if err != nil {
		return nil, err
	}
	out := api.Availability{EventId: req.EventId, Version: a.Version, GeneratedAt: a.GeneratedAt, Products: make([]api.ProductAvailability, 0, len(a.Products)), Instances: make([]api.InstanceAvailability, 0, len(a.Instances))}
	for _, in := range a.Instances {
		out.Instances = append(out.Instances, api.InstanceAvailability{InstanceId: in.InstanceID, Status: api.InstanceAvailabilityStatus(in.Status), Remaining: in.Remaining})
	}
	for _, p := range a.Products {
		var id openapi_types.UUID
		_ = id.UnmarshalText([]byte(p.ProductID))
		out.Products = append(out.Products, api.ProductAvailability{
			ProductId: id, Status: api.ProductAvailabilityStatus(p.Status), OpensAt: p.OpensAt,
			PriceStepIndex: p.PriceStepIndex, AllInMinor: p.AllInMinor, Remaining: p.Remaining,
		})
	}
	cc := "public, max-age=2"
	return api.GetAvailability200JSONResponse{Body: out, Headers: api.GetAvailability200ResponseHeaders{CacheControl: &cc}}, nil
}
