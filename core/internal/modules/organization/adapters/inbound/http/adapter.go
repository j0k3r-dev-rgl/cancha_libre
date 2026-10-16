// Package http implements the organization bounded context's inbound HTTP
// adapter: the request/response DTOs of docs/roadmap-backend.md section 5.2, the
// one-to-one mapping from organization domain sentinels to the frozen RFC 7807
// catalog of docs/design-identity-endpoints.md section 1.1, and the huma
// operation registrations.
package http

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/modules/organization/core/usecase"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/authn"
	platformhttp "canchalibre/core/internal/platform/http"
)

// Deps are the wired dependencies of the organization HTTP adapter.
type Deps struct {
	Service *usecase.Service
	Guard   *authn.Guard
	Logger  *slog.Logger
}

// Register registers the 6 organization operations of the frozen S1 inventory.
func Register(api huma.API, deps Deps) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	registerAssignCoOwner(api, deps)
	registerRevokeCoOwner(api, deps)
	registerListMemberships(api, deps)
	registerInviteMember(api, deps)
	registerRevokeMembership(api, deps)
	registerRevokeInvitation(api, deps)
}

var (
	err400Validation = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusBadRequest,
		Type:        platformhttp.TypeValidation,
		Description: "Request shape or field constraint failed.",
		Detail:      "Request validation failed.",
	}
	err422Validation = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusUnprocessableEntity,
		Type:        platformhttp.TypeValidation,
		Description: "Request body or parameter failed schema validation.",
		Detail:      "Request validation failed.",
	}
	err401Unauthenticated = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusUnauthorized,
		Type:        platformhttp.TypeUnauthenticated,
		Description: "No valid credential was presented.",
		Detail:      "Authentication is required.",
	}
	err403Forbidden = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusForbidden,
		Type:        platformhttp.TypeForbidden,
		Description: "The authenticated actor is not permitted.",
		Detail:      "The authenticated actor is not permitted to perform this action.",
	}
	err404NotFound = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusNotFound,
		Type:        platformhttp.TypeNotFound,
		Description: "No such record, or a record outside the actor's scope.",
		Detail:      "The requested resource was not found.",
	}
	err409Conflict = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusConflict,
		Type:        platformhttp.TypeConflict,
		Description: "The request contradicts the current state.",
		Detail:      "The request contradicts the current state.",
	}
	err409LastCoOwner = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusConflict,
		Type:        platformhttp.TypeLastCoOwner,
		Description: "Revoking would leave the business without a co-owner.",
		Detail:      "Cannot revoke the last active co-owner of a business.",
	}
	err415UnsupportedMediaType = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusUnsupportedMediaType,
		Type:        platformhttp.TypeUnsupportedMediaType,
		Description: "The request Content-Type is not supported.",
		Detail:      "The request Content-Type is not supported.",
	}
	err500Internal = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusInternalServerError,
		Type:        platformhttp.TypeInternal,
		Description: "Unexpected internal server error.",
		Detail:      "Unexpected internal server error.",
	}
)

// errorResponses builds the explicit huma Responses entries for one operation.
func errorResponses(api huma.API, specs ...platformhttp.ErrorResponseSpec) map[string]*huma.Response {
	return platformhttp.BuildErrorResponses(api, specs...)
}

// problemFromDomain maps the organization domain sentinels to the frozen catalog
// status plus URN, one-to-one.
func problemFromDomain(err error) huma.StatusError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrValidation):
		return platformhttp.NewTypedError(nethttp.StatusBadRequest, platformhttp.TypeValidation, "Request validation failed.")
	case errors.Is(err, domain.ErrForbidden):
		return platformhttp.NewTypedError(nethttp.StatusForbidden, platformhttp.TypeForbidden, "The authenticated actor is not permitted to perform this action.")
	case errors.Is(err, domain.ErrNotFound):
		return platformhttp.NewTypedError(nethttp.StatusNotFound, platformhttp.TypeNotFound, "The requested resource was not found.")
	case errors.Is(err, domain.ErrConflict):
		return platformhttp.NewTypedError(nethttp.StatusConflict, platformhttp.TypeConflict, "The request contradicts the current state.")
	case errors.Is(err, domain.ErrLastCoOwner):
		return platformhttp.NewTypedError(nethttp.StatusConflict, platformhttp.TypeLastCoOwner, "Cannot revoke the last active co-owner of a business.")
	case errors.Is(err, domain.ErrContextNotAuthorized):
		return platformhttp.NewTypedError(nethttp.StatusForbidden, platformhttp.TypeContextNotAuthorized, "The requested business and branch context is not authorized.")
	default:
		return nil
	}
}

// fail converts a use-case error into a problem response without leaking SQL or
// provider internals: unexpected errors are logged and mapped to `internal`.
func (d Deps) fail(ctx context.Context, err error) error {
	if problem := problemFromDomain(err); problem != nil {
		return problem
	}
	d.Logger.ErrorContext(ctx, "organization request failed with an unexpected error",
		"error", err, "request_id", platformhttp.RequestID(ctx))
	return platformhttp.NewTypedError(nethttp.StatusInternalServerError, platformhttp.TypeInternal, "Unexpected internal server error.")
}

// principal returns the principal injected by the authn guard middleware.
func (d Deps) principal(ctx context.Context) (actor.Principal, error) {
	p, ok := actor.FromContext(ctx)
	if !ok {
		return actor.Principal{}, platformhttp.NewTypedError(nethttp.StatusUnauthorized, platformhttp.TypeUnauthenticated, "Authentication is required.")
	}
	return p, nil
}

// guarded returns the per-request guard middleware for an authenticated operation.
func (d Deps) guarded(api huma.API) huma.Middlewares {
	return huma.Middlewares{d.Guard.Middleware(api)}
}

// emptyOutput is the output of an operation that returns no response body.
type emptyOutput struct{}
