package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"canchalibre/core/internal/modules/organization/adapters/outbound/postgres"
	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/platform/actor"
	platformpg "canchalibre/core/internal/platform/postgres"
)

const (
	seededBusinessB1     = "019294f0-0000-7000-8000-000000000101"
	seededBusinessB2     = "019294f0-0000-7000-8000-000000000102"
	seededBranchCentral  = "019294f0-0000-7000-8000-000000000201"
	seededBranchNorte    = "019294f0-0000-7000-8000-000000000202"
	seededBranchB2Sur    = "019294f0-0000-7000-8000-000000000203"
	seededOwnerAccountID = "019294f0-0000-7000-8000-000000000002"
	seededEmpAccountID   = "019294f0-0000-7000-8000-000000000003"
	seededSoporteAccID   = "019294f0-0000-7000-8000-000000000005"
)

func setupOrgTestDB(t *testing.T) *postgres.Repository {
	t.Helper()
	dbURL := os.Getenv("TEST_ORG_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5435/canchalibre_test_org?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	accountsFile := filepath.Join(tmpDir, "accounts.json")
	accountsJSON := `[
  {"role":"ADMIN","email":"admin@canchalibre.local","password":"dev-only-change-me-admin"},
  {"role":"OWNER","email":"owner@canchalibre.local","password":"dev-only-change-me-owner"},
  {"role":"EMPLOYEE","email":"employee@canchalibre.local","password":"dev-only-change-me-employee"},
  {"role":"PLAYER","email":"player@canchalibre.local","password":"dev-only-change-me-player"},
  {"role":"SOPORTE","email":"soporte@canchalibre.local","password":"dev-only-change-me-soporte"}
]`
	if err := os.WriteFile(accountsFile, []byte(accountsJSON), 0o600); err != nil {
		t.Fatalf("write accounts: %v", err)
	}
	topoPath := "dev/seed.topology.json"
	pool, err := platformpg.ResetAndConnect(ctx, platformpg.ResetOptions{
		DatabaseURL:      dbURL,
		SeedTopologyPath: topoPath,
		AccountsFilePath: accountsFile,
	})
	if err != nil {
		t.Fatalf("ResetAndConnect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return postgres.NewRepository(pool)
}

func TestGuardVerifyScopeAndRefusalMatrix(t *testing.T) {
	repo := setupOrgTestDB(t)
	ctx := context.Background()

	// OWNER of B1 is authorized on both B1-CENTRAL and B1-NORTE
	for _, br := range []string{seededBranchCentral, seededBranchNorte} {
		role, err := repo.VerifyScope(ctx, seededOwnerAccountID, seededBusinessB1, br)
		if err != nil || role != actor.RoleOwner {
			t.Fatalf("expected OWNER authorized on %s, got role=%s err=%v", br, role, err)
		}
	}

	// OWNER of B1 is refused on B2-SUR (and even if passing B1 business with B2-SUR branch)
	if _, err := repo.VerifyScope(ctx, seededOwnerAccountID, seededBusinessB2, seededBranchB2Sur); !errors.Is(err, domain.ErrContextNotAuthorized) {
		t.Fatalf("expected OWNER refused on B2, got %v", err)
	}
	if _, err := repo.VerifyScope(ctx, seededOwnerAccountID, seededBusinessB1, seededBranchB2Sur); !errors.Is(err, domain.ErrContextNotAuthorized) {
		t.Fatalf("expected OWNER refused when pairing B1 with B2-SUR branch, got %v", err)
	}

	// EMPLOYEE is authorized on B1-CENTRAL and refused on B1-NORTE and B2-SUR
	role, err := repo.VerifyScope(ctx, seededEmpAccountID, seededBusinessB1, seededBranchCentral)
	if err != nil || role != actor.RoleEmployee {
		t.Fatalf("expected EMPLOYEE authorized on B1-CENTRAL, got role=%s err=%v", role, err)
	}
	if _, err := repo.VerifyScope(ctx, seededEmpAccountID, seededBusinessB1, seededBranchNorte); !errors.Is(err, domain.ErrContextNotAuthorized) {
		t.Fatalf("expected EMPLOYEE refused on B1-NORTE, got %v", err)
	}
}

func TestConcurrentCoOwnerRevocationProtectsLastCoOwner(t *testing.T) {
	repo := setupOrgTestDB(t)
	ctx := context.Background()

	// Add a second co-owner to B1
	secondOwnerAccID := "019294f0-0000-7000-8000-000000000099"
	_, err := repo.CreateMembership(ctx, domain.Membership{
		AccountID:  secondOwnerAccID,
		BusinessID: seededBusinessB1,
		Role:       actor.RoleOwner,
	})
	if err != nil {
		t.Fatalf("create second co-owner: %v", err)
	}

	// Race revoking both co-owners concurrently: exactly one must succeed, and the
	// other must fail with ErrLastCoOwner.
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	targets := []string{seededOwnerAccountID, secondOwnerAccID}
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			errs[i] = repo.RevokeCoOwner(ctx, seededBusinessB1, targets[i])
		}()
	}
	close(start)
	wg.Wait()

	successes := 0
	lastOwnerErrs := 0
	for i, e := range errs {
		if e == nil {
			successes++
		} else if errors.Is(e, domain.ErrLastCoOwner) {
			lastOwnerErrs++
		} else {
			t.Fatalf("unexpected error from goroutine %d: %v", i, e)
		}
	}
	if successes != 1 || lastOwnerErrs != 1 {
		t.Fatalf("expected 1 success and 1 ErrLastCoOwner, got successes=%d lastOwnerErrs=%d", successes, lastOwnerErrs)
	}
}
