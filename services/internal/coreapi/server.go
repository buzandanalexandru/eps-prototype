// Package coreapi implements the core's contract operations and wires the HTTP stack:
// request id → recover → access log → CORS → token verification → contract validation → handlers.
package coreapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/jackc/pgx/v5/pgxpool"
	middleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/version"
)

// Server implements api.StrictServerInterface.
type Server struct {
	Service string
	Pool    *pgxpool.Pool // may be nil (version then omits schema versions)
	Checks  []health.Checker
	// Sale snapshots (step 3): nil in contract tests without a database.
	Snapshots    *snapshot.Store
	Availability *snapshot.AvailabilityService
	Usage        snapshot.Usage
}

var _ api.StrictServerInterface = (*Server)(nil)

func (s *Server) GetHealthz(ctx context.Context, _ api.GetHealthzRequestObject) (api.GetHealthzResponseObject, error) {
	return api.GetHealthz200JSONResponse{Status: "ok"}, nil
}

func (s *Server) GetReadyz(ctx context.Context, req api.GetReadyzRequestObject) (api.GetReadyzResponseObject, error) {
	verbose := req.Params.Verbose != nil && *req.Params.Verbose
	res, ok := health.Ready(ctx, s.Checks, verbose)
	if !ok {
		return api.GetReadyz503JSONResponse(res), nil
	}
	return api.GetReadyz200JSONResponse(res), nil
}

func (s *Server) GetVersion(ctx context.Context, _ api.GetVersionRequestObject) (api.GetVersionResponseObject, error) {
	out := api.Version{Service: s.Service, Commit: version.Commit}
	if version.BuiltAt != "" {
		b := version.BuiltAt
		out.BuiltAt = &b
	}
	if s.Pool != nil {
		if v, err := db.SchemaVersion(ctx, s.Pool, db.Control); err == nil {
			out.Schemas.Control = &v
		}
		if v, err := db.SchemaVersion(ctx, s.Pool, db.Sales); err == nil {
			out.Schemas.Sales = &v
		}
	}
	return api.GetVersion200JSONResponse(out), nil
}

func (s *Server) GetWhoami(ctx context.Context, _ api.GetWhoamiRequestObject) (api.GetWhoamiResponseObject, error) {
	c, ok := auth.FromContext(ctx)
	if !ok { // the validator already refuses this; kept as a second line
		return nil, errUnauthenticated
	}
	id := api.Identity{Sub: c.Subject, Kind: api.IdentityKind(c.Kind), Sid: c.Sid}
	if c.ExpiresAt != nil {
		id.Exp = c.ExpiresAt.Unix()
	}
	if c.Org != "" {
		org := c.Org
		id.Org = &org
	}
	if c.Role != "" {
		role := api.IdentityRole(c.Role)
		id.Role = &role
	}
	return api.GetWhoami200JSONResponse(id), nil
}

var errUnauthenticated = errors.New("unauthenticated")

// Options configure the HTTP stack.
type Options struct {
	Log      *slog.Logger
	Verifier *auth.Verifier
	CORS     *httpx.CORS
	// Extra routes mounted beside the contract (e.g. organiser APIs added by later steps).
	Extra func(mux *http.ServeMux)
}

// NewHandler builds the core's HTTP handler from the contract.
func NewHandler(s *Server, o Options) (http.Handler, error) {
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, err
	}
	spec.Servers = nil // validate paths regardless of host

	strict := api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errUnauthenticated) {
				httpx.WriteProblem(w, r, http.StatusUnauthorized, "unauthenticated", "")
				return
			}
			o.Log.Error("handler error", "error", err.Error(), "requestId", httpx.RequestID(r.Context()))
			httpx.WriteProblem(w, r, http.StatusInternalServerError, "internal", "")
		},
	})
	mux := http.NewServeMux()
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		}})
	if o.Extra != nil {
		o.Extra(mux)
	}

	validator := middleware.OapiRequestValidatorWithOptions(spec, &middleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: func(ctx context.Context, in *openapi3filter.AuthenticationInput) error {
				if _, ok := auth.FromContext(in.RequestValidationInput.Request.Context()); ok {
					return nil
				}
				return errUnauthenticated
			},
			MultiError: false,
		},
		// Routes outside the contract (Extra) carry their own guards.
		SilenceServersWarning: true,
		DoNotValidateServers:  true,
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts middleware.ErrorHandlerOpts) {
			status := opts.StatusCode
			var secErr *openapi3filter.SecurityRequirementsError
			if errors.As(err, &secErr) {
				status = http.StatusUnauthorized
			}
			if errors.Is(err, routers.ErrMethodNotAllowed) {
				status = http.StatusMethodNotAllowed
			}
			if status == http.StatusNotFound && o.Extra != nil {
				// Not a contract route: let the mux decide (it 404s with problem+json below).
				mux.ServeHTTP(w, r)
				return
			}
			detail := ""
			if status == http.StatusBadRequest {
				detail = err.Error()
			}
			if status == http.StatusUnauthorized {
				detail = "A valid core token is required."
			}
			httpx.WriteProblem(w, r, status, httpx.CodeForStatus(status), detail)
		},
	})

	h := httpx.Chain(mux,
		httpx.WithRequestID,
		httpx.WithRecover(o.Log),
		httpx.WithAccessLog(o.Log, s.Service),
		o.CORS.Middleware,
		auth.Middleware(o.Verifier, o.Log),
		validator,
	)
	return h, nil
}
