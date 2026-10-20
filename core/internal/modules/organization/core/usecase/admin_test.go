package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/modules/organization/core/usecase"
	"canchalibre/core/internal/platform/actor"
)

type mockOrgRepo struct {
	ports.Repository

	createBusinessFn         func(ctx context.Context, b domain.Business) (domain.Business, error)
	listBusinessesFn         func(ctx context.Context, filter ports.BusinessFilter) ([]domain.BusinessSummary, int, error)
	getBusinessByIDFn        func(ctx context.Context, businessID string) (domain.Business, []string, []domain.BusinessBranchView, error)
	updateBusinessFn         func(ctx context.Context, businessID string, name, legalName, taxID *string) (domain.Business, error)
	suspendBusinessFn        func(ctx context.Context, businessID string) (domain.Business, error)
	reactivateBusinessFn     func(ctx context.Context, businessID string) (domain.Business, error)
	archiveBusinessFn        func(ctx context.Context, businessID string) error
	unarchiveBusinessFn      func(ctx context.Context, businessID string) (domain.Business, error)
	listBranchRequestsFn     func(ctx context.Context, filter ports.BranchRequestFilter) ([]domain.BranchRequestItem, int, error)
	getBranchRequestDetailFn func(ctx context.Context, requestID string) (domain.BranchRequestDetail, error)
	approveBranchRequestFn   func(ctx context.Context, requestID, reviewedBy string) (domain.BranchApprovalHistory, error)
	rejectBranchRequestFn    func(ctx context.Context, requestID, reviewedBy, reason string) (domain.BranchApprovalHistory, error)
	listBranchesFn           func(ctx context.Context, filter ports.BranchFilter) ([]domain.BranchView, int, error)
	suspendBranchFn          func(ctx context.Context, branchID string) (domain.Branch, error)
	reactivateBranchFn       func(ctx context.Context, branchID string) (domain.Branch, error)
	createMembershipFn       func(ctx context.Context, m domain.Membership) (domain.Membership, error)
}

func (m *mockOrgRepo) CreateBusiness(ctx context.Context, b domain.Business) (domain.Business, error) {
	if m.createBusinessFn != nil {
		return m.createBusinessFn(ctx, b)
	}
	return b, nil
}
func (m *mockOrgRepo) ListBusinesses(ctx context.Context, filter ports.BusinessFilter) ([]domain.BusinessSummary, int, error) {
	if m.listBusinessesFn != nil {
		return m.listBusinessesFn(ctx, filter)
	}
	return nil, 0, nil
}
func (m *mockOrgRepo) GetBusinessByID(ctx context.Context, businessID string) (domain.Business, []string, []domain.BusinessBranchView, error) {
	if m.getBusinessByIDFn != nil {
		return m.getBusinessByIDFn(ctx, businessID)
	}
	return domain.Business{ID: businessID, Name: "Test"}, nil, nil, nil
}
func (m *mockOrgRepo) UpdateBusiness(ctx context.Context, businessID string, name, legalName, taxID *string) (domain.Business, error) {
	if m.updateBusinessFn != nil {
		return m.updateBusinessFn(ctx, businessID, name, legalName, taxID)
	}
	return domain.Business{ID: businessID}, nil
}
func (m *mockOrgRepo) SuspendBusiness(ctx context.Context, businessID string) (domain.Business, error) {
	if m.suspendBusinessFn != nil {
		return m.suspendBusinessFn(ctx, businessID)
	}
	now := time.Now().UTC()
	return domain.Business{ID: businessID, SuspendedAt: &now, Enabled: false}, nil
}
func (m *mockOrgRepo) ReactivateBusiness(ctx context.Context, businessID string) (domain.Business, error) {
	if m.reactivateBusinessFn != nil {
		return m.reactivateBusinessFn(ctx, businessID)
	}
	return domain.Business{ID: businessID, SuspendedAt: nil, Enabled: true}, nil
}
func (m *mockOrgRepo) ArchiveBusiness(ctx context.Context, businessID string) error {
	if m.archiveBusinessFn != nil {
		return m.archiveBusinessFn(ctx, businessID)
	}
	return nil
}
func (m *mockOrgRepo) UnarchiveBusiness(ctx context.Context, businessID string) (domain.Business, error) {
	if m.unarchiveBusinessFn != nil {
		return m.unarchiveBusinessFn(ctx, businessID)
	}
	return domain.Business{ID: businessID, ArchivedAt: nil}, nil
}
func (m *mockOrgRepo) ListBranchRequests(ctx context.Context, filter ports.BranchRequestFilter) ([]domain.BranchRequestItem, int, error) {
	if m.listBranchRequestsFn != nil {
		return m.listBranchRequestsFn(ctx, filter)
	}
	return nil, 0, nil
}
func (m *mockOrgRepo) GetBranchRequestDetail(ctx context.Context, requestID string) (domain.BranchRequestDetail, error) {
	if m.getBranchRequestDetailFn != nil {
		return m.getBranchRequestDetailFn(ctx, requestID)
	}
	return domain.BranchRequestDetail{}, nil
}
func (m *mockOrgRepo) ApproveBranchRequest(ctx context.Context, requestID, reviewedBy string) (domain.BranchApprovalHistory, error) {
	if m.approveBranchRequestFn != nil {
		return m.approveBranchRequestFn(ctx, requestID, reviewedBy)
	}
	now := time.Now().UTC()
	return domain.BranchApprovalHistory{ID: requestID, Status: domain.ApprovalStatusApproved, ReviewedBy: &reviewedBy, ReviewedAt: &now}, nil
}
func (m *mockOrgRepo) RejectBranchRequest(ctx context.Context, requestID, reviewedBy, reason string) (domain.BranchApprovalHistory, error) {
	if m.rejectBranchRequestFn != nil {
		return m.rejectBranchRequestFn(ctx, requestID, reviewedBy, reason)
	}
	now := time.Now().UTC()
	return domain.BranchApprovalHistory{ID: requestID, Status: domain.ApprovalStatusRejected, ReviewedBy: &reviewedBy, ReviewedAt: &now, RejectionReason: &reason}, nil
}
func (m *mockOrgRepo) ListBranches(ctx context.Context, filter ports.BranchFilter) ([]domain.BranchView, int, error) {
	if m.listBranchesFn != nil {
		return m.listBranchesFn(ctx, filter)
	}
	return nil, 0, nil
}
func (m *mockOrgRepo) SuspendBranch(ctx context.Context, branchID string) (domain.Branch, error) {
	if m.suspendBranchFn != nil {
		return m.suspendBranchFn(ctx, branchID)
	}
	now := time.Now().UTC()
	return domain.Branch{ID: branchID, SuspendedAt: &now}, nil
}
func (m *mockOrgRepo) ReactivateBranch(ctx context.Context, branchID string) (domain.Branch, error) {
	if m.reactivateBranchFn != nil {
		return m.reactivateBranchFn(ctx, branchID)
	}
	return domain.Branch{ID: branchID, SuspendedAt: nil}, nil
}
func (m *mockOrgRepo) CreateMembership(ctx context.Context, mem domain.Membership) (domain.Membership, error) {
	if m.createMembershipFn != nil {
		return m.createMembershipFn(ctx, mem)
	}
	return mem, nil
}

type mockIdentityPort struct {
	ports.IdentityPort
	ensureCoOwnerFn  func(ctx context.Context, issuerID, email, businessID string) (string, error)
	recordAuditFn    func(ctx context.Context, event, outcome string, actorID, subjectID *string, reason string, detail map[string]any) error
	getAccountsByIDs func(ctx context.Context, accountIDs []string) ([]domain.BusinessOwnerView, error)
}

func (m *mockIdentityPort) EnsureCoOwnerAccountByEmail(ctx context.Context, issuerID, email, businessID string) (string, error) {
	if m.ensureCoOwnerFn != nil {
		return m.ensureCoOwnerFn(ctx, issuerID, email, businessID)
	}
	return "019294f0-0000-7000-8000-000000000002", nil
}
func (m *mockIdentityPort) RecordAudit(ctx context.Context, event, outcome string, actorID, subjectID *string, reason string, detail map[string]any) error {
	if m.recordAuditFn != nil {
		return m.recordAuditFn(ctx, event, outcome, actorID, subjectID, reason, detail)
	}
	return nil
}
func (m *mockIdentityPort) GetAccountsByIDs(ctx context.Context, accountIDs []string) ([]domain.BusinessOwnerView, error) {
	if m.getAccountsByIDs != nil {
		return m.getAccountsByIDs(ctx, accountIDs)
	}
	return nil, nil
}

func TestBranchIsPublishedCalculation(t *testing.T) {
	br := domain.Branch{
		ApprovalStatus:       domain.ApprovalStatusApproved,
		CoordinatesConfirmed: true,
		Enabled:              true,
		SuspendedAt:          nil,
		ArchivedAt:           nil,
	}

	// All conditions met -> true
	if !br.IsPublished(true, false, false) {
		t.Fatalf("expected is_published to be true when all conditions are met")
	}

	// Branch not approved
	brNotApproved := br
	brNotApproved.ApprovalStatus = domain.ApprovalStatusPending
	if brNotApproved.IsPublished(true, false, false) {
		t.Fatalf("expected is_published to be false when approval is pending")
	}

	// Coordinates not confirmed
	brNoCoords := br
	brNoCoords.CoordinatesConfirmed = false
	if brNoCoords.IsPublished(true, false, false) {
		t.Fatalf("expected is_published to be false when coordinates are not confirmed")
	}

	// Branch disabled by owner
	brDisabled := br
	brDisabled.Enabled = false
	if brDisabled.IsPublished(true, false, false) {
		t.Fatalf("expected is_published to be false when branch is disabled")
	}

	// Branch suspended
	now := time.Now().UTC()
	brSuspended := br
	brSuspended.SuspendedAt = &now
	if brSuspended.IsPublished(true, false, false) {
		t.Fatalf("expected is_published to be false when branch is suspended")
	}

	// Business disabled or suspended
	if br.IsPublished(false, false, false) {
		t.Fatalf("expected is_published to be false when business is disabled")
	}
	if br.IsPublished(true, true, false) {
		t.Fatalf("expected is_published to be false when business is suspended")
	}
}

func TestCreateBusiness(t *testing.T) {
	repo := &mockOrgRepo{}
	idPort := &mockIdentityPort{}
	svc := usecase.NewService(repo, idPort)

	adminP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin}
	ownerP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000002", Actor: actor.RoleOwner}

	// Non-admin refused
	_, err := svc.CreateBusiness(context.Background(), ownerP, usecase.CreateBusinessInput{Name: "Club San Martin"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin, got %v", err)
	}

	// Empty name refused
	_, err = svc.CreateBusiness(context.Background(), adminP, usecase.CreateBusinessInput{Name: "   "})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for empty name, got %v", err)
	}

	// Invalid owner email refused
	_, err = svc.CreateBusiness(context.Background(), adminP, usecase.CreateBusinessInput{
		Name:        "Club San Martin",
		OwnerEmails: []string{"not-an-email"},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for invalid email, got %v", err)
	}

	// Valid creation
	detail, err := svc.CreateBusiness(context.Background(), adminP, usecase.CreateBusinessInput{
		Name:        "Club San Martin",
		OwnerEmails: []string{"owner@canchalibre.local"},
	})
	if err != nil {
		t.Fatalf("unexpected error on valid creation: %v", err)
	}
	if detail.Name != "Club San Martin" || !detail.Enabled || len(detail.Owners) != 1 {
		t.Fatalf("unexpected business detail: %+v", detail)
	}
}

func TestSuspendAndReactivateBusiness(t *testing.T) {
	repo := &mockOrgRepo{}
	svc := usecase.NewService(repo, &mockIdentityPort{})

	adminP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin}
	ownerP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000002", Actor: actor.RoleOwner}
	busID := "019294f0-0000-7000-8000-000000000101"

	// Non-admin refused
	_, err := svc.SuspendBusiness(context.Background(), ownerP, busID, "reason")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin, got %v", err)
	}

	// Missing reason refused
	_, err = svc.SuspendBusiness(context.Background(), adminP, busID, "   ")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for empty reason, got %v", err)
	}

	// Success suspend
	b, err := svc.SuspendBusiness(context.Background(), adminP, busID, "Violation of terms")
	if err != nil || b.SuspendedAt == nil || b.Enabled {
		t.Fatalf("expected business suspended, got b=%+v err=%v", b, err)
	}

	// Success reactivate
	b2, err := svc.ReactivateBusiness(context.Background(), adminP, busID)
	if err != nil || b2.SuspendedAt != nil || !b2.Enabled {
		t.Fatalf("expected business reactivated, got b2=%+v err=%v", b2, err)
	}
}

func TestBranchApprovalLifecycle(t *testing.T) {
	repo := &mockOrgRepo{}
	svc := usecase.NewService(repo, &mockIdentityPort{})

	adminP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin}
	soporteP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000005", Actor: actor.RoleSoporte}
	playerP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000004", Actor: actor.RolePlayer}
	reqID := "019294f0-0000-7000-8000-000000000301"

	// Player cannot approve or reject
	_, err := svc.ApproveBranchRequest(context.Background(), playerP, reqID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for PLAYER on approve, got %v", err)
	}
	_, err = svc.RejectBranchRequest(context.Background(), playerP, reqID, "reason")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for PLAYER on reject, got %v", err)
	}

	// Rejection requires non-empty reason
	_, err = svc.RejectBranchRequest(context.Background(), soporteP, reqID, "   ")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for empty rejection reason, got %v", err)
	}

	// SOPORTE can reject with reason
	hist, err := svc.RejectBranchRequest(context.Background(), soporteP, reqID, "Dirección no verificable")
	if err != nil || hist.Status != domain.ApprovalStatusRejected || hist.RejectionReason == nil || *hist.RejectionReason != "Dirección no verificable" {
		t.Fatalf("expected rejected history, got hist=%+v err=%v", hist, err)
	}

	// ADMIN can approve
	hist2, err := svc.ApproveBranchRequest(context.Background(), adminP, reqID)
	if err != nil || hist2.Status != domain.ApprovalStatusApproved {
		t.Fatalf("expected approved history, got hist2=%+v err=%v", hist2, err)
	}
}
