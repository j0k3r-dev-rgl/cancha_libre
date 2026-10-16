package domain

import (
	"errors"
	"time"

	"canchalibre/core/internal/platform/actor"
)

// Account lifecycle statuses in schema_identity.account.
const (
	StatusPending   = "PENDING"
	StatusActive    = "ACTIVE"
	StatusSuspended = "SUSPENDED"
	StatusClosed    = "CLOSED"
)

// Audit outcomes in schema_identity.identity_audit.
const (
	AuditOutcomeSuccess = "SUCCESS"
	AuditOutcomeRefused = "REFUSED"
)

// Domain sentinel errors mapped by inbound adapters to RFC 7807 problem types.
var (
	ErrValidation           = errors.New("validation failed")
	ErrUnauthenticated      = errors.New("unauthenticated")
	ErrRefreshReuse         = errors.New("refresh token reuse detected; session family revoked")
	ErrForbidden            = errors.New("forbidden")
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("resource state conflict")
	ErrInvitationConsumed   = errors.New("invitation already consumed")
	ErrInvitationExpired    = errors.New("invitation expired")
	ErrContextNotAuthorized = errors.New("requested business or branch context is not authorized")
)

// Account represents a row in schema_identity.account.
type Account struct {
	ID           string
	Email        string
	Kind         actor.AccountKind
	PlatformRole *actor.Role
	Status       string
	DisplayName  *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ClosedAt     *time.Time
}

// Invitation represents a row in schema_identity.invitation.
type Invitation struct {
	ID           string
	TokenHash    []byte
	Email        string
	IntendedRole actor.Role
	BusinessID   *string
	BranchID     *string
	IssuedBy     *string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
	ConsumedBy   *string
	RetiredAt    *time.Time
}

// RefreshSession represents a row in schema_identity.refresh_session.
type RefreshSession struct {
	ID         string
	AccountID  string
	FamilyID   string
	TokenHash  []byte
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RotatedAt  *time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
}

// ContextOption represents an available business+branch context for an authenticated work actor.
type ContextOption struct {
	BusinessID   string     `json:"business_id"`
	BusinessName string     `json:"business_name,omitempty"`
	BranchID     string     `json:"branch_id"`
	BranchName   string     `json:"branch_name,omitempty"`
	Role         actor.Role `json:"role"`
}

// LoginContextItem matches the documented login response context item shape:
// {business_id, branch_id, role}.
type LoginContextItem struct {
	BusinessID string     `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BranchID   string     `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	Role       actor.Role `json:"role" example:"OWNER" doc:"Membership role in this context."`
}

// AuditEntry represents an append-only row in schema_identity.identity_audit.
type AuditEntry struct {
	Event            string
	Outcome          string
	ActorAccountID   *string
	SubjectAccountID *string
	Reason           string
	Detail           map[string]any
}
