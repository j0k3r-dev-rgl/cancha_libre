package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"canchalibre/core/internal/modules/identity/adapters/outbound/postgres"
	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/platform/actor"
	platformpg "canchalibre/core/internal/platform/postgres"
	"canchalibre/core/internal/platform/security"
)

func setupTestDB(t *testing.T) *postgres.Repository {
	t.Helper()
	dbURL := os.Getenv("TEST_IDENTITY_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5435/canchalibre_test_identity?sslmode=disable"
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

func TestWorkAndCustomerAccountsShareEmailIndependently(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	pwWork, err := security.HashPassword("work-password-1234")
	if err != nil {
		t.Fatalf("hash work password: %v", err)
	}
	pwCust, err := security.HashPassword("cust-password-5678")
	if err != nil {
		t.Fatalf("hash cust password: %v", err)
	}

	workAcc, err := repo.CreateAccountWithCredential(ctx, domain.Account{
		Email:  "dual@canchalibre.local",
		Kind:   actor.KindWork,
		Status: domain.StatusActive,
	}, pwWork)
	if err != nil {
		t.Fatalf("create work account: %v", err)
	}

	custAcc, err := repo.CreateAccountWithCredential(ctx, domain.Account{
		Email:  "dual@canchalibre.local",
		Kind:   actor.KindCustomer,
		Status: domain.StatusActive,
	}, pwCust)
	if err != nil {
		t.Fatalf("create customer account with same email: %v", err)
	}

	if workAcc.ID == custAcc.ID {
		t.Fatal("expected distinct account IDs for WORK and CUSTOMER sharing an email")
	}

	// Duplicate (email, kind) must fail with ErrConflict
	_, err = repo.CreateAccountWithCredential(ctx, domain.Account{
		Email:  "DUAL@canchalibre.local",
		Kind:   actor.KindWork,
		Status: domain.StatusActive,
	}, pwWork)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected ErrConflict on duplicate (email, kind), got %v", err)
	}
}

func TestConcurrentRefreshRotationAndFamilyRevocationOnReuse(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	acc, _, err := repo.FindAccountByEmailAndKind(ctx, "admin@canchalibre.local", actor.KindWork)
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}

	raw1, hash1, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	_ = raw1
	sess1, err := repo.CreateRefreshSession(ctx, acc.ID, hash1, time.Now().UTC().Add(security.RefreshTokenTTL))
	if err != nil {
		t.Fatalf("CreateRefreshSession: %v", err)
	}

	// Race two concurrent rotations of hash1
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	newHashes := make([][]byte, 2)
	for i := 0; i < 2; i++ {
		_, h, err := security.NewOpaqueToken()
		if err != nil {
			t.Fatalf("new token %d: %v", i, err)
		}
		newHashes[i] = h
	}

	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.RotateRefreshSession(ctx, hash1, newHashes[i], time.Now().UTC().Add(security.RefreshTokenTTL))
		}()
	}
	close(start)
	wg.Wait()

	winners := 0
	reuseErrs := 0
	var winningIdx int
	for i, e := range errs {
		if e == nil {
			winners++
			winningIdx = i
		} else if errors.Is(e, domain.ErrRefreshReuse) {
			reuseErrs++
		} else {
			t.Fatalf("unexpected error from goroutine %d: %v", i, e)
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 winner in concurrent rotation, got %d (errs: %v)", winners, errs)
	}

	// Now create a fresh session and rotate it sequentially, then replay the old token
	// and verify the entire family (including the new token) is revoked.
	_, freshHash1, _ := security.NewOpaqueToken()
	freshSess, err := repo.CreateRefreshSession(ctx, acc.ID, freshHash1, time.Now().UTC().Add(security.RefreshTokenTTL))
	if err != nil {
		t.Fatalf("create fresh session: %v", err)
	}
	_ = sess1
	_ = winningIdx

	_, freshHash2, _ := security.NewOpaqueToken()
	rotatedSess, err := repo.RotateRefreshSession(ctx, freshHash1, freshHash2, time.Now().UTC().Add(security.RefreshTokenTTL))
	if err != nil {
		t.Fatalf("rotate fresh session: %v", err)
	}
	if rotatedSess.FamilyID != freshSess.FamilyID {
		t.Fatalf("expected family ID %s to be preserved, got %s", freshSess.FamilyID, rotatedSess.FamilyID)
	}

	// Replay freshHash1 -> must return ErrRefreshReuse and revoke the whole family
	_, freshHash3, _ := security.NewOpaqueToken()
	_, err = repo.RotateRefreshSession(ctx, freshHash1, freshHash3, time.Now().UTC().Add(security.RefreshTokenTTL))
	if !errors.Is(err, domain.ErrRefreshReuse) {
		t.Fatalf("expected ErrRefreshReuse on replayed token, got %v", err)
	}

	// Now freshHash2 (the successor in the same family) must also be rejected as unauthenticated (revoked)
	_, freshHash4, _ := security.NewOpaqueToken()
	_, err = repo.RotateRefreshSession(ctx, freshHash2, freshHash4, time.Now().UTC().Add(security.RefreshTokenTTL))
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected successor in revoked family to fail with ErrUnauthenticated, got %v", err)
	}
}
