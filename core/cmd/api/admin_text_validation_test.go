package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

// Snapshot every domain row, including timestamps, rather than only counting rows.
// Audit is checked separately because a service refusal may append one REFUSED row.
func snapshotAdminDomain(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT schemaname, tablename FROM pg_tables
 WHERE schemaname IN ('schema_identity', 'schema_organization')
 AND tablename <> 'identity_audit' ORDER BY schemaname, tablename`)
	if err != nil {
		t.Fatal(err)
	}
	tables := []pgx.Identifier{}
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		tables = append(tables, pgx.Identifier{schema, table})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var snapshot strings.Builder
	for _, table := range tables {
		var contents string
		query := `SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text), '[]'::jsonb)::text FROM ` + table.Sanitize() + ` r`
		if err := pool.QueryRow(ctx, query).Scan(&contents); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&snapshot, "%s=%s\n", table.Sanitize(), contents)
	}
	return snapshot.String()
}

func TestAdminTextNULValidation(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	now := time.Now().UTC()
	token, err := security.SignAccessToken([]byte(apiTestJWTSecret), security.AccessTokenClaims{
		Subject: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin,
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	const business = "/api/v1/admin/businesses/019294f0-0000-7000-8000-000000000101"
	const branch = "/api/v1/admin/branches/019294f0-0000-7000-8000-000000000201"
	const user = "/api/v1/admin/users/019294f0-0000-7000-8000-000000000003"
	const request = "/api/v1/admin/branch-requests/019294f0-0000-7000-8000-000000000402"
	tests := []struct{ name, method, path, body string }{
		{"create name", "POST", "/api/v1/admin/businesses", `{"name":"Test\u0000NullByte"}`},
		{"create legal name", "POST", "/api/v1/admin/businesses", `{"name":"Club","legal_name":"Bad\u0000Text"}`},
		{"create tax ID", "POST", "/api/v1/admin/businesses", `{"name":"Club","tax_id":"Bad\u0000Text"}`},
		{"update name", "PATCH", business, `{"name":"Bad\u0000Text"}`},
		{"update legal name", "PATCH", business, `{"legal_name":"Bad\u0000Text"}`},
		{"update tax ID", "PATCH", business, `{"tax_id":"Bad\u0000Text"}`},
		{"business suspension reason", "POST", business + "/suspend", `{"reason":"Bad\u0000Text"}`},
		{"branch suspension reason", "POST", branch + "/suspend", `{"reason":"Bad\u0000Text"}`},
		{"user suspension reason", "POST", user + "/suspend", `{"reason":"Bad\u0000Text"}`},
		{"rejection reason", "POST", request + "/reject", `{"rejection_reason":"Bad\u0000Text"}`},
		{"business search", "GET", "/api/v1/admin/businesses?search=" + url.QueryEscape("Bad\x00Text"), ""},
		{"branch city", "GET", "/api/v1/admin/branches?city=" + url.QueryEscape("Bad\x00Text"), ""},
		{"branch approval status", "GET", "/api/v1/admin/branches?approval_status=" + url.QueryEscape("Bad\x00Text"), ""},
		{"user search", "GET", "/api/v1/admin/users?search=" + url.QueryEscape("Bad\x00Text"), ""},
		{"user role", "GET", "/api/v1/admin/users?role=" + url.QueryEscape("Bad\x00Text"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := snapshotAdminDomain(t, pool)
			var auditBefore string
			if err := pool.QueryRow(context.Background(), `SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id), '[]'::jsonb)::text FROM schema_identity.identity_audit a`).Scan(&auditBefore); err != nil {
				t.Fatal(err)
			}
			refusedBefore := countRefusedAudits(t, pool)
			status, problem := do(t, router, tt.method, tt.path, tt.body, map[string]string{
				"Authorization": "Bearer " + token, "Content-Type": "application/json",
			})
			if status != http.StatusBadRequest || problem.Status != http.StatusBadRequest || problem.Type != "urn:canchalibre:error:validation" {
				t.Errorf("status=%d problem=%+v; want 400 validation", status, problem)
			}
			if after := snapshotAdminDomain(t, pool); after != before {
				t.Error("refused NUL input changed domain row values or counts")
			}
			if countRefusedAudits(t, pool) != refusedBefore+1 {
				t.Error("expected exactly one REFUSED audit row")
			}
			var retained string
			if err := pool.QueryRow(context.Background(), `SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id), '[]'::jsonb)::text FROM schema_identity.identity_audit a WHERE id < (SELECT max(id) FROM schema_identity.identity_audit)`).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if retained != auditBefore {
				t.Error("previous audit entries changed or extra audit rows were appended")
			}
		})
	}
}

// Previously accepted Unicode and ordinary whitespace must not be rejected by the NUL fix.
func TestAdminTextExistingBehavior(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	now := time.Now().UTC()
	token, err := security.SignAccessToken([]byte(apiTestJWTSecret), security.AccessTokenClaims{
		Subject: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin,
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	const business = "/api/v1/admin/businesses/019294f0-0000-7000-8000-000000000101"
	tests := []struct {
		name, method, path, body string
		status                   int
	}{
		{"create Unicode", "POST", "/api/v1/admin/businesses", `{"name":"  ⚽ Club Árabe العربية  ","legal_name":"Club\nLegal","tax_id":"AR-123"}`, 201},
		{"update Unicode", "PATCH", business, `{"name":"Club Actualizado ⚽","legal_name":"Legal\nName","tax_id":"AR-456"}`, 200},
		{"business search", "GET", "/api/v1/admin/businesses?search=Club", "", 200},
		{"branch filters", "GET", "/api/v1/admin/branches?city=Buenos+Aires&approval_status=APPROVED", "", 200},
		{"user filters", "GET", "/api/v1/admin/users?search=owner&role=OWNER", "", 200},
		{"business reason", "POST", business + "/suspend", `{"reason":"Review\nrequired ⚽"}`, 200},
		{"branch reason", "POST", "/api/v1/admin/branches/019294f0-0000-7000-8000-000000000201/suspend", `{"reason":"Review\nrequired ⚽"}`, 200},
		{"user reason", "POST", "/api/v1/admin/users/019294f0-0000-7000-8000-000000000003/suspend", `{"reason":"Review\nrequired ⚽"}`, 200},
		{"reject reason", "POST", "/api/v1/admin/branch-requests/019294f0-0000-7000-8000-000000000402/reject", `{"rejection_reason":"Confirm\naddress ⚽"}`, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tt.status, rec.Body.String())
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tt.name == "create Unicode" {
				var name string
				if err := json.Unmarshal(body["name"], &name); err != nil || name != "⚽ Club Árabe العربية" {
					t.Fatalf("name=%q err=%v", name, err)
				}
			}
		})
	}
}
