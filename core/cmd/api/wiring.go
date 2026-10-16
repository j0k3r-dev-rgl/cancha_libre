package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	identityhttp "canchalibre/core/internal/modules/identity/adapters/inbound/http"
	identitypostgres "canchalibre/core/internal/modules/identity/adapters/outbound/postgres"
	identitydomain "canchalibre/core/internal/modules/identity/core/domain"
	identityusecase "canchalibre/core/internal/modules/identity/core/usecase"
	organizationhttp "canchalibre/core/internal/modules/organization/adapters/inbound/http"
	organizationpostgres "canchalibre/core/internal/modules/organization/adapters/outbound/postgres"
	organizationdomain "canchalibre/core/internal/modules/organization/core/domain"
	organizationusecase "canchalibre/core/internal/modules/organization/core/usecase"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/authn"
	"canchalibre/core/internal/platform/config"
	platformhttp "canchalibre/core/internal/platform/http"
)

// newRouter wires the pgx pool, both bounded-context repositories, the bridge
// that exposes organization scope lookups as identity's ScopePort, the identity
// service as organization's IdentityPort, the authn guard and every S1 operation
// into one Chi router. It is a function instead of inline wiring so the contract
// test can build the real router without booting a server.
func newRouter(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) (*chi.Mux, huma.API) {
	if logger == nil {
		logger = slog.Default()
	}

	identityRepo := identitypostgres.NewRepository(pool)
	organizationRepo := organizationpostgres.NewRepository(pool)

	// The bridge from organization's repository to identity's ScopePort must live
	// here: no module package may import another context's core
	// (internal/platform/authn/guard_test.go, TestNoCrossContextCoreImports).
	identityService := identityusecase.NewService(
		identityRepo,
		&identityScopeBridge{repo: organizationRepo},
		&loggingEmailSender{logger: logger, baseURL: cfg.InvitationURLBase},
		identityusecase.Config{
			JWTSecret:         []byte(cfg.JWTSecret),
			InvitationURLBase: cfg.InvitationURLBase,
		},
	)

	// identity.Service already implements organization/core/ports.IdentityPort.
	organizationService := organizationusecase.NewService(organizationRepo, identityService)

	guard := authn.NewGuard([]byte(cfg.JWTSecret), identityRepo, organizationRepo)

	router, api := platformhttp.NewRouter(logger)
	identityhttp.Register(api, identityhttp.Deps{Service: identityService, Guard: guard, Logger: logger})
	organizationhttp.Register(api, organizationhttp.Deps{Service: organizationService, Guard: guard, Logger: logger})

	return router, api
}

// identityScopeBridge adapts the organization repository to the identity
// ScopePort, translating between the two contexts' own value types.
type identityScopeBridge struct {
	repo *organizationpostgres.Repository
}

// ListAccountContexts satisfies identity/ports.ScopePort.
func (b *identityScopeBridge) ListAccountContexts(ctx context.Context, accountID string) ([]identitydomain.ContextOption, error) {
	views, err := b.repo.ListContextsForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]identitydomain.ContextOption, 0, len(views))
	for _, view := range views {
		out = append(out, identitydomain.ContextOption{
			BusinessID:   view.BusinessID,
			BusinessName: view.BusinessName,
			BranchID:     view.BranchID,
			BranchName:   view.BranchName,
			Role:         view.Role,
		})
	}
	return out, nil
}

// VerifyScope satisfies identity/ports.ScopePort.
func (b *identityScopeBridge) VerifyScope(ctx context.Context, accountID, businessID, branchID string) (actor.Role, error) {
	return b.repo.VerifyScope(ctx, accountID, businessID, branchID)
}

// EnsureMembershipOnActivation satisfies identity/ports.ScopePort. A duplicate
// membership means the desired state already holds, so it is not an error.
func (b *identityScopeBridge) EnsureMembershipOnActivation(ctx context.Context, accountID, businessID string, branchID *string, role actor.Role) error {
	_, err := b.repo.CreateMembership(ctx, organizationdomain.Membership{
		AccountID:  accountID,
		BusinessID: businessID,
		BranchID:   branchID,
		Role:       role,
	})
	if err != nil && errors.Is(err, organizationdomain.ErrConflict) {
		return nil
	}
	return err
}

// loggingEmailSender is the non-delivering identity EmailSender acknowledged by
// S1: the email transport is a separate prerequisite, so delivery is reported at
// debug level instead of failing the request or swallowing it silently.
type loggingEmailSender struct {
	logger  *slog.Logger
	baseURL string
}

// SendInvitation satisfies identity/ports.EmailSender.
func (s *loggingEmailSender) SendInvitation(ctx context.Context, email, role, rawToken string) error {
	if s == nil || s.logger == nil {
		return nil
	}
	s.logger.DebugContext(ctx, "email delivery is not configured in S1; invitation left for manual delivery",
		"email", email, "role", role, "activation_url", activationURL(s.baseURL, rawToken))
	return nil
}

// SendPasswordRecovery satisfies identity/ports.EmailSender.
func (s *loggingEmailSender) SendPasswordRecovery(ctx context.Context, email, rawToken string) error {
	if s == nil || s.logger == nil {
		return nil
	}
	s.logger.DebugContext(ctx, "email delivery is not configured in S1; password recovery left for manual delivery",
		"email", email, "recovery_url", activationURL(s.baseURL, rawToken))
	return nil
}

// activationURL builds the single-use activation link handed to the placeholder
// sender. It mirrors the URL shape of cmd/provision-admin.
func activationURL(baseURL, rawToken string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "http://localhost:3000/auth/activate"
	}
	return base + "?token=" + rawToken
}
