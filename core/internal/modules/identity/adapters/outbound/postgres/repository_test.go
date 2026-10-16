package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/modules/identity/adapters/outbound/postgres"
	"canchalibre/core/internal/modules/identity/core/domain"
	identityusecase "canchalibre/core/internal/modules/identity/core/usecase"
	"canchalibre/core/internal/platform/actor"
	platformpg "canchalibre/core/internal/platform/postgres"
	"canchalibre/core/internal/platform/security"
)

// setupTestDB resets and seeds the identity test database and returns only the
// repository. The pool-returning variant is used by tests that must assert the
// observable database state, which is where the F2 invariant is enforced.
func setupTestDB(t *testing.T) *postgres.Repository {
	t.Helper()
	repo, _ := setupTestDBWithPool(t)
	return repo
}

func setupTestDBWithPool(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
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
	return postgres.NewRepository(pool), pool
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

// newIdentityService builds the real identity use-case service over the test
// repository, so the observable activation outcome is asserted through the
// public service and not only through the repository call.
func newIdentityService(repo *postgres.Repository) *identityusecase.Service {
	return identityusecase.NewService(repo, nil, nil, identityusecase.Config{
		JWTSecret: []byte("0123456789abcdef0123456789abcdef"),
	})
}

// countLiveInvitations counts the rows a client can still activate for one
// target: unconsumed, not retired and not expired. It is the database fact the
// F2 rule makes observable.
func countLiveInvitations(t *testing.T, pool *pgxpool.Pool, email string, role actor.Role, businessID, branchID *string) int {
	t.Helper()
	var live int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM schema_identity.invitation
		WHERE lower(email) = lower($1)
		  AND intended_role = $2
		  AND consumed_at IS NULL
		  AND retired_at IS NULL
		  AND expires_at > now()
		  AND business_id IS NOT DISTINCT FROM $3::uuid
		  AND branch_id IS NOT DISTINCT FROM $4::uuid
	`, email, string(role), businessID, branchID).Scan(&live); err != nil {
		t.Fatalf("count live invitations: %v", err)
	}
	return live
}

// TestConcurrentDuplicateInvitationsAreRefusedByTheDatabase proves finding F2:
// the "at most one unconsumed invitation per (email, intended_role, business_id,
// branch_id)" rule is a database fact, not a read-then-write check that two
// concurrent requests can both pass. Exactly one concurrent insert succeeds,
// exactly one live invitation exists for the target, and every loser observes
// domain.ErrConflict, which the inbound adapter maps to the documented 409.
func TestConcurrentDuplicateInvitationsAreRefusedByTheDatabase(t *testing.T) {
	repo, pool := setupTestDBWithPool(t)
	ctx := context.Background()

	businessID := "019294f0-0000-7000-8000-000000000101"
	branchID := "019294f0-0000-7000-8000-000000000201"

	cases := []struct {
		name       string
		email      string
		role       actor.Role
		businessID *string
		branchID   *string
	}{
		{
			name:       "EMPLOYEE invitation scoped to a business and branch",
			email:      "race-employee@canchalibre.local",
			role:       actor.RoleEmployee,
			businessID: &businessID,
			branchID:   &branchID,
		},
		{
			name:  "business-less SOPORTE invitation",
			email: "race-soporte@canchalibre.local",
			role:  actor.RoleSoporte,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const concurrency = 2
			hashes := make([][]byte, concurrency)
			for i := range hashes {
				_, h, err := security.NewOpaqueToken()
				if err != nil {
					t.Fatalf("new token %d: %v", i, err)
				}
				hashes[i] = h
			}

			errs := make([]error, concurrency)
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(concurrency)
			for i := 0; i < concurrency; i++ {
				i := i
				go func() {
					defer wg.Done()
					<-start
					_, errs[i] = repo.CreateInvitation(ctx, domain.Invitation{
						TokenHash:    hashes[i],
						Email:        tc.email,
						IntendedRole: tc.role,
						BusinessID:   tc.businessID,
						BranchID:     tc.branchID,
						ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
					})
				}()
			}
			close(start)
			wg.Wait()

			winners := 0
			for i, err := range errs {
				switch {
				case err == nil:
					winners++
				case errors.Is(err, domain.ErrConflict):
					// Documented loser outcome: the inbound adapter maps it to 409.
				default:
					t.Fatalf("goroutine %d got %v, want nil or domain.ErrConflict", i, err)
				}
			}
			if winners != 1 {
				t.Fatalf("concurrent insert winners = %d, want exactly 1 (errs: %v)", winners, errs)
			}

			if live := countLiveInvitations(t, pool, tc.email, tc.role, tc.businessID, tc.branchID); live != 1 {
				t.Fatalf("live invitations for the target = %d, want exactly 1", live)
			}

			// The serialized repeat is the same conflict without concurrency.
			_, dupHash, err := security.NewOpaqueToken()
			if err != nil {
				t.Fatalf("new duplicate token: %v", err)
			}
			if _, err := repo.CreateInvitation(ctx, domain.Invitation{
				TokenHash:    dupHash,
				Email:        tc.email,
				IntendedRole: tc.role,
				BusinessID:   tc.businessID,
				BranchID:     tc.branchID,
				ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
			}); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("serialized duplicate insert = %v, want domain.ErrConflict", err)
			}
			if live := countLiveInvitations(t, pool, tc.email, tc.role, tc.businessID, tc.branchID); live != 1 {
				t.Fatalf("live invitations after the refused repeat = %d, want exactly 1", live)
			}
		})
	}
}

// TestExpiredInvitationDoesNotBlockANewOne proves the expiry half of F2: because
// the partial unique index cannot test now(), an unconsumed invitation that is
// already expired must be cleared by the insert path and must never block a new
// invitation for the same target. Expiry therefore behaves as it did before.
func TestExpiredInvitationDoesNotBlockANewOne(t *testing.T) {
	repo, pool := setupTestDBWithPool(t)
	ctx := context.Background()

	email := "expiry-boundary@canchalibre.local"
	businessID := "019294f0-0000-7000-8000-000000000102"
	branchID := "019294f0-0000-7000-8000-000000000202"

	_, oldHash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new expired token: %v", err)
	}
	expired, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    oldHash,
		Email:        email,
		IntendedRole: actor.RoleEmployee,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create expired invitation: %v", err)
	}

	_, newHash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new live token: %v", err)
	}
	fresh, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    newHash,
		Email:        email,
		IntendedRole: actor.RoleEmployee,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
	})
	if err != nil {
		t.Fatalf("an expired but unconsumed invitation blocked a new insert: %v", err)
	}

	if live := countLiveInvitations(t, pool, email, actor.RoleEmployee, &businessID, &branchID); live != 1 {
		t.Fatalf("live invitations = %d, want exactly 1 (the new one)", live)
	}

	// The expired row is retired, never deleted: it survives so its token keeps
	// answering the documented expiration refusal instead of becoming an unknown
	// token.
	var retained int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM schema_identity.invitation WHERE id = $1::uuid
	`, expired.ID).Scan(&retained); err != nil {
		t.Fatalf("count the expired row: %v", err)
	}
	if retained != 1 {
		t.Fatalf("expired invitation row count = %d, want 1 (retained, never deleted)", retained)
	}
	var retiredAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT retired_at FROM schema_identity.invitation WHERE id = $1::uuid
	`, expired.ID).Scan(&retiredAt); err != nil {
		t.Fatalf("read retired_at: %v", err)
	}
	if retiredAt == nil {
		t.Fatalf("the expired invitation was not retired by the later insert")
	}

	// The winning invitation is still usable: the insert path did not disturb it.
	found, err := repo.FindInvitationByHash(ctx, newHash)
	if err != nil {
		t.Fatalf("find the new invitation by hash: %v", err)
	}
	if found.ID != fresh.ID {
		t.Fatalf("found invitation id = %s, want %s", found.ID, fresh.ID)
	}
}

// accountRowFingerprint returns the full stored row as deterministic JSON, so a
// test can prove that a repeated command left every column byte-identical.
func accountRowFingerprint(t *testing.T, pool *pgxpool.Pool, accountID string) string {
	t.Helper()
	var row string
	if err := pool.QueryRow(context.Background(), `
		SELECT to_jsonb(a)::text
		FROM schema_identity.account a
		WHERE a.id = $1::uuid
	`, accountID).Scan(&row); err != nil {
		t.Fatalf("read account row fingerprint: %v", err)
	}
	return row
}

// TestRevokeSupportOperatorIsStrictlyIdempotent proves finding F5: the first
// DELETE /admin/support-operators/{accountId} closes an ACTIVE SOPORTE operator
// and revokes its refresh sessions, and the second leaves the row byte-identical
// instead of rewriting updated_at. A non-SOPORTE account and an unknown account
// still answer the documented not-found.
func TestRevokeSupportOperatorIsStrictlyIdempotent(t *testing.T) {
	repo, pool := setupTestDBWithPool(t)
	ctx := context.Background()

	password, err := security.HashPassword("soporte-password-1234")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	soporteRole := actor.RoleSoporte
	operator, err := repo.CreateAccountWithCredential(ctx, domain.Account{
		Email:        "revoke-idempotent@canchalibre.local",
		Kind:         actor.KindWork,
		Status:       domain.StatusActive,
		PlatformRole: &soporteRole,
	}, password)
	if err != nil {
		t.Fatalf("create SOPORTE account: %v", err)
	}

	_, sessionHash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	if _, err := repo.CreateRefreshSession(ctx, operator.ID, sessionHash, time.Now().UTC().Add(security.RefreshTokenTTL)); err != nil {
		t.Fatalf("CreateRefreshSession: %v", err)
	}

	if err := repo.RevokeSupportOperator(ctx, operator.ID); err != nil {
		t.Fatalf("first revoke: %v", err)
	}

	firstFingerprint := accountRowFingerprint(t, pool, operator.ID)
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM schema_identity.account WHERE id = $1::uuid`, operator.ID).Scan(&status); err != nil {
		t.Fatalf("read status after first revoke: %v", err)
	}
	if status != domain.StatusClosed {
		t.Fatalf("status after first revoke = %q, want %q", status, domain.StatusClosed)
	}
	var revoked int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM schema_identity.refresh_session
		WHERE account_id = $1::uuid AND revoked_at IS NOT NULL
	`, operator.ID).Scan(&revoked); err != nil {
		t.Fatalf("count revoked sessions: %v", err)
	}
	if revoked != 1 {
		t.Fatalf("revoked refresh sessions after first revoke = %d, want 1", revoked)
	}

	if err := repo.RevokeSupportOperator(ctx, operator.ID); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
	if secondFingerprint := accountRowFingerprint(t, pool, operator.ID); secondFingerprint != firstFingerprint {
		t.Fatalf("repeat revoke rewrote the row:\n before: %s\n after:  %s", firstFingerprint, secondFingerprint)
	}

	// A non-SOPORTE account and an unknown account both answer the documented not-found.
	adminRole := actor.RoleAdmin
	admin, err := repo.CreateAccountWithCredential(ctx, domain.Account{
		Email:        "revoke-admin@canchalibre.local",
		Kind:         actor.KindWork,
		Status:       domain.StatusActive,
		PlatformRole: &adminRole,
	}, password)
	if err != nil {
		t.Fatalf("create ADMIN account: %v", err)
	}
	if err := repo.RevokeSupportOperator(ctx, admin.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoking a non-SOPORTE account = %v, want domain.ErrNotFound", err)
	}
	if err := repo.RevokeSupportOperator(ctx, "019294f0-0000-7000-8000-00000000dead"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoking an unknown account = %v, want domain.ErrNotFound", err)
	}
}

// TestSupersededInvitationIsRetainedAndRefusedAsExpired proves the F2 retirement
// model: superseding leaves the row in place with retired_at set, so its token
// keeps answering the documented expired refusal (409
// urn:canchalibre:error:invitation-expired) instead of becoming an unknown token.
func TestRevokedInvitationIsRetiredNotBackdated(t *testing.T) {
	repo, pool := setupTestDBWithPool(t)
	ctx := context.Background()
	svc := newIdentityService(repo)

	const email = "revoked-retired@canchalibre.local"
	businessID := "019294f0-0000-7000-8000-000000000101"
	branchID := "019294f0-0000-7000-8000-000000000201"

	raw, hash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	inv, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    hash,
		Email:        email,
		IntendedRole: actor.RoleEmployee,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	if err := repo.DeleteUnconsumedInvitation(ctx, inv.ID, businessID); err != nil {
		t.Fatalf("revoke invitation: %v", err)
	}

	// Revoking is a lifecycle fact, not a backdated expiry: the row survives,
	// retired_at carries the fact and expires_at is left alone.
	var retiredAt *time.Time
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT retired_at, expires_at FROM schema_identity.invitation WHERE id = $1::uuid
	`, inv.ID).Scan(&retiredAt, &expiresAt); err != nil {
		t.Fatalf("read the revoked row: %v", err)
	}
	if retiredAt == nil {
		t.Fatalf("retired_at is NULL after revoking, want a retirement timestamp")
	}
	if !expiresAt.Equal(inv.ExpiresAt) {
		t.Fatalf("revoking rewrote expires_at (%s -> %s); the lifecycle fact of this table is retired_at", inv.ExpiresAt, expiresAt)
	}

	// The client-visible refusal is unchanged: the revoked token answers
	// invitation-expired, never an unknown token.
	if _, err := svc.ActivateInvitation(ctx, raw, "retired-password-1234"); !errors.Is(err, domain.ErrInvitationExpired) {
		t.Fatalf("activating a revoked invitation = %v, want domain.ErrInvitationExpired", err)
	}

	// Revoking twice is a no-op: the row is byte-identical.
	var before, after string
	if err := pool.QueryRow(ctx, `
		SELECT to_jsonb(i)::text FROM schema_identity.invitation i WHERE id = $1::uuid
	`, inv.ID).Scan(&before); err != nil {
		t.Fatalf("fingerprint before the repeat: %v", err)
	}
	if err := repo.DeleteUnconsumedInvitation(ctx, inv.ID, businessID); err != nil {
		t.Fatalf("repeat revoke: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT to_jsonb(i)::text FROM schema_identity.invitation i WHERE id = $1::uuid
	`, inv.ID).Scan(&after); err != nil {
		t.Fatalf("fingerprint after the repeat: %v", err)
	}
	if before != after {
		t.Fatalf("repeat revoke rewrote the row:\n before: %s\n after:  %s", before, after)
	}

	// A retired row no longer blocks a new invitation for the same target.
	_, newHash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new live token: %v", err)
	}
	if _, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    newHash,
		Email:        email,
		IntendedRole: actor.RoleEmployee,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
	}); err != nil {
		t.Fatalf("a revoked invitation blocked a new one: %v", err)
	}
	if live := countLiveInvitations(t, pool, email, actor.RoleEmployee, &businessID, &branchID); live != 1 {
		t.Fatalf("live invitations = %d, want exactly 1 after revoking and reissuing", live)
	}
}

func TestSupersededInvitationIsRetainedAndRefusedAsExpired(t *testing.T) {
	repo, pool := setupTestDBWithPool(t)
	ctx := context.Background()
	svc := newIdentityService(repo)

	const email = "superseded-retained@canchalibre.local"
	businessID := "019294f0-0000-7000-8000-000000000103"
	branchID := "019294f0-0000-7000-8000-000000000203"

	raw, hash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	inv, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    hash,
		Email:        email,
		IntendedRole: actor.RoleEmployee,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	if err := repo.SupersedeInvitations(ctx, email, actor.RoleEmployee); err != nil {
		t.Fatalf("supersede invitations: %v", err)
	}

	if _, err := svc.ActivateInvitation(ctx, raw, "retired-password-1234"); !errors.Is(err, domain.ErrInvitationExpired) {
		t.Fatalf("activating a superseded invitation = %v, want domain.ErrInvitationExpired", err)
	}

	var retained int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM schema_identity.invitation WHERE id = $1::uuid
	`, inv.ID).Scan(&retained); err != nil {
		t.Fatalf("count the superseded row: %v", err)
	}
	if retained != 1 {
		t.Fatalf("superseded row count = %d, want 1 (retained, never deleted)", retained)
	}
	var retiredAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT retired_at FROM schema_identity.invitation WHERE id = $1::uuid
	`, inv.ID).Scan(&retiredAt); err != nil {
		t.Fatalf("read retired_at: %v", err)
	}
	if retiredAt == nil {
		t.Fatalf("retired_at is NULL after supersede, want a retirement timestamp")
	}

	found, err := repo.FindInvitationByHash(ctx, hash)
	if err != nil {
		t.Fatalf("a superseded invitation must remain findable: %v", err)
	}
	if found.ID != inv.ID {
		t.Fatalf("found invitation id = %s, want %s", found.ID, inv.ID)
	}

	var consumedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT consumed_at FROM schema_identity.invitation WHERE id = $1::uuid
	`, inv.ID).Scan(&consumedAt); err != nil {
		t.Fatalf("read consumed_at: %v", err)
	}
	if consumedAt != nil {
		t.Fatalf("the refused activation consumed the superseded invitation")
	}
}

// TestExpiredInvitationRetiredByLaterInsertStillAnswersInvitationExpired proves
// the regression the retirement model removes: an expired invitation touched by
// a later insert for the same target survives with retired_at set, so its token
// still answers domain.ErrInvitationExpired (409 invitation-expired) instead of
// disappearing into the unknown-token validation path (400).
func TestExpiredInvitationRetiredByLaterInsertStillAnswersInvitationExpired(t *testing.T) {
	repo, pool := setupTestDBWithPool(t)
	ctx := context.Background()
	svc := newIdentityService(repo)

	const email = "expired-then-retired@canchalibre.local"
	role := actor.RoleEmployee
	businessID := "019294f0-0000-7000-8000-000000000104"
	branchID := "019294f0-0000-7000-8000-000000000204"

	rawOld, hashOld, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new expired token: %v", err)
	}
	expired, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    hashOld,
		Email:        email,
		IntendedRole: role,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create expired invitation: %v", err)
	}

	_, newHash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new live token: %v", err)
	}
	if _, err := repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    newHash,
		Email:        email,
		IntendedRole: role,
		BusinessID:   &businessID,
		BranchID:     &branchID,
		ExpiresAt:    time.Now().UTC().Add(security.DefaultInvitationTTL),
	}); err != nil {
		t.Fatalf("a later insert must simply retire the expired row: %v", err)
	}

	if _, err := svc.ActivateInvitation(ctx, rawOld, "expired-password-1234"); !errors.Is(err, domain.ErrInvitationExpired) {
		t.Fatalf("activating the retired expired invitation = %v, want domain.ErrInvitationExpired", err)
	}

	var retained int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM schema_identity.invitation WHERE id = $1::uuid
	`, expired.ID).Scan(&retained); err != nil {
		t.Fatalf("count the expired row: %v", err)
	}
	if retained != 1 {
		t.Fatalf("expired row count = %d, want 1 (retained, never deleted)", retained)
	}
	var retiredAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT retired_at FROM schema_identity.invitation WHERE id = $1::uuid
	`, expired.ID).Scan(&retiredAt); err != nil {
		t.Fatalf("read retired_at: %v", err)
	}
	if retiredAt == nil {
		t.Fatalf("the expired invitation was not retired by the later insert")
	}

	if live := countLiveInvitations(t, pool, email, role, &businessID, &branchID); live != 1 {
		t.Fatalf("live invitations = %d, want exactly 1", live)
	}
}
