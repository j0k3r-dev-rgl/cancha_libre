package ports

import (
	"context"
	"time"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/platform/actor"
)

// BusinessFilter defines query parameters for the admin business listing.
type BusinessFilter struct {
	Status string // "all", "active", "suspended", "archived"
	Search string
	Limit  int
	Offset int
}

// BranchRequestFilter defines query parameters for the branch requests inbox.
type BranchRequestFilter struct {
	Status string // "PENDING", "APPROVED", "REJECTED", ""
	Limit  int
	Offset int
}

// BranchFilter defines query parameters for the global branch listing.
type BranchFilter struct {
	BusinessID     string
	ApprovalStatus string
	Status         string // "all", "active", "suspended", "archived"
	City           string
	Limit          int
	Offset         int
}

// Repository defines persistence operations for schema_organization.
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

	// Slice S2 Platform Admin additions
	CreateBusiness(ctx context.Context, b domain.Business) (domain.Business, error)
	ListBusinesses(ctx context.Context, filter BusinessFilter) ([]domain.BusinessSummary, int, error)
	GetBusinessByID(ctx context.Context, businessID string) (domain.Business, []string, []domain.BusinessBranchView, error)
	UpdateBusiness(ctx context.Context, businessID string, name, legalName, taxID *string) (domain.Business, error)
	SuspendBusiness(ctx context.Context, businessID string) (domain.Business, error)
	ReactivateBusiness(ctx context.Context, businessID string) (domain.Business, error)
	ArchiveBusiness(ctx context.Context, businessID string) error
	UnarchiveBusiness(ctx context.Context, businessID string) (domain.Business, error)

	ListBranchRequests(ctx context.Context, filter BranchRequestFilter) ([]domain.BranchRequestItem, int, error)
	GetBranchRequestDetail(ctx context.Context, requestID string) (domain.BranchRequestDetail, error)
	ApproveBranchRequest(ctx context.Context, requestID, reviewedBy string) (domain.BranchApprovalHistory, error)
	RejectBranchRequest(ctx context.Context, requestID, reviewedBy, reason string) (domain.BranchApprovalHistory, error)

	ListBranches(ctx context.Context, filter BranchFilter) ([]domain.BranchView, int, error)
	SuspendBranch(ctx context.Context, branchID string) (domain.Branch, error)
	ReactivateBranch(ctx context.Context, branchID string) (domain.Branch, error)
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
	GetAccountsByIDs(ctx context.Context, accountIDs []string) ([]domain.BusinessOwnerView, error)
}
