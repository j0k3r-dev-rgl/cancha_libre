package http

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// APIVersion is the version reported by the generated OpenAPI 3.1 contract.
const APIVersion = "0.1.0"

// APITitle is the title reported by the generated OpenAPI 3.1 contract.
const APITitle = "CanchaLibre API"

// APIDescription is the description reported by the generated OpenAPI 3.1
// contract.
const APIDescription = "Backend API for CanchaLibre: accounts, organization, " +
	"inventory, booking, payments, discovery and notifications for sports-facility " +
	"booking."

// systemTag groups the operational probes described by this file. Every tag an
// operation uses must also be declared in the contract root.
const systemTag = "system"

// systemTagDescription explains the system tag in the contract.
const systemTagDescription = "Liveness and connectivity probes of the API process."

// APIServerURL is the base URL the contract declares. There is no public domain
// yet, so it documents the local development server that .pi/api.json targets.
// It must be revisited when a real deployment URL exists.
const APIServerURL = "http://localhost:8080"

// NewRouter constructs the Chi router with the standard middlewares and the
// huma API that generates the OpenAPI 3.1 contract from the operations
// registered below.
//
// The contract is served at /openapi.json, the schema documents at /schemas and
// the interactive documentation at /docs. Every documented operation declares
// its success response and the error responses it may return, using huma's
// RFC 7807 compatible error model.
func NewRouter() *chi.Mux {
	r := chi.NewRouter()

	// Standard production middlewares
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	api := humachi.New(r, config())

	registerHealth(api)
	registerPing(api)

	describeSchema(api, "HealthOutputBody", "Body of the liveness check: the fixed liveness status plus the server time in UTC.")
	describeSchema(api, "PingOutputBody", "Body of the connectivity check: the fixed acknowledgement returned when the API is reachable.")

	return r
}

// config builds the huma configuration used by the router. It keeps huma's
// defaults and completes the contract information the default configuration
// leaves empty: the API description and the global tag list.
func config() huma.Config {
	cfg := huma.DefaultConfig(APITitle, APIVersion)
	cfg.Info.Description = APIDescription
	cfg.Tags = []*huma.Tag{
		{Name: systemTag, Description: systemTagDescription},
	}
	cfg.Servers = []*huma.Server{
		{URL: APIServerURL},
	}
	return cfg
}

// describeSchema attaches a description to a published component schema. The
// registry only knows the type once the operation that returns it has been
// registered, so this runs after registration.
func describeSchema(api huma.API, name, description string) {
	schema := api.OpenAPI().Components.Schemas.SchemaFromRef("#/components/schemas/" + name)
	if schema != nil {
		schema.Description = description
	}
}

// healthOutput is the documented body of the liveness check.
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
		Tags:        []string{systemTag},
		Errors:      []int{http.StatusInternalServerError},
	}, func(_ context.Context, _ *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		out.Body.Time = time.Now().UTC().Format(time.RFC3339)
		return out, nil
	})
}

// pingOutput is the documented body of the connectivity check.
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
		Tags:        []string{systemTag},
		Errors:      []int{http.StatusInternalServerError},
	}, func(_ context.Context, _ *struct{}) (*pingOutput, error) {
		out := &pingOutput{}
		out.Body.Message = "pong"
		return out, nil
	})
}
