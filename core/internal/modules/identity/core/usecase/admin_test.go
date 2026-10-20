package usecase_test

import (
	"context"
	"errors"
	"testing"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/ports"
	"canchalibre/core/internal/modules/identity/core/usecase"
	"canchalibre/core/internal/platform/actor"
)

type mockIdentityRepo struct {
	ports.Repository

	listUsersFn        func(ctx context.Context, filter ports.UserFilter) ([]domain.UserSummary, int, error)
	getAccountDetailFn func(ctx context.Context, accountID string) (domain.Account, error)
	findAccountByIDFn  func(ctx context.Context, accountID string) (domain.Account, error)
	suspendAccountFn   func(ctx context.Context, accountID string) error
	reactivateAccountFn func(ctx context.Context, accountID string) error
	revokeSessionsFn   func(ctx context.Context, accountID string) error
	appendAuditFn      func(ctx context.Context, entry domain.AuditEntry) error
}

func (m *mockIdentityRepo) ListUsers(ctx context.Context, filter ports.UserFilter) ([]domain.UserSummary, int, error) {
	if m.listUsersFn != nil {
		return m.listUsersFn(ctx, filter)
	}
	return nil, 0, nil
}
func (m *mockIdentityRepo) GetAccountDetail(ctx context.Context, accountID string) (domain.Account, error) {
	if m.getAccountDetailFn != nil {
		return m.getAccountDetailFn(ctx, accountID)
	}
	return domain.Account{ID: accountID, Email: "test@canchalibre.local", Status: domain.StatusActive}, nil
}
func (m *mockIdentityRepo) FindAccountByID(ctx context.Context, accountID string) (domain.Account, error) {
	if m.findAccountByIDFn != nil {
		return m.findAccountByIDFn(ctx, accountID)
	}
	return domain.Account{ID: accountID, Email: "test@canchalibre.local", Status: domain.StatusActive}, nil
}
func (m *mockIdentityRepo) SuspendAccount(ctx context.Context, accountID string) error {
	if m.suspendAccountFn != nil {
		return m.suspendAccountFn(ctx, accountID)
	}
	return nil
}
func (m *mockIdentityRepo) ReactivateAccount(ctx context.Context, accountID string) error {
	if m.reactivateAccountFn != nil {
		return m.reactivateAccountFn(ctx, accountID)
	}
	return nil
}
func (m *mockIdentityRepo) RevokeRefreshSessionsByAccount(ctx context.Context, accountID string) error {
	if m.revokeSessionsFn != nil {
		return m.revokeSessionsFn(ctx, accountID)
	}
	return nil
}
func (m *mockIdentityRepo) AppendAudit(ctx context.Context, entry domain.AuditEntry) error {
	if m.appendAuditFn != nil {
		return m.appendAuditFn(ctx, entry)
	}
	return nil
}

type mockScopePort struct {
	ports.ScopePort
	listMembershipsFn func(ctx context.Context, accountID string) ([]domain.UserMembershipView, error)
}

func (m *mockScopePort) ListUserMemberships(ctx context.Context, accountID string) ([]domain.UserMembershipView, error) {
	if m.listMembershipsFn != nil {
		return m.listMembershipsFn(ctx, accountID)
	}
	return nil, nil
}

func TestListUsers(t *testing.T) {
	repo := &mockIdentityRepo{
		listUsersFn: func(ctx context.Context, filter ports.UserFilter) ([]domain.UserSummary, int, error) {
			return []domain.UserSummary{
				{ID: "019294f0-0000-7000-8000-000000000001", Email: "admin@canchalibre.local", Kind: actor.KindWork, Status: domain.StatusActive},
			}, 1, nil
		},
	}
	svc := usecase.NewService(repo, &mockScopePort{}, nil, usecase.Config{JWTSecret: []byte("01234567890123456789012345678901")})

	adminP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin}
	ownerP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000002", Actor: actor.RoleOwner}

	// Non-admin refused
	_, _, err := svc.ListUsers(context.Background(), ownerP, ports.UserFilter{})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin, got %v", err)
	}

	// Invalid kind filter refused
	_, _, err = svc.ListUsers(context.Background(), adminP, ports.UserFilter{Kind: "INVALID"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for invalid kind, got %v", err)
	}

	// Valid listing
	items, total, err := svc.ListUsers(context.Background(), adminP, ports.UserFilter{})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 item, got total=%d items=%+v err=%v", total, items, err)
	}
}

func TestSuspendAndReactivateUser(t *testing.T) {
	repo := &mockIdentityRepo{}
	svc := usecase.NewService(repo, &mockScopePort{}, nil, usecase.Config{JWTSecret: []byte("01234567890123456789012345678901")})

	adminP := actor.Principal{AccountID: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin}
	targetUserID := "019294f0-0000-7000-8000-000000000002"

	// Self-suspension refused
	err := svc.SuspendUser(context.Background(), adminP, adminP.AccountID, "reason")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for self-suspension, got %v", err)
	}

	// Empty reason refused
	err = svc.SuspendUser(context.Background(), adminP, targetUserID, "   ")
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected ErrValidation for empty reason, got %v", err)
	}

	// Suspending already suspended account -> conflict
	repo.findAccountByIDFn = func(ctx context.Context, id string) (domain.Account, error) {
		return domain.Account{ID: id, Status: domain.StatusSuspended}, nil
	}
	err = svc.SuspendUser(context.Background(), adminP, targetUserID, "Violation")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict when already suspended, got %v", err)
	}

	// Suspending active account -> success and revokes sessions
	sessionsRevoked := false
	repo.revokeSessionsFn = func(ctx context.Context, id string) error {
		sessionsRevoked = true
		return nil
	}
	repo.findAccountByIDFn = func(ctx context.Context, id string) (domain.Account, error) {
		return domain.Account{ID: id, Status: domain.StatusActive}, nil
	}
	err = svc.SuspendUser(context.Background(), adminP, targetUserID, "Violation of terms")
	if err != nil || !sessionsRevoked {
		t.Fatalf("expected success and sessions revoked, got err=%v sessionsRevoked=%v", err, sessionsRevoked)
	}

	// Reactivating non-suspended account -> conflict
	err = svc.ReactivateUser(context.Background(), adminP, targetUserID)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict when reactivating non-suspended account, got %v", err)
	}

	// Reactivating suspended account -> success
	repo.findAccountByIDFn = func(ctx context.Context, id string) (domain.Account, error) {
		return domain.Account{ID: id, Status: domain.StatusSuspended}, nil
	}
	err = svc.ReactivateUser(context.Background(), adminP, targetUserID)
	if err != nil {
		t.Fatalf("expected success reactivating suspended account, got %v", err)
	}
}
