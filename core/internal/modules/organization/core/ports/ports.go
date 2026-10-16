package ports

import (
	"context"
	"time"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/platform/actor"
)

// Repository defines persistence operations for the schema_organization spine.
type Repository interface {
	VerifyScope(ctx context.Context, accountID, businessID, branchID string) (actor.Role, error)
	HasAnyActiveMembership(ctx context.Context, accountID string) (actor.Role, bool, error)
	ListContextsForAccount(ctx context.Context, accountID string) ([]domain.ContextView, error)

	BusinessExists(ctx context.Context, businessID string) (bool, error)
	BranchBelongsToBusiness(ctx context.Context, businessID, branchID string) (bool, error)
	IsActiveCoOwner(ctx context.Context, accountID, businessID string) (bool, error)
	HasActiveMembershipInBusiness(ctx context.Context, accountID, businessID string) (bool, error)

	CreateMembership(ctx context.Context, m domain.Membership) (domain.Membership, error)
	RevokeCoOwner(ctx context.Context, businessID, accountID string) error
	ListMemberships(ctx context.Context, businessID string) ([]domain.Membership, error)
	RevokeMembershipByID(ctx context.Context, businessID, membershipID string) error
}

// IdentityPort bridges account and invitation operations needed by organization
// use cases without importing identity/core.
type IdentityPort interface {
	VerifyWorkAccountReadyForCoOwner(ctx context.Context, accountID string) error
	EnsureCoOwnerAccountByEmail(ctx context.Context, issuerAccountID, email, businessID string) (accountID string, err error)
	LookupAccountKindAndRoleByEmail(ctx context.Context, email string) (accountExists bool, accountID string, platformRole *actor.Role, err error)
	CreateWorkInvitation(ctx context.Context, issuerAccountID, email string, intendedRole actor.Role, businessID string, branchID *string) (invitationID string, expiresAt time.Time, err error)
	RevokeWorkInvitation(ctx context.Context, actorAccountID, businessID, invitationID string) error
	RecordAudit(ctx context.Context, event, outcome string, actorAccountID, subjectAccountID *string, reason string, detail map[string]any) error
}
