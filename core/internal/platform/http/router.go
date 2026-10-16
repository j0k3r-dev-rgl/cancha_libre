package http

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

var reflectTypeProblemDetail = reflect.TypeOf(ProblemDetail{})

// APIVersion is the version reported by the generated OpenAPI 3.1 contract.
const APIVersion = "0.1.0"

// APITitle is the title reported by the generated OpenAPI 3.1 contract.
const APITitle = "CanchaLibre API"

// APIDescription is the description reported by the generated OpenAPI 3.1 contract.
const APIDescription = "Backend API for CanchaLibre: accounts, organization, " +
	"inventory, booking, payments, discovery and notifications for sports-facility " +
	"booking."

// Tags declared at the contract root.
const (
	TagSystem       = "system"
	TagAuth         = "auth"
	TagAccount      = "account"
	TagAdmin        = "admin"
	TagOrganization = "organization"
)

// APIServerURL is the base URL the contract declares.
const APIServerURL = "http://localhost:8080"

// NewRouter constructs the Chi router, installs the RFC 7807 error override and
// NotFound/MethodNotAllowed fallbacks, registers /health and /api/v1/ping, and
// returns both the Chi mux and the huma.API instance so modules can register
// their operations.
func NewRouter() (*chi.Mux, huma.API) {
	InstallErrorOverride()

	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		WriteProblem(w, http.StatusNotFound, TypeNotFound, "The requested resource was not found.")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		WriteProblem(w, http.StatusMethodNotAllowed, TypeMethodNotAllowed, "The HTTP method is not allowed for this resource.")
	})

	api := humachi.New(r, buildHumaConfig())

	registerHealth(api)
	registerPing(api)

	DescribeSchema(api, "HealthOutputBody", "Body of the liveness check: the fixed liveness status plus the server time in UTC.")
	DescribeSchema(api, "PingOutputBody", "Body of the connectivity check: the fixed acknowledgement returned when the API is reachable.")

	return r, api
}

func buildHumaConfig() huma.Config {
	cfg := huma.DefaultConfig(APITitle, APIVersion)
	cfg.Info.Description = APIDescription
	cfg.Tags = []*huma.Tag{
		{Name: TagSystem, Description: "Liveness and connectivity probes of the API process."},
		{Name: TagAuth, Description: "Authentication, session refresh, invitation activation, password recovery and logout."},
		{Name: TagAccount, Description: "Authenticated actor profile and active business/branch context selection."},
		{Name: TagAdmin, Description: "Platform administrator operations for business co-owners and support operators."},
		{Name: TagOrganization, Description: "Business co-owner operations for memberships and work invitations."},
	}
	cfg.Servers = []*huma.Server{
		{URL: APIServerURL},
	}
	return cfg
}

// DescribeSchema attaches a description to a published component schema.
func DescribeSchema(api huma.API, name, description string) {
	schema := api.OpenAPI().Components.Schemas.SchemaFromRef("#/components/schemas/" + name)
	if schema != nil {
		schema.Description = description
	}
}

type healthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Fixed liveness status."`
		Time   string `json:"time" example:"2026-01-01T00:00:00Z" doc:"Server time in UTC when the check ran."`
	}
}

func registerHealth(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Liveness check",
		Description: "Reports that the API process is running.",
		Tags:        []string{TagSystem},
		Responses: BuildErrorResponses(api, ErrorResponseSpec{
			Status:      http.StatusInternalServerError,
			Type:        TypeInternal,
			Description: "Unexpected internal server error.",
		}),
	}, func(_ context.Context, _ *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		out.Body.Time = time.Now().UTC().Format(time.RFC3339)
		return out, nil
	})
}

type pingOutput struct {
	Body struct {
		Message string `json:"message" example:"pong" doc:"Fixed acknowledgement returned by the connectivity check."`
	}
}

func registerPing(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "ping",
		Method:      http.MethodGet,
		Path:        "/api/v1/ping",
		Summary:     "Connectivity check",
		Description: "Returns a fixed acknowledgement when the API is reachable.",
		Tags:        []string{TagSystem},
		Responses: BuildErrorResponses(api, ErrorResponseSpec{
			Status:      http.StatusInternalServerError,
			Type:        TypeInternal,
			Description: "Unexpected internal server error.",
		}),
	}, func(_ context.Context, _ *struct{}) (*pingOutput, error) {
		out := &pingOutput{}
		out.Body.Message = "pong"
		return out, nil
	})
}
