package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/danielgtaylor/huma/v2"
)

// Closed RFC 7807 problem-type catalog (docs/design-identity-endpoints.md §1.1).
const (
	TypeValidation           = "urn:canchalibre:error:validation"
	TypeUnauthenticated      = "urn:canchalibre:error:unauthenticated"
	TypeRefreshReuse         = "urn:canchalibre:error:refresh-reuse"
	TypeForbidden            = "urn:canchalibre:error:forbidden"
	TypeNotFound             = "urn:canchalibre:error:not-found"
	TypeConflict             = "urn:canchalibre:error:conflict"
	TypeLastCoOwner          = "urn:canchalibre:error:last-co-owner"
	TypeInvitationConsumed   = "urn:canchalibre:error:invitation-consumed"
	TypeInvitationExpired    = "urn:canchalibre:error:invitation-expired"
	TypeContextNotAuthorized = "urn:canchalibre:error:context-not-authorized"
	TypeMethodNotAllowed     = "urn:canchalibre:error:method-not-allowed"
	TypeUnsupportedMediaType = "urn:canchalibre:error:unsupported-media-type"
	TypeInternal             = "urn:canchalibre:error:internal"
)

// FieldError describes a single field-level validation error inside ProblemDetail.
type FieldError struct {
	Location string `json:"location,omitempty" example:"body.email" doc:"Request location of the invalid field (e.g. body.email or path.businessId)."`
	Message  string `json:"message" example:"invalid email address" doc:"Human-readable validation message for the field."`
}

// ProblemDetail represents an RFC 7807 response body without an instance field.
type ProblemDetail struct {
	Schema string        `json:"$schema,omitempty" readOnly:"true" example:"http://localhost:8080/schemas/ProblemDetail.json" doc:"URL of the problem JSON schema."`
	Type   string        `json:"type" example:"urn:canchalibre:error:validation" doc:"Stable URN identifying the problem type."`
	Title  string        `json:"title" example:"Bad Request" doc:"HTTP status text corresponding to status."`
	Status int           `json:"status" example:"400" doc:"HTTP status code."`
	Detail string        `json:"detail,omitempty" example:"Request validation failed." doc:"Human-readable explanation specific to this occurrence."`
	Errors []*FieldError `json:"errors,omitempty" example:"[{\"location\":\"body.email\",\"message\":\"invalid email address\"}]" doc:"Optional field-level validation errors."`
}

func (e *ProblemDetail) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return e.Title
}

func (e *ProblemDetail) GetStatus() int {
	return e.Status
}

func (e *ProblemDetail) ContentType(ct string) string {
	if ct == "application/json" {
		return "application/problem+json"
	}
	return ct
}

// DefaultTypeForStatus maps any HTTP status code to a catalog URN, failing closed
// to TypeInternal so about:blank can never be emitted.
func DefaultTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return TypeValidation
	case http.StatusUnauthorized:
		return TypeUnauthenticated
	case http.StatusForbidden:
		return TypeForbidden
	case http.StatusNotFound:
		return TypeNotFound
	case http.StatusMethodNotAllowed:
		return TypeMethodNotAllowed
	case http.StatusConflict:
		return TypeConflict
	case http.StatusUnsupportedMediaType:
		return TypeUnsupportedMediaType
	default:
		return TypeInternal
	}
}

var installErrorOverrideOnce sync.Once

// InstallErrorOverride sets huma.NewError to our total, fail-closed problem mapper.
func InstallErrorOverride() {
	installErrorOverrideOnce.Do(func() {
		huma.NewError = NewStatusError
		huma.NewErrorWithContext = func(_ huma.Context, status int, msg string, errs ...error) huma.StatusError {
			return NewStatusError(status, msg, errs...)
		}
	})
}

// NewStatusError builds a ProblemDetail using the default catalog URN for status.
func NewStatusError(status int, msg string, errs ...error) huma.StatusError {
	return NewTypedError(status, DefaultTypeForStatus(status), msg, errs...)
}

// NewTypedError builds a ProblemDetail with an explicit catalog URN.
func NewTypedError(status int, problemType, msg string, errs ...error) huma.StatusError {
	if problemType == "" || problemType == "about:blank" {
		problemType = DefaultTypeForStatus(status)
	}
	details := make([]*FieldError, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		if d, ok := err.(huma.ErrorDetailer); ok {
			ed := d.ErrorDetail()
			details = append(details, &FieldError{
				Location: ed.Location,
				Message:  ed.Message,
			})
		} else {
			details = append(details, &FieldError{Message: err.Error()})
		}
	}
	title := http.StatusText(status)
	if title == "" {
		title = "Internal Server Error"
	}
	return &ProblemDetail{
		Type:   problemType,
		Title:  title,
		Status: status,
		Detail: msg,
		Errors: details,
	}
}

// WriteProblem writes an RFC 7807 ProblemDetail directly to an http.ResponseWriter
// (used by Chi NotFound/MethodNotAllowed fallbacks).
func WriteProblem(w http.ResponseWriter, status int, problemType, detail string) {
	if problemType == "" || problemType == "about:blank" {
		problemType = DefaultTypeForStatus(status)
	}
	title := http.StatusText(status)
	if title == "" {
		title = "Internal Server Error"
	}
	body := ProblemDetail{
		Schema: fmt.Sprintf("%s/schemas/ProblemDetail.json", APIServerURL),
		Type:   problemType,
		Title:  title,
		Status: status,
		Detail: detail,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Link", `</schemas/ProblemDetail.json>; rel="describedBy"`)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ErrorResponseSpec describes a documented error response for an operation.
type ErrorResponseSpec struct {
	Status      int
	Type        string
	Description string
	Detail      string
}

// BuildErrorResponses produces explicit huma Responses entries with per-error
// Examples so the OpenAPI 3.1 contract publishes every documented URN and
// satisfies oas3-missing-example.
func BuildErrorResponses(api huma.API, specs ...ErrorResponseSpec) map[string]*huma.Response {
	registry := api.OpenAPI().Components.Schemas
	var problemSchema *huma.Schema
	if registry != nil {
		problemSchema = registry.Schema(reflectTypeProblemDetail, true, "ProblemDetail")
		DescribeSchema(api, "ProblemDetail", "RFC 7807 problem detail carrying a stable CanchaLibre error type URN.")
		DescribeSchema(api, "FieldError", "Field-level validation error entry inside an RFC 7807 problem detail.")
	}
	out := make(map[string]*huma.Response, len(specs))
	for _, s := range specs {
		codeStr := strconv.Itoa(s.Status)
		title := http.StatusText(s.Status)
		desc := s.Description
		if desc == "" {
			desc = title
		}
		detail := s.Detail
		if detail == "" {
			detail = desc
		}
		example := map[string]any{
			"$schema": APIServerURL + "/schemas/ProblemDetail.json",
			"type":    s.Type,
			"title":   title,
			"status":  s.Status,
			"detail":  detail,
		}
		if existing, ok := out[codeStr]; ok {
			mt := existing.Content["application/problem+json"]
			if mt != nil {
				if mt.Examples == nil {
					mt.Examples = map[string]*huma.Example{}
					if mt.Example != nil {
						mt.Examples["default"] = &huma.Example{Value: mt.Example}
						mt.Example = nil
					}
				}
				mt.Examples[s.Type] = &huma.Example{
					Summary: desc,
					Value:   example,
				}
			}
			continue
		}
		out[codeStr] = &huma.Response{
			Description: desc,
			Content: map[string]*huma.MediaType{
				"application/problem+json": {
					Schema:  problemSchema,
					Example: example,
				},
			},
		}
	}
	return out
}

// RefusalAuditor records a single append-only REFUSED row in schema_identity.identity_audit
// before an HTTP error response is flushed to the wire.
type RefusalAuditor interface {
	RecordHTTPRejAudit(ctx context.Context, method, path string, status int, problemType, detail string, actorAccountID *string)
}

type actorHolderKey struct{}

// SetRequestActorID records the authenticated account ID in the request-scoped
// holder so the outer refusal-audit middleware can attribute 403/404/409 refusals.
func SetRequestActorID(ctx context.Context, accountID string) {
	if ptr, ok := ctx.Value(actorHolderKey{}).(*string); ok && ptr != nil {
		*ptr = accountID
	}
}

// RefusalAuditMiddleware intercepts HTTP responses with status >= 400 and records
// exactly one REFUSED row in schema_identity.identity_audit synchronously before
// writing the response to the network connection.
func RefusalAuditMiddleware(auditor RefusalAuditor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if auditor == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var actorID string
			ctx := context.WithValue(r.Context(), actorHolderKey{}, &actorID)
			bw := &bufferedResponseWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
			}
			next.ServeHTTP(bw, r.WithContext(ctx))

			if bw.status >= 400 {
				var prob ProblemDetail
				_ = json.Unmarshal(bw.buf.Bytes(), &prob)
				pType := prob.Type
				if pType == "" {
					pType = DefaultTypeForStatus(bw.status)
				}
				detail := prob.Detail
				if detail == "" {
					detail = http.StatusText(bw.status)
				}
				var actorPtr *string
				if actorID != "" {
					actorPtr = &actorID
				}
				auditor.RecordHTTPRejAudit(r.Context(), r.Method, r.URL.Path, bw.status, pType, detail, actorPtr)
			}
			bw.flush()
		})
	}
}

type bufferedResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	buf         bytes.Buffer
}

func (b *bufferedResponseWriter) WriteHeader(code int) {
	if !b.wroteHeader {
		b.status = code
		b.wroteHeader = true
	}
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
	return b.buf.Write(p)
}

func (b *bufferedResponseWriter) flush() {
	if b.wroteHeader {
		b.ResponseWriter.WriteHeader(b.status)
	}
	if b.buf.Len() > 0 {
		_, _ = b.ResponseWriter.Write(b.buf.Bytes())
	}
}
