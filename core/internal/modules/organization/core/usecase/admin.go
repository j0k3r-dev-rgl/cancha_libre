package usecase

import (
	"context"
	"errors"
	"strings"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

// CreateBusinessInput holds the payload for POST /api/v1/admin/businesses.
type CreateBusinessInput struct {
	Name        string   `json:"name"`
	LegalName   *string  `json:"legal_name,omitempty"`
	TaxID       *string  `json:"tax_id,omitempty"`
	OwnerEmails []string `json:"owner_emails,omitempty"`
}

// UpdateBusinessInput holds the payload for PATCH /api/v1/admin/businesses/{businessId}.
type UpdateBusinessInput struct {
	Name      *string `json:"name,omitempty"`
	LegalName *string `json:"legal_name,omitempty"`
	TaxID     *string `json:"tax_id,omitempty"`
}

// CreateBusiness creates a new business and assigns initial co-owners (ADMIN only).
func (s *Service) CreateBusiness(ctx context.Context, p actor.Principal, in CreateBusinessInput) (domain.BusinessDetail, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "create_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.BusinessDetail{}, domain.ErrForbidden
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		s.audit(ctx, "create_business", "REFUSED", &p.AccountID, nil, "name cannot be empty", nil)
		return domain.BusinessDetail{}, domain.ErrValidation
	}

	var legalName, taxID *string
	if in.LegalName != nil {
		ln := strings.TrimSpace(*in.LegalName)
		if ln != "" {
			legalName = &ln
		}
	}
	if in.TaxID != nil {
		tid := strings.TrimSpace(*in.TaxID)
		if tid != "" {
			taxID = &tid
		}
	}

	// Validate owner emails before creating the business
	normalizedEmails := make([]string, 0, len(in.OwnerEmails))
	for _, rawEmail := range in.OwnerEmails {
		norm, err := security.NormalizeEmail(rawEmail)
		if err != nil {
			s.audit(ctx, "create_business", "REFUSED", &p.AccountID, nil, "invalid owner email: "+rawEmail, nil)
			return domain.BusinessDetail{}, domain.ErrValidation
		}
		normalizedEmails = append(normalizedEmails, norm)
	}

	busID, err := actor.NewUUIDv7()
	if err != nil {
		return domain.BusinessDetail{}, err
	}

	b, err := s.repo.CreateBusiness(ctx, domain.Business{
		ID:        busID,
		Name:      name,
		LegalName: legalName,
		TaxID:     taxID,
		Enabled:   true,
	})
	if err != nil {
		s.audit(ctx, "create_business", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.BusinessDetail{}, err
	}

	ownerViews := make([]domain.BusinessOwnerView, 0, len(normalizedEmails))
	for _, email := range normalizedEmails {
		accID, err := s.identity.EnsureCoOwnerAccountByEmail(ctx, p.AccountID, email, b.ID)
		if err != nil {
			s.audit(ctx, "create_business_owner", "REFUSED", &p.AccountID, nil, err.Error(), nil)
			continue
		}
		_, err = s.repo.CreateMembership(ctx, domain.Membership{
			AccountID:  accID,
			BusinessID: b.ID,
			Role:       actor.RoleOwner,
		})
		if err != nil && !errors.Is(err, domain.ErrConflict) {
			s.audit(ctx, "create_business_owner", "REFUSED", &p.AccountID, &accID, err.Error(), nil)
			continue
		}
		ownerViews = append(ownerViews, domain.BusinessOwnerView{
			AccountID: accID,
			Email:     email,
			Status:    "PENDING",
		})
	}

	s.audit(ctx, "create_business", "SUCCESS", &p.AccountID, nil, "business created", map[string]any{
		"business_id": b.ID,
		"name":        b.Name,
	})

	return domain.BusinessDetail{
		Business: b,
		Owners:   ownerViews,
		Branches: []domain.BusinessBranchView{},
	}, nil
}

// ListBusinesses returns paginated business summaries (ADMIN only).
func (s *Service) ListBusinesses(ctx context.Context, p actor.Principal, filter ports.BusinessFilter) ([]domain.BusinessSummary, int, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "list_businesses", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return nil, 0, domain.ErrForbidden
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	if filter.Status != "" && filter.Status != "all" && filter.Status != "active" && filter.Status != "suspended" && filter.Status != "archived" {
		s.audit(ctx, "list_businesses", "REFUSED", &p.AccountID, nil, "invalid status filter", nil)
		return nil, 0, domain.ErrValidation
	}

	return s.repo.ListBusinesses(ctx, filter)
}

// GetBusiness returns full administrative details of a business (ADMIN only).
func (s *Service) GetBusiness(ctx context.Context, p actor.Principal, businessID string) (domain.BusinessDetail, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "get_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.BusinessDetail{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "get_business", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.BusinessDetail{}, domain.ErrNotFound
	}

	bus, ownerIDs, branches, err := s.repo.GetBusinessByID(ctx, normID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.audit(ctx, "get_business", "REFUSED", &p.AccountID, nil, "business not found", nil)
			return domain.BusinessDetail{}, domain.ErrNotFound
		}
		return domain.BusinessDetail{}, err
	}

	var owners []domain.BusinessOwnerView
	if len(ownerIDs) > 0 && s.identity != nil {
		views, err := s.identity.GetAccountsByIDs(ctx, ownerIDs)
		if err == nil {
			owners = views
		}
	}
	if owners == nil {
		owners = []domain.BusinessOwnerView{}
	}
	if branches == nil {
		branches = []domain.BusinessBranchView{}
	}

	return domain.BusinessDetail{
		Business: bus,
		Owners:   owners,
		Branches: branches,
	}, nil
}

// UpdateBusiness updates commercial or fiscal details of a business (ADMIN only).
func (s *Service) UpdateBusiness(ctx context.Context, p actor.Principal, businessID string, in UpdateBusinessInput) (domain.Business, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "update_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Business{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "update_business", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.Business{}, domain.ErrNotFound
	}

	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		s.audit(ctx, "update_business", "REFUSED", &p.AccountID, nil, "name cannot be empty", nil)
		return domain.Business{}, domain.ErrValidation
	}

	b, err := s.repo.UpdateBusiness(ctx, normID, in.Name, in.LegalName, in.TaxID)
	if err != nil {
		s.audit(ctx, "update_business", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.Business{}, err
	}

	s.audit(ctx, "update_business", "SUCCESS", &p.AccountID, nil, "business updated", map[string]any{
		"business_id": normID,
	})
	return b, nil
}

// SuspendBusiness administratively suspends a business (ADMIN only).
func (s *Service) SuspendBusiness(ctx context.Context, p actor.Principal, businessID, reason string) (domain.Business, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "suspend_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Business{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "suspend_business", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.Business{}, domain.ErrNotFound
	}
	if strings.TrimSpace(reason) == "" {
		s.audit(ctx, "suspend_business", "REFUSED", &p.AccountID, nil, "reason is required", nil)
		return domain.Business{}, domain.ErrValidation
	}

	b, err := s.repo.SuspendBusiness(ctx, normID)
	if err != nil {
		s.audit(ctx, "suspend_business", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.Business{}, err
	}

	s.audit(ctx, "suspend_business", "SUCCESS", &p.AccountID, nil, "business suspended: "+reason, map[string]any{
		"business_id": normID,
		"reason":      reason,
	})
	return b, nil
}

// ReactivateBusiness lifts administrative suspension of a business (ADMIN only).
func (s *Service) ReactivateBusiness(ctx context.Context, p actor.Principal, businessID string) (domain.Business, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "reactivate_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Business{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "reactivate_business", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.Business{}, domain.ErrNotFound
	}

	b, err := s.repo.ReactivateBusiness(ctx, normID)
	if err != nil {
		s.audit(ctx, "reactivate_business", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.Business{}, err
	}

	s.audit(ctx, "reactivate_business", "SUCCESS", &p.AccountID, nil, "business reactivated", map[string]any{
		"business_id": normID,
	})
	return b, nil
}

// ArchiveBusiness logically archives a business (BE-ORG-06, ADMIN only).
func (s *Service) ArchiveBusiness(ctx context.Context, p actor.Principal, businessID string) error {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "archive_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "archive_business", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.ErrNotFound
	}

	if err := s.repo.ArchiveBusiness(ctx, normID); err != nil {
		s.audit(ctx, "archive_business", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return err
	}

	s.audit(ctx, "archive_business", "SUCCESS", &p.AccountID, nil, "business archived", map[string]any{
		"business_id": normID,
	})
	return nil
}

// UnarchiveBusiness restores an archived business (BE-ORG-07, ADMIN only).
func (s *Service) UnarchiveBusiness(ctx context.Context, p actor.Principal, businessID string) (domain.Business, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "unarchive_business", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Business{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "unarchive_business", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.Business{}, domain.ErrNotFound
	}

	b, err := s.repo.UnarchiveBusiness(ctx, normID)
	if err != nil {
		s.audit(ctx, "unarchive_business", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.Business{}, err
	}

	s.audit(ctx, "unarchive_business", "SUCCESS", &p.AccountID, nil, "business unarchived", map[string]any{
		"business_id": normID,
	})
	return b, nil
}

// ListBranchRequests returns the inbox of branch requests (ADMIN and SOPORTE).
func (s *Service) ListBranchRequests(ctx context.Context, p actor.Principal, filter ports.BranchRequestFilter) ([]domain.BranchRequestItem, int, error) {
	if p.Actor != actor.RoleAdmin && p.Actor != actor.RoleSoporte {
		s.audit(ctx, "list_branch_requests", "REFUSED", &p.AccountID, nil, "actor is not ADMIN or SOPORTE", nil)
		return nil, 0, domain.ErrForbidden
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	status := strings.ToUpper(strings.TrimSpace(filter.Status))
	if status != "" && status != "PENDING" && status != "APPROVED" && status != "REJECTED" {
		s.audit(ctx, "list_branch_requests", "REFUSED", &p.AccountID, nil, "invalid status filter", nil)
		return nil, 0, domain.ErrValidation
	}
	filter.Status = status

	return s.repo.ListBranchRequests(ctx, filter)
}

// GetBranchRequest returns detail and history of a branch request (ADMIN and SOPORTE).
func (s *Service) GetBranchRequest(ctx context.Context, p actor.Principal, requestID string) (domain.BranchRequestDetail, error) {
	if p.Actor != actor.RoleAdmin && p.Actor != actor.RoleSoporte {
		s.audit(ctx, "get_branch_request", "REFUSED", &p.AccountID, nil, "actor is not ADMIN or SOPORTE", nil)
		return domain.BranchRequestDetail{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(requestID)
	if !ok {
		s.audit(ctx, "get_branch_request", "REFUSED", &p.AccountID, nil, "invalid requestId UUID", nil)
		return domain.BranchRequestDetail{}, domain.ErrNotFound
	}

	return s.repo.GetBranchRequestDetail(ctx, normID)
}

// ApproveBranchRequest approves a pending branch request (BE-ORG-02, ADMIN and SOPORTE).
func (s *Service) ApproveBranchRequest(ctx context.Context, p actor.Principal, requestID string) (domain.BranchApprovalHistory, error) {
	if p.Actor != actor.RoleAdmin && p.Actor != actor.RoleSoporte {
		s.audit(ctx, "approve_branch_request", "REFUSED", &p.AccountID, nil, "actor is not ADMIN or SOPORTE", nil)
		return domain.BranchApprovalHistory{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(requestID)
	if !ok {
		s.audit(ctx, "approve_branch_request", "REFUSED", &p.AccountID, nil, "invalid requestId UUID", nil)
		return domain.BranchApprovalHistory{}, domain.ErrNotFound
	}

	hist, err := s.repo.ApproveBranchRequest(ctx, normID, p.AccountID)
	if err != nil {
		s.audit(ctx, "approve_branch_request", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.BranchApprovalHistory{}, err
	}

	s.audit(ctx, "approve_branch_request", "SUCCESS", &p.AccountID, nil, "branch request approved", map[string]any{
		"request_id": normID,
		"branch_id":  hist.BranchID,
	})
	return hist, nil
}

// RejectBranchRequest rejects a pending branch request with mandatory reason (BE-ORG-02, ADMIN and SOPORTE).
func (s *Service) RejectBranchRequest(ctx context.Context, p actor.Principal, requestID, reason string) (domain.BranchApprovalHistory, error) {
	if p.Actor != actor.RoleAdmin && p.Actor != actor.RoleSoporte {
		s.audit(ctx, "reject_branch_request", "REFUSED", &p.AccountID, nil, "actor is not ADMIN or SOPORTE", nil)
		return domain.BranchApprovalHistory{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(requestID)
	if !ok {
		s.audit(ctx, "reject_branch_request", "REFUSED", &p.AccountID, nil, "invalid requestId UUID", nil)
		return domain.BranchApprovalHistory{}, domain.ErrNotFound
	}
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		s.audit(ctx, "reject_branch_request", "REFUSED", &p.AccountID, nil, "rejection_reason cannot be empty", nil)
		return domain.BranchApprovalHistory{}, domain.ErrValidation
	}

	hist, err := s.repo.RejectBranchRequest(ctx, normID, p.AccountID, trimmedReason)
	if err != nil {
		s.audit(ctx, "reject_branch_request", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.BranchApprovalHistory{}, err
	}

	s.audit(ctx, "reject_branch_request", "SUCCESS", &p.AccountID, nil, "branch request rejected: "+trimmedReason, map[string]any{
		"request_id": normID,
		"branch_id":  hist.BranchID,
		"reason":     trimmedReason,
	})
	return hist, nil
}

// ListBranches returns global branch summaries (ADMIN and SOPORTE).
func (s *Service) ListBranches(ctx context.Context, p actor.Principal, filter ports.BranchFilter) ([]domain.BranchView, int, error) {
	if p.Actor != actor.RoleAdmin && p.Actor != actor.RoleSoporte {
		s.audit(ctx, "list_branches", "REFUSED", &p.AccountID, nil, "actor is not ADMIN or SOPORTE", nil)
		return nil, 0, domain.ErrForbidden
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.BusinessID != "" {
		if norm, ok := actor.NormalizeUUID(filter.BusinessID); ok {
			filter.BusinessID = norm
		} else {
			return nil, 0, domain.ErrValidation
		}
	}
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	if filter.Status != "" && filter.Status != "all" && filter.Status != "active" && filter.Status != "suspended" && filter.Status != "archived" {
		s.audit(ctx, "list_branches", "REFUSED", &p.AccountID, nil, "invalid status filter", nil)
		return nil, 0, domain.ErrValidation
	}

	return s.repo.ListBranches(ctx, filter)
}

// SuspendBranch administratively suspends an individual branch (ADMIN only).
func (s *Service) SuspendBranch(ctx context.Context, p actor.Principal, branchID, reason string) (domain.Branch, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "suspend_branch", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Branch{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(branchID)
	if !ok {
		s.audit(ctx, "suspend_branch", "REFUSED", &p.AccountID, nil, "invalid branchId UUID", nil)
		return domain.Branch{}, domain.ErrNotFound
	}
	if strings.TrimSpace(reason) == "" {
		s.audit(ctx, "suspend_branch", "REFUSED", &p.AccountID, nil, "reason is required", nil)
		return domain.Branch{}, domain.ErrValidation
	}

	b, err := s.repo.SuspendBranch(ctx, normID)
	if err != nil {
		s.audit(ctx, "suspend_branch", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.Branch{}, err
	}

	s.audit(ctx, "suspend_branch", "SUCCESS", &p.AccountID, nil, "branch suspended: "+reason, map[string]any{
		"branch_id": normID,
		"reason":    reason,
	})
	return b, nil
}

// ReactivateBranch lifts administrative suspension of a branch (ADMIN only).
func (s *Service) ReactivateBranch(ctx context.Context, p actor.Principal, branchID string) (domain.Branch, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "reactivate_branch", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Branch{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(branchID)
	if !ok {
		s.audit(ctx, "reactivate_branch", "REFUSED", &p.AccountID, nil, "invalid branchId UUID", nil)
		return domain.Branch{}, domain.ErrNotFound
	}

	b, err := s.repo.ReactivateBranch(ctx, normID)
	if err != nil {
		s.audit(ctx, "reactivate_branch", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.Branch{}, err
	}

	s.audit(ctx, "reactivate_branch", "SUCCESS", &p.AccountID, nil, "branch reactivated", map[string]any{
		"branch_id": normID,
	})
	return b, nil
}
