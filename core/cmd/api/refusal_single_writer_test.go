package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/platform/config"
	platformpg "canchalibre/core/internal/platform/postgres"
)

// apiTestDatabaseURL is this package's own target. Every Postgres-backed test
// package resets and seeds a database of its own, because `go test ./...` runs
// package binaries in parallel and two packages sharing one target would reset
// each other mid-run.
const apiTestDatabaseURL = "postgres://postgres:postgres@localhost:5435/canchalibre_test_api?sslmode=disable"

// setupAPITestDatabase resets and seeds a real database for the refusal tests and
// returns its pool. It is the only harness in this package that connects: the
// contract tests intentionally use a lazy pool that never reaches PostgreSQL.
func setupAPITestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("TEST_API_DATABASE_URL")
	if dbURL == "" {
		dbURL = apiTestDatabaseURL
	}

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
		DatabaseURL:      dbURL,
		SeedTopologyPath: "dev/seed.topology.json",
		AccountsFilePath: accountsFile,
	})
	if err != nil {
		t.Fatalf("ResetAndConnect(%s): %v", dbURL, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newDatabaseBackedRouter builds the real composition root over a connected pool,
// so a refusal is exercised through the same HTTP stack the process serves.
func newDatabaseBackedRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()

	router, _ := newRouter(pool, config.Config{
		JWTSecret:         "0123456789abcdef0123456789abcdef",
		InvitationURLBase: "http://localhost:3000/auth/activate",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return router
}

type problemResponse struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// do sends one request through the router and returns the status and the decoded
// problem detail. A body that is not a problem document fails the test, because
// every refusal in this API is RFC 7807.
func do(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) (int, problemResponse) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var prob problemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prob); err != nil {
		t.Fatalf("%s %s: response body is not a problem document: %v (%s)", method, path, err, rec.Body.String())
	}
	return rec.Code, prob
}

// countRefusedAudits counts the append-only REFUSED rows a refusal left behind.
func countRefusedAudits(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM schema_identity.identity_audit WHERE outcome = 'REFUSED'`).Scan(&n); err != nil {
		t.Fatalf("count REFUSED audit rows: %v", err)
	}
	return n
}

// TestOneServiceRefusalAppendsExactlyOneAuditRow proves finding F1 across the seam
// where the defect lived: the use case refuses a wrong-password login and appends
// one REFUSED row, and the HTTP layer must not append a second one of its own. A
// transport-level refusal is the recorded boundary — it appends none — because the
// identity use cases are the single writer. The rule is observable only as a row
// count, so this test needs a real database; the contract tests cannot see it.
func TestOneServiceRefusalAppendsExactlyOneAuditRow(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)

	// A use-case-level refusal: the account exists and is ACTIVE, the password is
	// simply wrong, so the credential check refuses after the handler ran.
	status, prob := do(t, router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"owner@canchalibre.local","password":"definitely-wrong-1234","accountKind":"WORK"}`,
		map[string]string{"Content-Type": "application/json"})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong-password login status = %d, want %d (%+v)", status, http.StatusUnauthorized, prob)
	}
	if prob.Type != "urn:canchalibre:error:unauthenticated" {
		t.Fatalf("wrong-password login type = %q, want urn:canchalibre:error:unauthenticated", prob.Type)
	}
	if got := countRefusedAudits(t, pool); got != 1 {
		t.Fatalf("REFUSED audit rows after one refused login = %d, want exactly 1: the use case is the single writer and the HTTP layer must not duplicate the row", got)
	}

	// A transport-level refusal: the guard refuses before any handler runs.
	status, prob = do(t, router, http.MethodGet, "/api/v1/me", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /me status = %d, want %d (%+v)", status, http.StatusUnauthorized, prob)
	}
	if prob.Type != "urn:canchalibre:error:unauthenticated" {
		t.Fatalf("unauthenticated /me type = %q, want urn:canchalibre:error:unauthenticated", prob.Type)
	}
	if got := countRefusedAudits(t, pool); got != 1 {
		t.Fatalf("REFUSED audit rows after a transport-level refusal = %d, want 1: the guard is not an audit writer", got)
	}

	// The refused login left no domain state behind: no session was created.
	var sessions int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM schema_identity.refresh_session`).Scan(&sessions); err != nil {
		t.Fatalf("count refresh sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("refresh sessions after a refused login = %d, want 0", sessions)
	}
}
