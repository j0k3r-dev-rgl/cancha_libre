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
	ErrBusinessSuspended    = errors.New("business is suspended")
	ErrBranchSuspended      = errors.New("branch is suspended")
	ErrAlreadySuspended     = errors.New("resource is already suspended")
	ErrNotSuspended         = errors.New("resource is not suspended")
	ErrAlreadyArchived      = errors.New("resource is already archived")
	ErrNotArchived          = errors.New("resource is not archived")
	ErrActiveReservations   = errors.New("cannot archive: active reservations or pending requests exist")
	ErrAlreadyResolved      = errors.New("branch request is already approved or rejected")
)

// ApprovalStatus represents the review state of a branch or branch request.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "PENDING_APPROVAL"
	ApprovalStatusApproved ApprovalStatus = "APPROVED"
	ApprovalStatusRejected ApprovalStatus = "REJECTED"
)

// Business represents a row in schema_organization.business.
type Business struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	LegalName   *string    `json:"legal_name,omitempty"`
	TaxID       *string    `json:"tax_id,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
	SuspendedAt *time.Time `json:"suspended_at,omitempty"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

// Branch represents a row in schema_organization.branch.
type Branch struct {
	ID                   string         `json:"id"`
	BusinessID           string         `json:"business_id"`
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug"`
	Description          *string        `json:"description,omitempty"`
	Phone                *string        `json:"phone,omitempty"`
	Whatsapp             *string        `json:"whatsapp,omitempty"`
	Email                *string        `json:"email,omitempty"`
	LegalName            *string        `json:"legal_name,omitempty"`
	TaxID                *string        `json:"tax_id,omitempty"`
	StreetAddress        string         `json:"street_address"`
	FloorApt             *string        `json:"floor_apt,omitempty"`
	City                 string         `json:"city"`
	StateProvince        string         `json:"state_province"`
	PostalCode           string         `json:"postal_code"`
	Country              string         `json:"country"`
	Latitude             *float64       `json:"latitude,omitempty"`
	Longitude            *float64       `json:"longitude,omitempty"`
	GooglePlaceID        *string        `json:"google_place_id,omitempty"`
	CoordinatesConfirmed bool           `json:"coordinates_confirmed"`
	MinBookingLeadMins   int            `json:"min_booking_lead_minutes"`
	MinCancelLeadMins    int            `json:"min_cancellation_lead_minutes"`
	ApprovalStatus       ApprovalStatus `json:"approval_status"`
	Enabled              bool           `json:"enabled"`
	SuspendedAt          *time.Time     `json:"suspended_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	ArchivedAt           *time.Time     `json:"archived_at,omitempty"`
}

// IsPublished computes the publication predicate: approved, coordinates confirmed,
// owner enabled, neither branch nor business suspended or archived.
func (b *Branch) IsPublished(businessEnabled, businessSuspended, businessArchived bool) bool {
	return b.ApprovalStatus == ApprovalStatusApproved &&
		b.CoordinatesConfirmed &&
		b.Enabled &&
		b.SuspendedAt == nil &&
		b.ArchivedAt == nil &&
		businessEnabled &&
		!businessSuspended &&
		!businessArchived
}

// BranchApprovalHistory represents a row in schema_organization.branch_approval_history.
type BranchApprovalHistory struct {
	ID              string         `json:"id"`
	BranchID        string         `json:"branch_id"`
	SubmittedBy     string         `json:"submitted_by"`
	SubmittedAt     time.Time      `json:"submitted_at"`
	ReviewedBy      *string        `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time     `json:"reviewed_at,omitempty"`
	Status          ApprovalStatus `json:"status"`
	RejectionReason *string        `json:"rejection_reason,omitempty"`
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

// BusinessSummary represents a business row for admin list views.
type BusinessSummary struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	LegalName         *string    `json:"legal_name,omitempty"`
	TaxID             *string    `json:"tax_id,omitempty"`
	Enabled           bool       `json:"enabled"`
	SuspendedAt       *time.Time `json:"suspended_at,omitempty"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	BranchesCount     int        `json:"branches_count"`
	ActiveOwnersCount int        `json:"active_owners_count"`
}

// BusinessOwnerView describes an owner account associated with a business.
type BusinessOwnerView struct {
	AccountID   string  `json:"account_id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
	Status      string  `json:"status"`
}

// BusinessBranchView describes a branch associated with a business in detail view.
type BusinessBranchView struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug"`
	ApprovalStatus       ApprovalStatus `json:"approval_status"`
	CoordinatesConfirmed bool           `json:"coordinates_confirmed"`
	Enabled              bool           `json:"enabled"`
	IsPublished          bool           `json:"is_published"`
	SuspendedAt          *time.Time     `json:"suspended_at,omitempty"`
	ArchivedAt           *time.Time     `json:"archived_at,omitempty"`
}

// BusinessDetail holds the complete administrative view of a business.
type BusinessDetail struct {
	Business
	Owners   []BusinessOwnerView  `json:"owners"`
	Branches []BusinessBranchView `json:"branches"`
}

// BranchView represents a branch in the global admin list.
type BranchView struct {
	ID                   string         `json:"id"`
	BusinessID           string         `json:"business_id"`
	BusinessName         string         `json:"business_name,omitempty"`
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug"`
	City                 string         `json:"city"`
	StateProvince        string         `json:"state_province"`
	ApprovalStatus       ApprovalStatus `json:"approval_status"`
	CoordinatesConfirmed bool           `json:"coordinates_confirmed"`
	Enabled              bool           `json:"enabled"`
	IsPublished          bool           `json:"is_published"`
	SuspendedAt          *time.Time     `json:"suspended_at,omitempty"`
	ArchivedAt           *time.Time     `json:"archived_at,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
}

// BranchRequestItem represents an item in the branch requests inbox.
type BranchRequestItem struct {
	ID              string         `json:"id"`
	BranchID        string         `json:"branch_id"`
	BusinessID      string         `json:"business_id"`
	BusinessName    string         `json:"business_name"`
	BranchName      string         `json:"branch_name"`
	SubmittedBy     string         `json:"submitted_by"`
	SubmittedAt     time.Time      `json:"submitted_at"`
	ReviewedBy      *string        `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time     `json:"reviewed_at,omitempty"`
	Status          ApprovalStatus `json:"status"`
	RejectionReason *string        `json:"rejection_reason,omitempty"`
}

// BranchRequestDetail represents a branch request with full branch and history details.
type BranchRequestDetail struct {
	BranchRequestItem
	Branch  Branch                  `json:"branch"`
	History []BranchApprovalHistory `json:"history"`
}

// UserMembershipView represents an active or historical membership for a user in the admin view.
type UserMembershipView struct {
	ID           string     `json:"id"`
	BusinessID   string     `json:"business_id"`
	BusinessName string     `json:"business_name"`
	BranchID     *string    `json:"branch_id,omitempty"`
	BranchName   *string    `json:"branch_name,omitempty"`
	Role         actor.Role `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
}

