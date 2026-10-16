package authn

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/platform/actor"
	platformhttp "canchalibre/core/internal/platform/http"
	"canchalibre/core/internal/platform/security"
)

// AccountState represents the live database state of an account read by the guard.
type AccountState struct {
	AccountID    string
	Email        string
	Kind         actor.AccountKind
	PlatformRole *actor.Role
	Status       string
}

// AccountVerifier reads the live status of an account from schema_identity.
type AccountVerifier interface {
	GetAccountState(ctx context.Context, accountID string) (AccountState, error)
}

// ScopeVerifier executes the single indexed guard lookup in schema_organization.
type ScopeVerifier interface {
	VerifyScope(ctx context.Context, accountID, businessID, branchID string) (actor.Role, error)
	HasAnyActiveMembership(ctx context.Context, accountID string) (actor.Role, bool, error)
}

// Guard enforces the T1.2 per-request invariant: the token states where the actor
// asked to work, and the database states whether they still may.
type Guard struct {
	secret   []byte
	accounts AccountVerifier
	scopes   ScopeVerifier
	now      func() time.Time
}

// NewGuard constructs a Guard with the HS256 secret and the account/scope ports.
func NewGuard(secret []byte, accounts AccountVerifier, scopes ScopeVerifier) *Guard {
	return &Guard{
		secret:   secret,
		accounts: accounts,
		scopes:   scopes,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// AuthenticateRequest validates the Authorization header and re-verifies live
// account and membership state. It returns (Principal, 0, "", nil) on success,
// or (Principal{}, httpStatus, problemTypeURN, err) on refusal.
func (g *Guard) AuthenticateRequest(ctx context.Context, authHeader string) (actor.Principal, int, string, error) {
	raw := strings.TrimSpace(authHeader)
	if !strings.HasPrefix(raw, "Bearer ") {
		return actor.Principal{}, http.StatusUnauthorized, platformhttp.TypeUnauthenticated, errors.New("missing or malformed Authorization bearer token")
	}
	tok := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	if tok == "" {
		return actor.Principal{}, http.StatusUnauthorized, platformhttp.TypeUnauthenticated, errors.New("empty bearer token")
	}

	claims, err := security.VerifyAccessToken(g.secret, tok, g.now())
	if err != nil {
		return actor.Principal{}, http.StatusUnauthorized, platformhttp.TypeUnauthenticated, errors.New("invalid or expired access token")
	}

	acc, err := g.accounts.GetAccountState(ctx, claims.Subject)
	if err != nil {
		return actor.Principal{}, http.StatusUnauthorized, platformhttp.TypeUnauthenticated, errors.New("account does not exist")
	}
	if acc.Status != "ACTIVE" {
		return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("account is not active")
	}

	var resolvedRole actor.Role
	switch acc.Kind {
	case actor.KindCustomer:
		if claims.Actor != actor.RolePlayer || claims.Context != nil {
			return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("invalid role or context for customer account")
		}
		resolvedRole = actor.RolePlayer
	case actor.KindWork:
		if acc.PlatformRole != nil {
			if claims.Actor != *acc.PlatformRole || claims.Context != nil {
				return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("invalid role or context for platform actor")
			}
			resolvedRole = *acc.PlatformRole
		} else {
			if claims.Context != nil && claims.Context.BranchID != "" {
				role, err := g.scopes.VerifyScope(ctx, acc.AccountID, claims.Context.BusinessID, claims.Context.BranchID)
				if err != nil || role != claims.Actor {
					return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("active membership or scope is no longer authorized")
				}
				resolvedRole = role
			} else if claims.Context != nil && claims.Context.BusinessID != "" && claims.Actor == actor.RoleOwner {
				// Business-wide OWNER token without a specific branch selected yet
				role, err := g.scopes.VerifyScope(ctx, acc.AccountID, claims.Context.BusinessID, "")
				if err != nil || role != actor.RoleOwner {
					return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("active business ownership is no longer authorized")
				}
				resolvedRole = role
			} else {
				// Authenticated work account without an active context selected yet (e.g. calling /me or /me/contexts or /me/context)
				role, ok, err := g.scopes.HasAnyActiveMembership(ctx, acc.AccountID)
				if err != nil || !ok {
					return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("work account has no active membership")
				}
				resolvedRole = role
			}
		}
	default:
		return actor.Principal{}, http.StatusForbidden, platformhttp.TypeForbidden, errors.New("unknown account kind")
	}

	return actor.Principal{
		AccountID: acc.AccountID,
		Email:     acc.Email,
		Kind:      acc.Kind,
		Actor:     resolvedRole,
		Context:   claims.Context,
	}, 0, "", nil
}

// Middleware returns a Huma operation middleware that enforces AuthenticateRequest,
// records the authenticated account ID for refusal auditing, and injects the
// verified Principal into the request context.
func (g *Guard) Middleware(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		p, status, probType, err := g.AuthenticateRequest(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			_ = probType
			_ = huma.WriteErr(api, ctx, status, err.Error())
			return
		}
		platformhttp.SetRequestActorID(ctx.Context(), p.AccountID)
		next(huma.WithContext(ctx, actor.WithPrincipal(ctx.Context(), p)))
	}
}
