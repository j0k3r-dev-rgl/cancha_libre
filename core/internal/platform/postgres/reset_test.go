package postgres_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	orgpostgres "canchalibre/core/internal/modules/organization/adapters/outbound/postgres"
	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/platform/postgres"
)

func TestResetAndSeedRecreatesDatabaseAndLoadsTopology(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5435/canchalibre_test_platform?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create a temporary accounts file with valid >=12 char passwords
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
		t.Fatalf("write temp accounts file: %v", err)
	}

	topologyPath := filepath.Join("..", "..", "..", "dev", "seed.topology.json")

	pool1, err := postgres.ResetAndConnect(ctx, postgres.ResetOptions{
		DatabaseURL:      dbURL,
		SeedTopologyPath: topologyPath,
		AccountsFilePath: accountsFile,
	})
	if err != nil {
		t.Fatalf("first ResetAndConnect failed: %v", err)
	}

	defer pool1.Close()
	assertApprovalFixtures(t, ctx, pool1)

	var oid1 uint32
	if err := pool1.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname = current_database()").Scan(&oid1); err != nil {
		pool1.Close()
		t.Fatalf("read first oid: %v", err)
	}

	var accountCount, businessCount, branchCount, membershipCount int
	if err := pool1.QueryRow(ctx, "SELECT count(*) FROM schema_identity.account").Scan(&accountCount); err != nil {
		pool1.Close()
		t.Fatalf("count accounts: %v", err)
	}
	if err := pool1.QueryRow(ctx, "SELECT count(*) FROM schema_organization.business").Scan(&businessCount); err != nil {
		pool1.Close()
		t.Fatalf("count businesses: %v", err)
	}
	if err := pool1.QueryRow(ctx, "SELECT count(*) FROM schema_organization.branch").Scan(&branchCount); err != nil {
		pool1.Close()
		t.Fatalf("count branches: %v", err)
	}
	if err := pool1.QueryRow(ctx, "SELECT count(*) FROM schema_organization.membership").Scan(&membershipCount); err != nil {
		pool1.Close()
		t.Fatalf("count memberships: %v", err)
	}

	if accountCount != 5 || businessCount != 2 || branchCount != 8 || membershipCount != 2 {
		pool1.Close()
		t.Fatalf("unexpected seeded counts: accounts=%d businesses=%d branches=%d memberships=%d",
			accountCount, businessCount, branchCount, membershipCount)
	}

	// S2 schema tables check: verify new tables are created by the reset
	for _, table := range []string{
		"schema_organization.branch_approval_history",
		"schema_organization.branch_operating_hours",
		"schema_organization.branch_photo",
		"schema_organization.court",
	} {
		var exists bool
		if err := pool1.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil || !exists {
			pool1.Close()
			t.Fatalf("expected table %s to exist, err=%v exists=%v", table, err, exists)
		}
	}

	// Verify branch approval_status and coordinates_confirmed seeded values
	var approvedCount int
	if err := pool1.QueryRow(ctx, "SELECT count(*) FROM schema_organization.branch WHERE approval_status = 'APPROVED' AND coordinates_confirmed = true").Scan(&approvedCount); err != nil || approvedCount != 3 {
		pool1.Close()
		t.Fatalf("expected 3 approved branches, got %d, err=%v", approvedCount, err)
	}

	pool1.Close()

	// Second reset must produce a different database OID (proving DROP + CREATE)
	pool2, err := postgres.ResetAndConnect(ctx, postgres.ResetOptions{
		DatabaseURL:      dbURL,
		SeedTopologyPath: topologyPath,
		AccountsFilePath: accountsFile,
	})
	if err != nil {
		t.Fatalf("second ResetAndConnect failed: %v", err)
	}
	defer pool2.Close()
	assertApprovalFixtures(t, ctx, pool2)

	var oid2 uint32
	if err := pool2.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname = current_database()").Scan(&oid2); err != nil {
		t.Fatalf("read second oid: %v", err)
	}
	if oid1 == oid2 {
		t.Fatalf("expected database OID to change after DROP + CREATE, both were %d", oid1)
	}
}

func assertApprovalFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	repo := orgpostgres.NewRepository(pool)
	items, total, err := repo.ListBranchRequests(ctx, ports.BranchRequestFilter{Status: "PENDING", Limit: 100})
	if err != nil || total != 5 || len(items) != 5 {
		t.Fatalf("expected five pending approval fixtures: total=%d items=%d err=%v", total, len(items), err)
	}
	for i := 1; i <= 5; i++ {
		requestID := fmt.Sprintf("019294f0-0000-7000-8000-00000000040%d", i)
		detail, err := repo.GetBranchRequestDetail(ctx, requestID)
		if err != nil {
			t.Fatalf("read fixture %s through repository: %v", requestID, err)
		}
		branchID := fmt.Sprintf("019294f0-0000-7000-8000-00000000021%d", i)
		if detail.BranchID != branchID || string(detail.Status) != "PENDING" ||
			detail.SubmittedBy != "019294f0-0000-7000-8000-000000000002" ||
			detail.ReviewedBy != nil || detail.ReviewedAt != nil || detail.RejectionReason != nil {
			t.Fatalf("unexpected pending fixture: %+v", detail.BranchRequestItem)
		}
		if string(detail.Branch.ApprovalStatus) != "PENDING_APPROVAL" || detail.Branch.Enabled {
			t.Fatalf("pending fixture branch must not be operational: %+v", detail.Branch)
		}
		if i == 1 {
			if len(detail.History) != 2 {
				t.Fatalf("expected preserved prior rejection plus pending round: %+v", detail.History)
			}
			prior := detail.History[0]
			if prior.ID != "019294f0-0000-7000-8000-000000000400" || string(prior.Status) != "REJECTED" ||
				prior.ReviewedBy == nil || *prior.ReviewedBy != "019294f0-0000-7000-8000-000000000005" ||
				prior.ReviewedAt == nil || prior.RejectionReason == nil || *prior.RejectionReason != "Confirm street address" ||
				!prior.SubmittedAt.Before(*prior.ReviewedAt) || !prior.ReviewedAt.Before(detail.SubmittedAt) {
				t.Fatalf("invalid preserved review round: %+v", prior)
			}
		} else if len(detail.History) != 1 {
			t.Fatalf("expected single pending round for %s: %+v", requestID, detail.History)
		}
	}
	var unresolvedActors int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_organization.branch_approval_history h
		WHERE NOT EXISTS (SELECT 1 FROM schema_identity.account a WHERE a.id = h.submitted_by)
		OR (h.reviewed_by IS NOT NULL AND NOT EXISTS
			(SELECT 1 FROM schema_identity.account a WHERE a.id = h.reviewed_by))`).Scan(&unresolvedActors); err != nil || unresolvedActors != 0 {
		t.Fatalf("unresolved fixture actor references: count=%d err=%v", unresolvedActors, err)
	}
}
