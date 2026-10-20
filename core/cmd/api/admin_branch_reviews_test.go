package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

func TestAdminBranchReviewFixturesHTTP(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	ctx := context.Background()
	const prefix = "019294f0-0000-7000-8000-"
	tokens := map[actor.Role]string{}
	for role, suffix := range map[actor.Role]string{actor.RoleAdmin: "000000000001", actor.RoleSoporte: "000000000005"} {
		now := time.Now().UTC()
		token, err := security.SignAccessToken([]byte(apiTestJWTSecret), security.AccessTokenClaims{
			Subject: prefix + suffix, Actor: role, IssuedAt: now, ExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		tokens[role] = token
	}
	call := func(role actor.Role, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokens[role])
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	readRound := func(id string) string {
		t.Helper()
		var row string
		if err := pool.QueryRow(ctx,
			`SELECT to_jsonb(h)::text FROM schema_organization.branch_approval_history h WHERE id = $1::uuid`,
			id).Scan(&row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	prior := readRound(prefix + "000000000400")
	tests := []struct {
		name, requestSuffix, branchSuffix, action, status string
		role                                              actor.Role
	}{
		{"admin approve", "000000000401", "000000000211", "approve", "APPROVED", actor.RoleAdmin},
		{"admin reject", "000000000402", "000000000212", "reject", "REJECTED", actor.RoleAdmin},
		{"support approve", "000000000403", "000000000213", "approve", "APPROVED", actor.RoleSoporte},
		{"support reject", "000000000404", "000000000214", "reject", "REJECTED", actor.RoleSoporte},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/v1/admin/branch-requests/" + prefix + tt.requestSuffix
			detail := call(tt.role, http.MethodGet, path, "")
			if detail.Code != http.StatusOK {
				t.Fatalf("fixture detail: status=%d body=%s", detail.Code, detail.Body.String())
			}
			var pending struct {
				ID, Status string
				History    []json.RawMessage
			}
			if err := json.Unmarshal(detail.Body.Bytes(), &pending); err != nil {
				t.Fatal(err)
			}
			if pending.ID != prefix+tt.requestSuffix || pending.Status != "PENDING" || len(pending.History) == 0 {
				t.Fatalf("unexpected fixture detail: %+v", pending)
			}
			body := ""
			if tt.action == "reject" {
				body = `{"rejection_reason":"Confirm address"}`
			}
			rec := call(tt.role, http.MethodPost, path+"/"+tt.action, body)
			if rec.Code != http.StatusOK {
				t.Fatalf("review: status=%d body=%s", rec.Code, rec.Body.String())
			}
			var response struct {
				ID              string     `json:"id"`
				BranchID        string     `json:"branch_id"`
				Status          string     `json:"status"`
				ReviewedBy      string     `json:"reviewed_by"`
				ReviewedAt      *time.Time `json:"reviewed_at"`
				RejectionReason string     `json:"rejection_reason"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			expectedReviewer := prefix + "000000000001"
			if tt.role == actor.RoleSoporte {
				expectedReviewer = prefix + "000000000005"
			}
			if response.ID != prefix+tt.requestSuffix || response.BranchID != prefix+tt.branchSuffix ||
				response.Status != tt.status || response.ReviewedBy != expectedReviewer || response.ReviewedAt == nil {
				t.Fatalf("unexpected review response: %+v", response)
			}
			if tt.action == "reject" && response.RejectionReason != "Confirm address" {
				t.Fatalf("rejection reason=%q", response.RejectionReason)
			}
			var branchStatus, reviewer string
			var reviewedAt time.Time
			if err := pool.QueryRow(ctx, `SELECT approval_status FROM schema_organization.branch WHERE id = $1::uuid`,
				response.BranchID).Scan(&branchStatus); err != nil || branchStatus != tt.status {
				t.Fatalf("branch status=%s err=%v", branchStatus, err)
			}
			if err := pool.QueryRow(ctx, `SELECT reviewed_by::text, reviewed_at
				FROM schema_organization.branch_approval_history WHERE id = $1::uuid`,
				response.ID).Scan(&reviewer, &reviewedAt); err != nil || reviewer != response.ReviewedBy ||
				!reviewedAt.Equal(*response.ReviewedAt) {
				t.Fatalf("persisted reviewer=%s reviewed_at=%s err=%v", reviewer, reviewedAt, err)
			}
			before := snapshotAdminDomain(t, pool)
			repeated := call(tt.role, http.MethodPost, path+"/"+tt.action, body)
			var problem problemResponse
			if err := json.Unmarshal(repeated.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if repeated.Code != http.StatusConflict || problem.Type != "urn:canchalibre:error:conflict" {
				t.Fatalf("resolved repeat: status=%d problem=%+v", repeated.Code, problem)
			}
			if after := snapshotAdminDomain(t, pool); before != after {
				t.Fatal("resolved repeat mutated domain rows")
			}
		})
	}
	if after := readRound(prefix + "000000000400"); after != prior {
		t.Fatal("resolving the new request changed the previous review round")
	}

	// Two public requests compete for the same pending round. Synchronize their
	// start rather than sleeping; only the winning transition may change state.
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	path := "/api/v1/admin/branch-requests/" + prefix + "000000000405"
	go func() {
		<-start
		results <- call(actor.RoleAdmin, http.MethodPost, path+"/approve", "")
	}()
	go func() {
		<-start
		results <- call(actor.RoleSoporte, http.MethodPost, path+"/reject", `{"rejection_reason":"Confirm address"}`)
	}()
	close(start)
	codes := map[int]int{}
	winnerStatus := ""
	for range 2 {
		rec := <-results
		codes[rec.Code]++
		if rec.Code == http.StatusConflict {
			var problem problemResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Type != "urn:canchalibre:error:conflict" {
				t.Errorf("race refusal body=%s err=%v", rec.Body.String(), err)
			}
		} else if rec.Code == http.StatusOK {
			var result struct{ Status string }
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			winnerStatus = result.Status
		}
	}
	if !reflect.DeepEqual(codes, map[int]int{http.StatusOK: 1, http.StatusConflict: 1}) {
		t.Fatalf("competing review status counts=%v", codes)
	}
	var branchStatus, requestStatus string
	if err := pool.QueryRow(ctx, `SELECT b.approval_status, h.status
		FROM schema_organization.branch b
		JOIN schema_organization.branch_approval_history h ON h.branch_id = b.id
		WHERE h.id = $1::uuid`, prefix+"000000000405").Scan(&branchStatus, &requestStatus); err != nil {
		t.Fatal(err)
	}
	if branchStatus != winnerStatus || requestStatus != winnerStatus {
		t.Fatalf("race state: branch=%s request=%s winner=%s", branchStatus, requestStatus, winnerStatus)
	}
}
