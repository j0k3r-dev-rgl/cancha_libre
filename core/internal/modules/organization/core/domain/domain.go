package domain

import (
	"errors"
	"time"

	"canchalibre/core/internal/platform/actor"
)

// Domain sentinel errors for schema_organization.
var (
	ErrValidation           = errors.New("validation failed")
	ErrForbidden            = errors.New("forbidden")
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("membership already exists or role conflict")
	ErrLastCoOwner          = errors.New("cannot revoke the last active co-owner of a business")
	ErrContextNotAuthorized = errors.New("requested business or branch context is not authorized")
)

// Business represents a row in schema_organization.business.
type Business struct {
	ID         string
	Name       string
	Enabled    bool
	CreatedAt  time.Time
	ArchivedAt *time.Time
}

// Branch represents a row in schema_organization.branch.
type Branch struct {
	ID         string
	BusinessID string
	Name       string
	Enabled    bool
	CreatedAt  time.Time
	ArchivedAt *time.Time
}

// Membership represents a row in schema_organization.membership.
type Membership struct {
	ID         string     `json:"membership_id"`
	AccountID  string     `json:"account_id"`
	BusinessID string     `json:"business_id,omitempty"`
	BranchID   *string    `json:"branch_id"`
	Role       actor.Role `json:"role"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"-"`
}

// ContextView represents an active business+branch context available to an account.
type ContextView struct {
	BusinessID   string     `json:"business_id"`
	BusinessName string     `json:"business_name"`
	BranchID     string     `json:"branch_id"`
	BranchName   string     `json:"branch_name"`
	Role         actor.Role `json:"role"`
}
