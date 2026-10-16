package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	platformpg "canchalibre/core/internal/platform/postgres"
	"canchalibre/core/internal/platform/security"
)

// testDatabaseURL points at a database dedicated to this package so the
// default canchalibre_dev database used by cmd/api is never touched. The
// database is recreated from scratch by postgres.ResetAndConnect.
const testDatabaseURL = "postgres://postgres:postgres@localhost:5435/canchalibre_provision_admin_test?sslmode=disable"

const (
	testJWTSecret      = "provision-admin-test-jwt-secret-at-least-32-bytes"
	testInvitationBase = "https://canchalibre.test/auth/activate"
)

type commandResult struct {
	code   int
	stdout string
	stderr string
}

func runCommand(args ...string) commandResult {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// setRequiredEnv installs the mandatory environment of the command. t.Setenv
// restores the previous values when the test ends, so no override leaks into
// another package's tests.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", testJWTSecret)
	t.Setenv("INVITATION_URL_BASE", testInvitationBase)
}

// setupTestDatabase recreates the isolated test database with the identity and
// organization schemas plus the committed seed, and points DATABASE_URL at it.
func setupTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accountsFile := filepath.Join(t.TempDir(), "accounts.json")
	accountsJSON := `[
  {"role":"ADMIN","email":"admin@canchalibre.local","password":"dev-only-change-me-admin"},
  {"role":"OWNER","email":"owner@canchalibre.local","password":"dev-only-change-me-owner"},
  {"role":"EMPLOYEE","email":"employee@canchalibre.local","password":"dev-only-change-me-employee"},
  {"role":"PLAYER","email":"player@canchalibre.local","password":"dev-only-change-me-player"},
  {"role":"SOPORTE","email":"soporte@canchalibre.local","password":"dev-only-change-me-soporte"}
]`
	if err := os.WriteFile(accountsFile, []byte(accountsJSON), 0o600); err != nil {
		t.Fatalf("write accounts file: %v", err)
	}

	pool, err := platformpg.ResetAndConnect(ctx, platformpg.ResetOptions{
		DatabaseURL:      testDatabaseURL,
		SeedTopologyPath: "dev/seed.topology.json",
		AccountsFilePath: accountsFile,
	})
	if err != nil {
		t.Fatalf("ResetAndConnect(%s): %v", testDatabaseURL, err)
	}
	t.Cleanup(pool.Close)
	t.Setenv("DATABASE_URL", testDatabaseURL)
	return pool
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

type rowSnapshot struct {
	accounts      int
	invitations   int
	audits        int
	successAudits int
}

func snapshot(t *testing.T, pool *pgxpool.Pool) rowSnapshot {
	t.Helper()
	return rowSnapshot{
		accounts:      countRows(t, pool, `SELECT count(*) FROM schema_identity.account`),
		invitations:   countRows(t, pool, `SELECT count(*) FROM schema_identity.invitation`),
		audits:        countRows(t, pool, `SELECT count(*) FROM schema_identity.identity_audit`),
		successAudits: countRows(t, pool, `SELECT count(*) FROM schema_identity.identity_audit WHERE outcome = 'SUCCESS'`),
	}
}

type invitationRow struct {
	ID           string
	IntendedRole string
	BusinessID   *string
	BranchID     *string
	IssuedBy     *string
	ConsumedAt   *time.Time
	ExpiresAt    time.Time
	TokenHash    []byte
}

func adminInvitations(t *testing.T, pool *pgxpool.Pool, email string) []invitationRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT id::text, intended_role, business_id::text, branch_id::text, issued_by::text,
		       consumed_at, expires_at, token_hash
		FROM schema_identity.invitation
		WHERE lower(email) = lower($1)
		ORDER BY created_at ASC
	`, email)
	if err != nil {
		t.Fatalf("query invitations: %v", err)
	}
	defer rows.Close()

	var out []invitationRow
	for rows.Next() {
		var r invitationRow
		if err := rows.Scan(&r.ID, &r.IntendedRole, &r.BusinessID, &r.BranchID, &r.IssuedBy,
			&r.ConsumedAt, &r.ExpiresAt, &r.TokenHash); err != nil {
			t.Fatalf("scan invitation: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate invitations: %v", err)
	}
	return out
}

func activationURL(t *testing.T, res commandResult) string {
	t.Helper()
	if res.stdout == "" {
		t.Fatalf("stdout is empty, want the activation URL")
	}
	if strings.Count(res.stdout, "?token=") != 1 {
		t.Fatalf("stdout = %q, want the activation URL printed exactly once", res.stdout)
	}
	url := strings.TrimSuffix(res.stdout, "\n")
	if strings.Contains(url, "\n") {
		t.Fatalf("stdout = %q, want a single line", res.stdout)
	}
	return url
}

// TestMissingEmailIsUsageError proves S7 (exit code 2 for a usage error): a
// missing --email fails argument validation with nothing on stdout and no row
// written to the database.
func TestMissingEmailIsUsageError(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	before := snapshot(t, pool)

	res := runCommand()

	if res.code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty", res.stdout)
	}
	if strings.TrimSpace(res.stderr) == "" {
		t.Fatalf("stderr is empty, want a plain usage reason")
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("database mutated by a usage error: before=%+v after=%+v", before, after)
	}
}

// TestMalformedEmailIsRefused proves S7: §4 of
// docs/design-identity-endpoints.md lists a malformed email under *Refusals*
// (exit code 1), not under usage errors. Nothing is printed to stdout, a plain
// reason goes to stderr, and no row is written.
func TestMalformedEmailIsRefused(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	before := snapshot(t, pool)

	res := runCommand("--email", "not-an-email")

	if res.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty", res.stdout)
	}
	if !strings.Contains(res.stderr, "email") {
		t.Fatalf("stderr = %q, want a plain reason mentioning the email", res.stderr)
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("database mutated by a malformed email: before=%+v after=%+v", before, after)
	}
}

// TestMissingDatabaseURLIsUsageError proves S4/S7: DATABASE_URL is mandatory
// with no silent default, and its absence is a usage failure on exit code 2.
func TestMissingDatabaseURLIsUsageError(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DATABASE_URL", "")

	res := runCommand("--email", "first-admin@example.com")

	if res.code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty", res.stdout)
	}
	if !strings.Contains(res.stderr, "DATABASE_URL") {
		t.Fatalf("stderr = %q, want a plain reason naming DATABASE_URL", res.stderr)
	}
}

// TestMissingJWTSecretIsUsageError proves S4/S7: the invitation signing secret
// is mandatory, its absence is a usage failure, and no database row is written
// because the failure happens before any connection is opened.
func TestMissingJWTSecretIsUsageError(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	t.Setenv("JWT_SECRET", "")
	before := snapshot(t, pool)

	res := runCommand("--email", "first-admin@example.com")

	if res.code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty", res.stdout)
	}
	if !strings.Contains(res.stderr, "JWT_SECRET") {
		t.Fatalf("stderr = %q, want a plain reason naming JWT_SECRET", res.stderr)
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("database mutated despite a usage error: before=%+v after=%+v", before, after)
	}
}

// TestValidEmailProvisionsPendingAdminInvitationAndAudit proves S7: a valid
// --email on a fresh database exits 0, prints the activation URL exactly once,
// creates a PENDING WORK account if absent, one single-use ADMIN invitation
// matching the design-identity-ddl.md row shape, and one audit row whose actor
// is the OS command invocation.
func TestValidEmailProvisionsPendingAdminInvitationAndAudit(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	ctx := context.Background()

	const email = "first-admin@example.com"
	before := snapshot(t, pool)

	res := runCommand("--email", email)
	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.code, res.stderr)
	}
	url := activationURL(t, res)
	rawToken, ok := strings.CutPrefix(url, testInvitationBase+"?token=")
	if !ok {
		t.Fatalf("activation URL = %q, want the INVITATION_URL_BASE prefix %q", url, testInvitationBase)
	}
	if _, err := base64.RawURLEncoding.DecodeString(rawToken); err != nil {
		t.Fatalf("token %q is not base64url: %v", rawToken, err)
	}

	// The account is a PENDING WORK account with the ADMIN platform role.
	var kind, status string
	var platformRole *string
	if err := pool.QueryRow(ctx, `
		SELECT kind, status, platform_role
		FROM schema_identity.account
		WHERE lower(email) = lower($1) AND kind = 'WORK'
	`, email).Scan(&kind, &status, &platformRole); err != nil {
		t.Fatalf("query provisioned account: %v", err)
	}
	if kind != "WORK" || status != "PENDING" {
		t.Fatalf("account kind/status = %s/%s, want WORK/PENDING", kind, status)
	}
	if platformRole == nil || *platformRole != "ADMIN" {
		t.Fatalf("platform_role = %v, want ADMIN", platformRole)
	}

	// Exactly one single-use ADMIN invitation with the frozen scope shape, and
	// the stored digest is the SHA-256 of the printed token: never the raw token.
	rows := adminInvitations(t, pool, email)
	if len(rows) != 1 {
		t.Fatalf("invitation count = %d, want exactly 1", len(rows))
	}
	inv := rows[0]
	if inv.IntendedRole != "ADMIN" {
		t.Fatalf("intended_role = %q, want ADMIN", inv.IntendedRole)
	}
	if inv.BusinessID != nil || inv.BranchID != nil {
		t.Fatalf("invitation scope = (%v, %v), want (nil, nil) for ADMIN", inv.BusinessID, inv.BranchID)
	}
	if inv.IssuedBy != nil {
		t.Fatalf("issued_by = %v, want nil for an OS-command invocation", *inv.IssuedBy)
	}
	if inv.ConsumedAt != nil {
		t.Fatalf("consumed_at = %v, want nil for an unconsumed invitation", inv.ConsumedAt)
	}
	if !inv.ExpiresAt.After(time.Now()) {
		t.Fatalf("expires_at = %v, want a future expiry", inv.ExpiresAt)
	}
	if len(inv.TokenHash) != 32 {
		t.Fatalf("token_hash length = %d, want a 32-byte SHA-256 digest", len(inv.TokenHash))
	}
	if !bytes.Equal(inv.TokenHash, security.HashOpaqueToken(rawToken)) {
		t.Fatalf("token_hash is not the SHA-256 digest of the printed token")
	}

	// Exactly one audit row: the OS-command invocation as actor, with the binary
	// and the operator's --email recorded in the detail.
	var event, outcome, detailBinary, detailEmail string
	var actorAccountID *string
	if err := pool.QueryRow(ctx, `
		SELECT event, outcome, actor_account_id::text, detail->>'binary', detail->>'email'
		FROM schema_identity.identity_audit
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&event, &outcome, &actorAccountID, &detailBinary, &detailEmail); err != nil {
		t.Fatalf("query audit row: %v", err)
	}
	if event != "provision_admin" || outcome != "SUCCESS" {
		t.Fatalf("audit event/outcome = %s/%s, want provision_admin/SUCCESS", event, outcome)
	}
	if actorAccountID != nil {
		t.Fatalf("actor_account_id = %v, want NULL for the OS-command invocation", *actorAccountID)
	}
	if detailBinary != "provision-admin" || !strings.EqualFold(detailEmail, email) {
		t.Fatalf("audit detail = binary %q email %q, want provision-admin and %q", detailBinary, detailEmail, email)
	}

	after := snapshot(t, pool)
	if after.accounts != before.accounts+1 {
		t.Fatalf("account count = %d, want %d", after.accounts, before.accounts+1)
	}
	if after.invitations != before.invitations+1 {
		t.Fatalf("invitation count = %d, want %d", after.invitations, before.invitations+1)
	}
	if after.audits != before.audits+1 || after.successAudits != before.successAudits+1 {
		t.Fatalf("audit counts = %+v, want exactly one new SUCCESS audit row", after)
	}
}

// TestSecondRunWithoutSupersedeRefusesWithoutDomainMutation proves S7: a second
// run for the same email while the invitation is unexpired exits 1, prints a
// plain reason on stderr, and mutates no domain row (no new invitation, no new
// account, no new SUCCESS audit row).
func TestSecondRunWithoutSupersedeRefusesWithoutDomainMutation(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)

	const email = "second-run@example.com"
	if first := runCommand("--email", email); first.code != 0 {
		t.Fatalf("first run exit code = %d, want 0; stderr:\n%s", first.code, first.stderr)
	}
	before := snapshot(t, pool)

	res := runCommand("--email", email)

	if res.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty for a refusal", res.stdout)
	}
	if !strings.Contains(res.stderr, "unexpired") && !strings.Contains(res.stderr, "supersede") {
		t.Fatalf("stderr = %q, want a plain reason about the unexpired invitation", res.stderr)
	}

	after := snapshot(t, pool)
	if after.invitations != before.invitations {
		t.Fatalf("invitation count = %d, want %d (no new invitation)", after.invitations, before.invitations)
	}
	if after.accounts != before.accounts {
		t.Fatalf("account count = %d, want %d (no new account)", after.accounts, before.accounts)
	}
	if after.successAudits != before.successAudits {
		t.Fatalf("success audit count = %d, want %d (no new provisioning audit row)", after.successAudits, before.successAudits)
	}
	// docs/design-identity-ddl.md §Refusals: a refusal records only the
	// append-only REFUSED audit row and never domain state.
	refusedAdded := countRows(t, pool, `
		SELECT count(*) FROM schema_identity.identity_audit
		WHERE outcome = 'REFUSED' AND actor_account_id IS NULL AND event = 'provision_admin'
	`)
	if refusedAdded < 1 {
		t.Fatalf("refused audit rows = %d, want at least one matching provision_admin refusal", refusedAdded)
	}
	if adminInvitations(t, pool, email)[0].ConsumedAt != nil {
		t.Fatalf("the original invitation was consumed by the refused run")
	}
}

// TestSecondRunWithSupersedeReplacesUnexpiredInvitation proves S7: --supersede
// exits 0 and replaces the previous unexpired ADMIN invitation by expiring it
// (expires_at moved into the past, consumed_at still NULL, the row retained)
// while reusing the existing PENDING account.
func TestSecondRunWithSupersedeReplacesUnexpiredInvitation(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	ctx := context.Background()

	const email = "supersede@example.com"
	first := runCommand("--email", email)
	if first.code != 0 {
		t.Fatalf("first run exit code = %d, want 0; stderr:\n%s", first.code, first.stderr)
	}
	beforeAccounts := countRows(t, pool, `SELECT count(*) FROM schema_identity.account WHERE lower(email) = lower($1)`, email)

	second := runCommand("--email", email, "--supersede")
	if second.code != 0 {
		t.Fatalf("second run exit code = %d, want 0; stderr:\n%s", second.code, second.stderr)
	}
	if second.stdout == first.stdout {
		t.Fatalf("second run reused the first activation URL: %q", second.stdout)
	}
	activationURL(t, second)

	rows := adminInvitations(t, pool, email)
	if len(rows) != 2 {
		t.Fatalf("invitation count = %d, want 2 (the superseded row is retained)", len(rows))
	}
	old, current := rows[0], rows[1]
	if !old.ExpiresAt.Before(time.Now()) {
		t.Fatalf("superseded invitation expires_at = %v, want it moved into the past", old.ExpiresAt)
	}
	if old.ConsumedAt != nil {
		t.Fatalf("superseded invitation consumed_at = %v, want NULL (superseded, not consumed)", old.ConsumedAt)
	}
	if !current.ExpiresAt.After(time.Now()) {
		t.Fatalf("new invitation expires_at = %v, want a future expiry", current.ExpiresAt)
	}
	if bytes.Equal(old.TokenHash, current.TokenHash) {
		t.Fatalf("the new invitation reused the superseded token hash")
	}
	if after := countRows(t, pool, `SELECT count(*) FROM schema_identity.account WHERE lower(email) = lower($1)`, email); after != beforeAccounts {
		t.Fatalf("account count = %d, want %d (the PENDING account is reused)", after, beforeAccounts)
	}

	var superseded string
	if err := pool.QueryRow(ctx, `
		SELECT detail->>'superseded'
		FROM schema_identity.identity_audit
		WHERE event = 'provision_admin' AND outcome = 'SUCCESS'
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&superseded); err != nil {
		t.Fatalf("query supersede audit row: %v", err)
	}
	if superseded != "true" {
		t.Fatalf("audit detail superseded = %q, want \"true\"", superseded)
	}
}

// TestDryRunMutatesNothing proves S7: --dry-run exits 0, reports, prints no
// activation URL, and leaves account, invitation and identity_audit counts
// unchanged.
func TestDryRunMutatesNothing(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)

	before := snapshot(t, pool)
	res := runCommand("--email", "dry-run@example.com", "--dry-run")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "dry-run") {
		t.Fatalf("stdout = %q, want a dry-run report", res.stdout)
	}
	if strings.Contains(res.stdout, "?token=") {
		t.Fatalf("stdout = %q, want no activation URL in dry-run", res.stdout)
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("dry-run mutated the database: before=%+v after=%+v", before, after)
	}
}

// TestAlreadyActiveAdminIsRefusedWithoutMutation proves S7: provisioning an
// email that is already an active ADMIN exits 1 and mutates no domain row.
func TestAlreadyActiveAdminIsRefusedWithoutMutation(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)

	before := snapshot(t, pool)
	res := runCommand("--email", "admin@canchalibre.local")

	if res.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty for a refusal", res.stdout)
	}
	if !strings.Contains(res.stderr, "active ADMIN") {
		t.Fatalf("stderr = %q, want a plain reason about the active ADMIN", res.stderr)
	}
	after := snapshot(t, pool)
	if after.accounts != before.accounts || after.invitations != before.invitations {
		t.Fatalf("refusal mutated domain state: before=%+v after=%+v", before, after)
	}
	if after.successAudits != before.successAudits {
		t.Fatalf("success audit count = %d, want %d", after.successAudits, before.successAudits)
	}
}

// TestAlreadyActiveNonAdminWorkAccountIsRefusedWithoutMutation proves the
// boundary of the same refusal rule for an active WORK account that is not an
// ADMIN.
func TestAlreadyActiveNonAdminWorkAccountIsRefusedWithoutMutation(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)

	before := snapshot(t, pool)
	res := runCommand("--email", "owner@canchalibre.local")

	if res.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", res.code, res.stderr)
	}
	if res.stdout != "" {
		t.Fatalf("stdout = %q, want empty for a refusal", res.stdout)
	}
	after := snapshot(t, pool)
	if after.accounts != before.accounts || after.invitations != before.invitations || after.successAudits != before.successAudits {
		t.Fatalf("refusal mutated state: before=%+v after=%+v", before, after)
	}
}

// TestActivationURLUsesInvitationURLBaseAndIsNeverLogged proves S7: the URL is
// built from INVITATION_URL_BASE and the raw token never reaches the structured
// logger on stderr even though the logger is active.
func TestActivationURLUsesInvitationURLBaseAndIsNeverLogged(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	const base = "https://canchalibre.example/dev/activate"
	t.Setenv("INVITATION_URL_BASE", base)

	res := runCommand("--email", "url-base@example.com")
	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.code, res.stderr)
	}
	url := activationURL(t, res)
	if !strings.HasPrefix(url, base+"?token=") {
		t.Fatalf("activation URL = %q, want the %q prefix", url, base)
	}
	rawToken := strings.TrimPrefix(url, base+"?token=")
	if len(rawToken) != 43 {
		t.Fatalf("token length = %d, want 43 base64url characters", len(rawToken))
	}
	if strings.Contains(res.stderr, rawToken) || strings.Contains(res.stderr, url) {
		t.Fatalf("the activation URL or token leaked into the logs:\n%s", res.stderr)
	}
	if !strings.Contains(res.stderr, "invitation_id") {
		t.Fatalf("stderr = %q, want an active structured logger line without secrets", res.stderr)
	}
	if countRows(t, pool, `SELECT count(*) FROM schema_identity.invitation WHERE lower(email) = 'url-base@example.com'`) != 1 {
		t.Fatalf("want exactly one invitation for the email")
	}
}

// TestTTLFlagAndDefaultSetInvitationExpiry proves S7: --ttl controls the
// invitation lifetime and defaults to 24h.
func TestTTLFlagAndDefaultSetInvitationExpiry(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)

	start := time.Now()
	if res := runCommand("--email", "ttl-two-hours@example.com", "--ttl", "2h"); res.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.code, res.stderr)
	}
	twoHours := adminInvitations(t, pool, "ttl-two-hours@example.com")[0].ExpiresAt
	assertExpiryWithin(t, twoHours, start.Add(2*time.Hour), 2*time.Minute)

	if res := runCommand("--email", "ttl-default@example.com"); res.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.code, res.stderr)
	}
	defaultTTL := adminInvitations(t, pool, "ttl-default@example.com")[0].ExpiresAt
	assertExpiryWithin(t, defaultTTL, start.Add(24*time.Hour), 2*time.Minute)
}

func assertExpiryWithin(t *testing.T, got, want time.Time, tolerance time.Duration) {
	t.Helper()
	if got.Before(want.Add(-tolerance)) || got.After(want.Add(tolerance)) {
		t.Fatalf("expires_at = %v, want within %v of %v", got, tolerance, want)
	}
}

// TestMalformedTTLIsUsageError proves S7: an unparseable or non-positive --ttl
// is a usage error on exit code 2 with nothing written.
func TestMalformedTTLIsUsageError(t *testing.T) {
	pool := setupTestDatabase(t)
	setRequiredEnv(t)
	before := snapshot(t, pool)

	for _, ttl := range []string{"not-a-duration", "-1h", "0s"} {
		res := runCommand("--email", "ttl-bad@example.com", "--ttl", ttl)
		if res.code != 2 {
			t.Fatalf("--ttl %s: exit code = %d, want 2; stderr:\n%s", ttl, res.code, res.stderr)
		}
		if res.stdout != "" {
			t.Fatalf("--ttl %s: stdout = %q, want empty", ttl, res.stdout)
		}
		if strings.TrimSpace(res.stderr) == "" {
			t.Fatalf("--ttl %s: stderr is empty, want a plain usage reason", ttl)
		}
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("database mutated by a usage error: before=%+v after=%+v", before, after)
	}
}
