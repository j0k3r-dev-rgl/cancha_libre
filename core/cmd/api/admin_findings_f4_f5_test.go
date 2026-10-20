package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

func TestFindingsF4AndF5(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	ctx := context.Background()

	now := time.Now().UTC()
	token, err := security.SignAccessToken([]byte(apiTestJWTSecret), security.AccessTokenClaims{
		Subject:   "019294f0-0000-7000-8000-000000000001",
		Actor:     actor.RoleAdmin,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// ==================== F4: Users Role Filter ====================
	t.Run("F4_UsersRoleFilter", func(t *testing.T) {
		roleTests := []struct {
			role      string
			wantEmail string
			wantTotal int
		}{
			{role: "OWNER", wantEmail: "owner@canchalibre.local", wantTotal: 1},
			{role: "EMPLOYEE", wantEmail: "employee@canchalibre.local", wantTotal: 1},
			{role: "ADMIN", wantEmail: "admin@canchalibre.local", wantTotal: 1},
			{role: "SOPORTE", wantEmail: "soporte@canchalibre.local", wantTotal: 1},
			{role: "PLAYER", wantEmail: "player@canchalibre.local", wantTotal: 1},
		}

		for _, tt := range roleTests {
			t.Run("role_"+tt.role, func(t *testing.T) {
				rec := call(http.MethodGet, "/api/v1/admin/users?role="+tt.role, "")
				if rec.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
				}
				var resp struct {
					Items []struct {
						ID    string `json:"id"`
						Email string `json:"email"`
					} `json:"items"`
					Total int `json:"total"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				if resp.Total != tt.wantTotal || len(resp.Items) != tt.wantTotal {
					t.Fatalf("role=%s: want total=%d, got total=%d, items=%+v", tt.role, tt.wantTotal, resp.Total, resp.Items)
				}
				if resp.Items[0].Email != tt.wantEmail {
					t.Fatalf("role=%s: want email=%s, got email=%s", tt.role, tt.wantEmail, resp.Items[0].Email)
				}
			})
		}

		// Invalid role filter returns 400 validation
		t.Run("invalid_role_filter", func(t *testing.T) {
			rec := call(http.MethodGet, "/api/v1/admin/users?role=INVALID_ROLE", "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for invalid role, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	})

	// ==================== F5: Business Unarchive Restores Branches ====================
	t.Run("F5_UnarchiveRestoresBranches", func(t *testing.T) {
		const b2ID = "019294f0-0000-7000-8000-000000000102"
		const b2BranchID = "019294f0-0000-7000-8000-000000000203"

		// 1. Verify initially both business and branch are unarchived
		var busArchived, brArchived bool
		if err := pool.QueryRow(ctx, "SELECT archived_at IS NOT NULL FROM schema_organization.business WHERE id = $1::uuid", b2ID).Scan(&busArchived); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT archived_at IS NOT NULL FROM schema_organization.branch WHERE id = $1::uuid", b2BranchID).Scan(&brArchived); err != nil {
			t.Fatal(err)
		}
		if busArchived || brArchived {
			t.Fatalf("expected B2 and its branch to be active initially, got busArchived=%v brArchived=%v", busArchived, brArchived)
		}

		// 2. Archive B2
		rec := call(http.MethodDelete, "/api/v1/admin/businesses/"+b2ID, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("archive business failed: status=%d body=%s", rec.Code, rec.Body.String())
		}

		// Verify both are archived in DB
		if err := pool.QueryRow(ctx, "SELECT archived_at IS NOT NULL FROM schema_organization.business WHERE id = $1::uuid", b2ID).Scan(&busArchived); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT archived_at IS NOT NULL FROM schema_organization.branch WHERE id = $1::uuid", b2BranchID).Scan(&brArchived); err != nil {
			t.Fatal(err)
		}
		if !busArchived || !brArchived {
			t.Fatalf("expected both to be archived after DELETE, got busArchived=%v brArchived=%v", busArchived, brArchived)
		}

		// 3. Unarchive B2
		rec = call(http.MethodPost, "/api/v1/admin/businesses/"+b2ID+"/unarchive", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("unarchive business failed: status=%d body=%s", rec.Code, rec.Body.String())
		}

		// 4. Verify both business and branch have archived_at = NULL in DB
		if err := pool.QueryRow(ctx, "SELECT archived_at IS NOT NULL FROM schema_organization.business WHERE id = $1::uuid", b2ID).Scan(&busArchived); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT archived_at IS NOT NULL FROM schema_organization.branch WHERE id = $1::uuid", b2BranchID).Scan(&brArchived); err != nil {
			t.Fatal(err)
		}
		if busArchived {
			t.Errorf("expected business B2 to have archived_at = NULL after unarchive")
		}
		if brArchived {
			t.Errorf("expected branch B2-SUR to be restored (archived_at = NULL) when business B2 was unarchived")
		}
	})
}
