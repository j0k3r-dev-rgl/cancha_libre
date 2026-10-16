package http

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RequestTimeout is the per-request deadline enforced by the timeout middleware.
const RequestTimeout = 30 * time.Second

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

// RouterOptions configures the platform router. The zero value builds the router
// with the default logger, no request recording and no private scrape path.
type RouterOptions struct {
	// Logger receives the server-side failure paths. Nil means slog.Default().
	Logger *slog.Logger

	// Recorder, when non-nil, installs the request recording middleware, so the
	// router reports what it served without knowing who measures it.
	Recorder RequestRecorder

	// Metrics, when non-nil, is mounted on MetricsPath as a raw GET route outside
	// the published contract. Nil keeps the path unrouted, which is how "no token
	// means no metrics endpoint" is realized: the caller simply passes no handler.
	Metrics http.Handler
}

// NewRouter constructs the Chi router, installs the RFC 7807 error override and
// NotFound/MethodNotAllowed fallbacks, registers /health and /api/v1/ping, and
// returns both the Chi mux and the huma.API instance so modules can register
// their operations. The platform audits nothing: the identity use cases are the
// single writer of REFUSED rows, because a transport-level duplicate was the F1
// defect. It does own the two server-side failure paths the access log alone could
// not describe: a recovered panic and an expired request deadline, both recorded
// through the logger it receives. Metrics are opt-in through RouterOptions: with
// no recorder there is no per-request cost, and with no metrics handler the
// private path does not exist.
func NewRouter(opts RouterOptions) (*chi.Mux, huma.API) {
	InstallErrorOverride()

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	if opts.Recorder != nil {
		r.Use(RecordRequests(opts.Recorder))
	}
	r.Use(RecoverPanic(logger))
	r.Use(chimiddleware.Timeout(RequestTimeout))
	r.Use(LogTimeouts(logger, RequestTimeout))

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		WriteProblem(w, http.StatusNotFound, TypeNotFound, "The requested resource was not found.")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		WriteProblem(w, http.StatusMethodNotAllowed, TypeMethodNotAllowed, "The HTTP method is not allowed for this resource.")
	})

	api := humachi.New(r, buildHumaConfig())

	registerHealth(api)
	registerPing(api)

	// The private scrape path is a raw router route on purpose: registering it
	// here, next to the platform routes, keeps it out of the huma contract every
	// client reads, and mounting it only when a handler was supplied is what makes
	// the endpoint absent rather than merely refused when metrics are disabled.
	if opts.Metrics != nil {
		r.Method(http.MethodGet, MetricsPath, opts.Metrics)
	}

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
