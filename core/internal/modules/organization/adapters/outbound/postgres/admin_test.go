package postgres_test

import (
	"context"
	"errors"
	"testing"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/platform/actor"
)

func TestBusinessLifecyclePostgres(t *testing.T) {
	repo := setupOrgTestDB(t)
	ctx := context.Background()

	// 1. Create a business
	busID, err := actor.NewUUIDv7()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}
	legalName := "Club Belgrano SRL"
	taxID := "30-11223344-5"
	created, err := repo.CreateBusiness(ctx, domain.Business{
		ID:        busID,
		Name:      "Club Belgrano",
		LegalName: &legalName,
		TaxID:     &taxID,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create business failed: %v", err)
	}
	if created.ID != busID || created.Name != "Club Belgrano" || !created.Enabled {
		t.Fatalf("unexpected created business: %+v", created)
	}

	// 2. List businesses
	items, total, err := repo.ListBusinesses(ctx, ports.BusinessFilter{Status: "all"})
	if err != nil {
		t.Fatalf("list businesses failed: %v", err)
	}
	// Seeded B1, B2 + newly created = 3
	if total < 3 || len(items) < 3 {
		t.Fatalf("expected at least 3 businesses, got total=%d items=%d", total, len(items))
	}

	// Search filter
	searchItems, searchTotal, err := repo.ListBusinesses(ctx, ports.BusinessFilter{Search: "Belgrano"})
	if err != nil || searchTotal != 1 || len(searchItems) != 1 {
		t.Fatalf("expected 1 search result for 'Belgrano', got total=%d items=%d err=%v", searchTotal, len(searchItems), err)
	}

	// 3. Get business by ID
	busDetail, ownerIDs, branches, err := repo.GetBusinessByID(ctx, busID)
	if err != nil {
		t.Fatalf("get business by id failed: %v", err)
	}
	if busDetail.ID != busID || len(ownerIDs) != 0 || len(branches) != 0 {
		t.Fatalf("unexpected detail: bus=%+v owners=%v branches=%v", busDetail, ownerIDs, branches)
	}

	// 4. Update business
	newName := "Club Belgrano Actualizado"
	updated, err := repo.UpdateBusiness(ctx, busID, &newName, nil, nil)
	if err != nil || updated.Name != newName {
		t.Fatalf("update business failed: %v, updated=%+v", err, updated)
	}

	// 5. Suspend business
	suspended, err := repo.SuspendBusiness(ctx, busID)
	if err != nil || suspended.SuspendedAt == nil || suspended.Enabled {
		t.Fatalf("suspend business failed: %v, suspended=%+v", err, suspended)
	}
	// Suspending already suspended business -> ErrAlreadySuspended
	if _, err := repo.SuspendBusiness(ctx, busID); !errors.Is(err, domain.ErrAlreadySuspended) {
		t.Fatalf("expected ErrAlreadySuspended, got %v", err)
	}

	// 6. Reactivate business
	reactivated, err := repo.ReactivateBusiness(ctx, busID)
	if err != nil || reactivated.SuspendedAt != nil || !reactivated.Enabled {
		t.Fatalf("reactivate business failed: %v, reactivated=%+v", err, reactivated)
	}
	// Reactivating active business -> ErrNotSuspended
	if _, err := repo.ReactivateBusiness(ctx, busID); !errors.Is(err, domain.ErrNotSuspended) {
		t.Fatalf("expected ErrNotSuspended, got %v", err)
	}

	// 7. Archive and unarchive business
	if err := repo.ArchiveBusiness(ctx, busID); err != nil {
		t.Fatalf("archive business failed: %v", err)
	}
	// Archiving already archived -> ErrAlreadyArchived
	if err := repo.ArchiveBusiness(ctx, busID); !errors.Is(err, domain.ErrAlreadyArchived) {
		t.Fatalf("expected ErrAlreadyArchived, got %v", err)
	}

	unarchived, err := repo.UnarchiveBusiness(ctx, busID)
	if err != nil || unarchived.ArchivedAt != nil {
		t.Fatalf("unarchive business failed: %v, unarchived=%+v", err, unarchived)
	}
}

func TestBranchRequestsLifecyclePostgres(t *testing.T) {
	repo := setupOrgTestDB(t)
	ctx := context.Background()

	reqID, err := actor.NewUUIDv7()
	if err != nil {
		t.Fatalf("uuid: %v", err)
	}

	// We can test ListBranchRequests against the seeded state:
	items, total, err := repo.ListBranchRequests(ctx, ports.BranchRequestFilter{})
	if err != nil {
		t.Fatalf("list branch requests failed: %v", err)
	}
	_ = items
	_ = total

	// Non-existent request returns ErrNotFound
	if _, err := repo.GetBranchRequestDetail(ctx, reqID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent request, got %v", err)
	}
	if _, err := repo.ApproveBranchRequest(ctx, reqID, seededSoporteAccID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent approve, got %v", err)
	}
	if _, err := repo.RejectBranchRequest(ctx, reqID, seededSoporteAccID, "bad address"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent reject, got %v", err)
	}

	// Branch supervision: List seeded branches
	branches, bTotal, err := repo.ListBranches(ctx, ports.BranchFilter{Status: "all"})
	if err != nil || bTotal < 3 || len(branches) < 3 {
		t.Fatalf("expected at least 3 seeded branches, got total=%d len=%d err=%v", bTotal, len(branches), err)
	}

	// Suspend and reactivate a seeded branch
	brSuspended, err := repo.SuspendBranch(ctx, seededBranchCentral)
	if err != nil || brSuspended.SuspendedAt == nil {
		t.Fatalf("suspend branch failed: %v, br=%+v", err, brSuspended)
	}
	brReactivated, err := repo.ReactivateBranch(ctx, seededBranchCentral)
	if err != nil || brReactivated.SuspendedAt != nil {
		t.Fatalf("reactivate branch failed: %v, br=%+v", err, brReactivated)
	}
}
