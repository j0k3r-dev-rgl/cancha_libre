package ports

import (
	"context"
	"time"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/platform/actor"
)

// Repository defines persistence operations owned by schema_identity.
type Repository interface {
	FindAccountByEmailAndKind(ctx context.Context, email string, kind actor.AccountKind) (domain.Account, string, error)
	FindAccountByEmailAnyKind(ctx context.Context, email string) ([]domain.Account, map[string]string, error)
	FindAccountByID(ctx context.Context, accountID string) (domain.Account, error)
	CreateAccountWithCredential(ctx context.Context, acc domain.Account, passwordHash string) (domain.Account, error)
	EnsurePendingWorkAccount(ctx context.Context, email string, platformRole *actor.Role) (domain.Account, error)

	CreateRefreshSession(ctx context.Context, accountID string, tokenHash []byte, expiresAt time.Time) (domain.RefreshSession, error)
	RotateRefreshSession(ctx context.Context, currentHash, nextHash []byte, nextExpiresAt time.Time) (domain.RefreshSession, error)
	RevokeRefreshSessionByHash(ctx context.Context, tokenHash []byte) error
	RevokeRefreshSessionsByAccount(ctx context.Context, accountID string) error

	CreateInvitation(ctx context.Context, inv domain.Invitation) (domain.Invitation, error)
	FindInvitationByHash(ctx context.Context, tokenHash []byte) (domain.Invitation, error)
	FindInvitationByID(ctx context.Context, invitationID string) (domain.Invitation, error)
	DeleteUnconsumedInvitation(ctx context.Context, invitationID string, businessID string) error
	HasUnexpiredInvitation(ctx context.Context, email string, intendedRole actor.Role, businessID, branchID *string) (bool, error)
	SupersedeInvitations(ctx context.Context, email string, intendedRole actor.Role) error
	ActivateInvitation(ctx context.Context, invitationID, email string, intendedRole actor.Role, passwordHash string, now time.Time) (domain.Account, domain.Invitation, error)

	CreatePasswordRecoveryToken(ctx context.Context, accountID string, tokenHash []byte, expiresAt time.Time) error
	ConsumePasswordRecoveryToken(ctx context.Context, tokenHash []byte, newPasswordHash string, now time.Time) (string, error)

	RevokeSupportOperator(ctx context.Context, accountID string) error

	ListUsers(ctx context.Context, filter UserFilter) ([]domain.UserSummary, int, error)
	GetAccountDetail(ctx context.Context, accountID string) (domain.Account, error)
	SuspendAccount(ctx context.Context, accountID string) error
	ReactivateAccount(ctx context.Context, accountID string) error

	AppendAudit(ctx context.Context, entry domain.AuditEntry) error
}

// UserFilter defines query parameters for the admin user listing.
type UserFilter struct {
	Kind   string
	Role   string
	Status string
	Search string
	Limit  int
	Offset int
}

// ScopePort bridges schema_organization queries needed by identity use cases
// without importing organization/core.
type ScopePort interface {
	ListAccountContexts(ctx context.Context, accountID string) ([]domain.ContextOption, error)
	VerifyScope(ctx context.Context, accountID, businessID, branchID string) (actor.Role, error)
	EnsureMembershipOnActivation(ctx context.Context, accountID, businessID string, branchID *string, role actor.Role) error
	ListUserMemberships(ctx context.Context, accountID string) ([]domain.UserMembershipView, error)
}

// EmailSender dispatches work activation and password recovery messages. In S1 a
// non-delivering adapter is wired at cmd/api until the email domain prerequisite closes.
type EmailSender interface {
	SendInvitation(ctx context.Context, email, role, rawToken string) error
	SendPasswordRecovery(ctx context.Context, email, rawToken string) error
}
