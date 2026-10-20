// Package http implements the identity bounded context's inbound HTTP adapter:
// the request/response DTOs of docs/endpoint-acceptance-criteria.md section 5.2, the one-to-one
// mapping from identity domain sentinels to the frozen RFC 7807 catalog of
// docs/design/platform-error-model.md section 1.1, and the huma operation
// registrations.
package http

import (
	"context"
	"errors"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/usecase"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/authn"
	platformhttp "canchalibre/core/internal/platform/http"
	"canchalibre/core/internal/platform/security"
)

// RefreshCookieName is the single HttpOnly cookie name shared by login, refresh
// and logout. The rotation contract of docs/design/identity-auth.md D5 requires
// one stable name across the three operations.
const RefreshCookieName = "cl_refresh_token"

// refreshCookiePath scopes the cookie to the auth routes that read it.
const refreshCookiePath = "/api/v1/auth"

// Deps are the wired dependencies of the identity HTTP adapter.
type Deps struct {
	Service *usecase.Service
	Guard   *authn.Guard
	Logger  *slog.Logger
}

// Register registers the identity operations for S1 and S2 (platform admin).
func Register(api huma.API, deps Deps) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	registerLogin(api, deps)
	registerRefresh(api, deps)
	registerActivateInvitation(api, deps)
	registerRequestPasswordRecovery(api, deps)
	registerResetPassword(api, deps)
	registerLogout(api, deps)
	registerGetOwnAccount(api, deps)
	registerListOwnContexts(api, deps)
	registerSelectContext(api, deps)
	registerInviteSupportOperator(api, deps)
	registerRevokeSupportOperator(api, deps)

	// S2 Platform Administration
	registerAdminUsers(api, deps)
}

// errorSpecs are the documented error responses of the identity inventory. The
// 422 entry is the catalog's own `validation` refusal for huma's schema-level body
// and parameter validation; page 1.3 of the design requires every refusal an
// operation can actually perform to be declared, not only the prose-row subset.
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
	err401RefreshReuse = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusUnauthorized,
		Type:        platformhttp.TypeRefreshReuse,
		Description: "A rotated refresh token was presented again.",
		Detail:      "The refresh token was already rotated; the session family was revoked.",
	}
	err403Forbidden = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusForbidden,
		Type:        platformhttp.TypeForbidden,
		Description: "The authenticated actor is not permitted.",
		Detail:      "The authenticated actor is not permitted to perform this action.",
	}
	err403ContextNotAuthorized = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusForbidden,
		Type:        platformhttp.TypeContextNotAuthorized,
		Description: "The requested business and branch context is not authorized.",
		Detail:      "The requested business and branch context is not authorized.",
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
	err409InvitationConsumed = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusConflict,
		Type:        platformhttp.TypeInvitationConsumed,
		Description: "The invitation was already activated.",
		Detail:      "The invitation has already been activated.",
	}
	err409InvitationExpired = platformhttp.ErrorResponseSpec{
		Status:      nethttp.StatusConflict,
		Type:        platformhttp.TypeInvitationExpired,
		Description: "The invitation is past its expiry.",
		Detail:      "The invitation has expired.",
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

// problemFromDomain maps the identity domain sentinels to the frozen catalog
// status plus URN, one-to-one. It returns nil for an unexpected error so the
// caller can hide internals behind urn:canchalibre:error:internal.
func problemFromDomain(err error) huma.StatusError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrValidation):
		return platformhttp.NewTypedError(nethttp.StatusBadRequest, platformhttp.TypeValidation, "Request validation failed.")
	case errors.Is(err, domain.ErrUnauthenticated):
		return platformhttp.NewTypedError(nethttp.StatusUnauthorized, platformhttp.TypeUnauthenticated, "Authentication is required.")
	case errors.Is(err, domain.ErrRefreshReuse):
		return platformhttp.NewTypedError(nethttp.StatusUnauthorized, platformhttp.TypeRefreshReuse, "The refresh token was already rotated; the session family was revoked.")
	case errors.Is(err, domain.ErrForbidden):
		return platformhttp.NewTypedError(nethttp.StatusForbidden, platformhttp.TypeForbidden, "The authenticated actor is not permitted to perform this action.")
	case errors.Is(err, domain.ErrNotFound):
		return platformhttp.NewTypedError(nethttp.StatusNotFound, platformhttp.TypeNotFound, "The requested resource was not found.")
	case errors.Is(err, domain.ErrConflict):
		return platformhttp.NewTypedError(nethttp.StatusConflict, platformhttp.TypeConflict, "The request contradicts the current state.")
	case errors.Is(err, domain.ErrInvitationConsumed):
		return platformhttp.NewTypedError(nethttp.StatusConflict, platformhttp.TypeInvitationConsumed, "The invitation has already been activated.")
	case errors.Is(err, domain.ErrInvitationExpired):
		return platformhttp.NewTypedError(nethttp.StatusConflict, platformhttp.TypeInvitationExpired, "The invitation has expired.")
	case errors.Is(err, domain.ErrContextNotAuthorized):
		return platformhttp.NewTypedError(nethttp.StatusForbidden, platformhttp.TypeContextNotAuthorized, "The requested business and branch context is not authorized.")
	default:
		return nil
	}
}

// fail converts a use-case error into a problem response without ever leaking SQL
// or provider internals: unexpected errors are logged and mapped to `internal`.
func (d Deps) fail(ctx context.Context, err error) error {
	if problem := problemFromDomain(err); problem != nil {
		return problem
	}
	d.Logger.ErrorContext(ctx, "identity request failed with an unexpected error",
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

// newRefreshCookie builds the rotated HttpOnly session cookie of D5.
func newRefreshCookie(rawToken string, expiresAt time.Time) nethttp.Cookie {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		ttl = security.RefreshTokenTTL
	}
	return nethttp.Cookie{
		Name:     RefreshCookieName,
		Value:    rawToken,
		Path:     refreshCookiePath,
		Expires:  expiresAt.UTC(),
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: nethttp.SameSiteLaxMode,
	}
}

// clearRefreshCookie expires the session cookie on logout.
func clearRefreshCookie() nethttp.Cookie {
	return nethttp.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: nethttp.SameSiteLaxMode,
	}
}
