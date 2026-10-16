package http

import (
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

// ProblemDetail represents an RFC 7807 response body without an instance field.
type ProblemDetail struct {
	Schema string              `json:"$schema,omitempty" readOnly:"true" doc:"URL of the problem JSON schema."`
	Type   string              `json:"type" example:"urn:canchalibre:error:validation" doc:"Stable URN identifying the problem type."`
	Title  string              `json:"title" example:"Bad Request" doc:"HTTP status text corresponding to status."`
	Status int                 `json:"status" example:"400" doc:"HTTP status code."`
	Detail string              `json:"detail,omitempty" example:"Request validation failed." doc:"Human-readable explanation specific to this occurrence."`
	Errors []*huma.ErrorDetail `json:"errors,omitempty" doc:"Optional field-level validation errors."`
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
	details := make([]*huma.ErrorDetail, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		if d, ok := err.(*huma.ErrorDetail); ok {
			details = append(details, d)
		} else {
			details = append(details, &huma.ErrorDetail{Message: err.Error()})
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
// (used by Chi NotFound/MethodNotAllowed fallbacks and auth middleware).
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
		if raw := registry.SchemaFromRef("#/components/schemas/ProblemDetail"); raw != nil {
			raw.Description = "RFC 7807 problem detail carrying a stable CanchaLibre error type URN."
		}
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
