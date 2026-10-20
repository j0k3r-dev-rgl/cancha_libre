package postgres_test

import (
	"context"
	"errors"
	"testing"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/ports"
	"canchalibre/core/internal/platform/actor"
)

func TestAdminUserManagementPostgres(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	// 1. List users (seeded: admin, owner, employee, player, soporte = 5)
	users, total, err := repo.ListUsers(ctx, ports.UserFilter{})
	if err != nil {
		t.Fatalf("list users failed: %v", err)
	}
	if total != 5 || len(users) != 5 {
		t.Fatalf("expected 5 seeded users, got total=%d len=%d", total, len(users))
	}

	// Filter by kind
	custUsers, custTotal, err := repo.ListUsers(ctx, ports.UserFilter{Kind: string(actor.KindCustomer)})
	if err != nil || custTotal != 1 || len(custUsers) != 1 {
		t.Fatalf("expected 1 customer user, got total=%d len=%d err=%v", custTotal, len(custUsers), err)
	}
	if custUsers[0].Email != "player@canchalibre.local" {
		t.Fatalf("expected player email, got %s", custUsers[0].Email)
	}

	// Filter by search
	searchUsers, searchTotal, err := repo.ListUsers(ctx, ports.UserFilter{Search: "admin@"})
	if err != nil || searchTotal != 1 || len(searchUsers) != 1 {
		t.Fatalf("expected 1 user searching 'admin@', got total=%d len=%d err=%v", searchTotal, len(searchUsers), err)
	}

	// 2. Get account detail
	adminUser := searchUsers[0]
	detail, err := repo.GetAccountDetail(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("get account detail failed: %v", err)
	}
	if detail.ID != adminUser.ID || detail.Email != "admin@canchalibre.local" || detail.Status != domain.StatusActive {
		t.Fatalf("unexpected account detail: %+v", detail)
	}

	// 3. Suspend account (use player account)
	playerID := custUsers[0].ID
	if err := repo.SuspendAccount(ctx, playerID); err != nil {
		t.Fatalf("suspend account failed: %v", err)
	}
	// Suspending already suspended account -> ErrConflict
	if err := repo.SuspendAccount(ctx, playerID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict when suspending already suspended account, got %v", err)
	}

	// Verify status is SUSPENDED
	playerDetail, err := repo.GetAccountDetail(ctx, playerID)
	if err != nil || playerDetail.Status != domain.StatusSuspended {
		t.Fatalf("expected status SUSPENDED, got status=%s err=%v", playerDetail.Status, err)
	}

	// 4. Reactivate account
	if err := repo.ReactivateAccount(ctx, playerID); err != nil {
		t.Fatalf("reactivate account failed: %v", err)
	}
	// Reactivating active account -> ErrConflict
	if err := repo.ReactivateAccount(ctx, playerID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict when reactivating active account, got %v", err)
	}

	// Verify status is ACTIVE
	playerDetail2, err := repo.GetAccountDetail(ctx, playerID)
	if err != nil || playerDetail2.Status != domain.StatusActive {
		t.Fatalf("expected status ACTIVE, got status=%s err=%v", playerDetail2.Status, err)
	}

	// Non-existent account returns ErrNotFound
	nonExistentID, _ := actor.NewUUIDv7()
	if _, err := repo.GetAccountDetail(ctx, nonExistentID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for get non-existent, got %v", err)
	}
	if err := repo.SuspendAccount(ctx, nonExistentID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for suspend non-existent, got %v", err)
	}
	if err := repo.ReactivateAccount(ctx, nonExistentID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for reactivate non-existent, got %v", err)
	}
}
