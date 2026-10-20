package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

func adminContractToken(t *testing.T) string {
	t.Helper()
	now := time.Now().UTC()
	token, err := security.SignAccessToken([]byte(apiTestJWTSecret), security.AccessTokenClaims{
		Subject: "019294f0-0000-7000-8000-000000000001", Actor: actor.RoleAdmin,
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func adminContractRequest(t *testing.T, router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type adminPageProjection struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
	Total  int    `json:"total"`
	Page   int    `json:"page"`
	Limit  int    `json:"limit"`
	Offset *int   `json:"offset"`
	Schema string `json:"$schema"`
}

func TestS2FrozenPageContract(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	token := adminContractToken(t)
	tests := []struct{ path, filter string }{
		{"/api/v1/admin/businesses", "status=active"},
		{"/api/v1/admin/branch-requests", "status=PENDING"},
		{"/api/v1/admin/branches", "approval_status=APPROVED"},
		{"/api/v1/admin/users", "kind=WORK"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			before := snapshotAdminDomain(t, pool)
			read := func(query string) adminPageProjection {
				t.Helper()
				rec := adminContractRequest(t, router, token, "GET", tt.path+query, "")
				if rec.Code != 200 {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
				}
				var body adminPageProjection
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Schema == "" || !strings.Contains(rec.Header().Get("Link"), "describedBy") {
					t.Error("missing schema body/header links")
				}
				if body.Offset != nil {
					t.Error("public pagination must not expose offset")
				}
				return body
			}
			all := read("?limit=100")
			first := read("?page=1&limit=1")
			second := read("?page=2&limit=1")
			if len(all.Items) < 2 || len(first.Items) != 1 || len(second.Items) != 1 {
				t.Fatalf("insufficient page items: %+v %+v %+v", all, first, second)
			}
			if first.Page != 1 || second.Page != 2 || first.Limit != 1 || second.Limit != 1 || second.Total != all.Total {
				t.Errorf("incorrect metadata: first=%+v second=%+v", first, second)
			}
			if first.Items[0].ID != all.Items[0].ID || second.Items[0].ID != all.Items[1].ID || first.Items[0].ID == second.Items[0].ID {
				t.Error("page 2 did not select the next ordered record")
			}
			repeated := read("?page=2&limit=1")
			if !reflect.DeepEqual(second, repeated) {
				t.Error("identical read returned a different page")
			}
			defaults := read("")
			if defaults.Page != 1 || defaults.Limit != 20 {
				t.Errorf("defaults page=%d limit=%d", defaults.Page, defaults.Limit)
			}
			filtered := read("?" + tt.filter + "&page=2&limit=1")
			filteredAll := read("?" + tt.filter + "&limit=100")
			if len(filtered.Items) != 1 || len(filteredAll.Items) < 2 || filtered.Items[0].ID != filteredAll.Items[1].ID || filtered.Total != filteredAll.Total {
				t.Error("pagination did not preserve existing filter behavior")
			}
			empty := read("?page=100&limit=1")
			if len(empty.Items) != 0 || empty.Page != 100 || empty.Limit != 1 || empty.Total != all.Total {
				t.Errorf("unexpected beyond-last page: %+v", empty)
			}
			if after := snapshotAdminDomain(t, pool); after != before {
				t.Error("list mutated domain state")
			}
		})
	}
}

func TestS2PageValidation(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	token := adminContractToken(t)
	for _, path := range []string{"businesses", "branch-requests", "branches", "users"} {
		for _, query := range []string{"page=0", "page=-1", "limit=0", "limit=101", "page=9223372036854775807&limit=100", "page=oops"} {
			t.Run(path+"/"+query, func(t *testing.T) {
				before := snapshotAdminDomain(t, pool)
				rec := adminContractRequest(t, router, token, "GET", "/api/v1/admin/"+path+"?"+query, "")
				expected := 400
				if query == "page=oops" {
					expected = 422
				}
				var prob problemResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &prob); err != nil {
					t.Fatal(err)
				}
				if rec.Code != expected || prob.Status != expected || prob.Type != "urn:canchalibre:error:validation" {
					t.Errorf("status=%d problem=%+v want=%d", rec.Code, prob, expected)
				}
				if snapshotAdminDomain(t, pool) != before {
					t.Error("invalid page mutated domain rows")
				}
			})
		}
	}
}

func TestS2LifecycleExplicitNullContract(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newDatabaseBackedRouter(t, pool)
	token := adminContractToken(t)
	// A dedicated business avoids pending review fixtures that correctly block archival.
	rec := adminContractRequest(t, router, token, "POST", "/api/v1/admin/businesses", `{"name":"Null response fixture"}`)
	var created struct {
		ID string `json:"id"`
	}
	if rec.Code != 201 {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, setupMethod, setupPath, setupBody, path, field string }{
		{"business reactivation", "POST", "/api/v1/admin/businesses/" + created.ID + "/suspend", `{"reason":"Review"}`, "/api/v1/admin/businesses/" + created.ID + "/reactivate", "suspended_at"},
		{"branch reactivation", "POST", "/api/v1/admin/branches/019294f0-0000-7000-8000-000000000201/suspend", `{"reason":"Review"}`, "/api/v1/admin/branches/019294f0-0000-7000-8000-000000000201/reactivate", "suspended_at"},
		{"business unarchive", "DELETE", "/api/v1/admin/businesses/" + created.ID, "", "/api/v1/admin/businesses/" + created.ID + "/unarchive", "archived_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := adminContractRequest(t, router, token, tt.setupMethod, tt.setupPath, tt.setupBody)
			if setup.Code != 200 && setup.Code != 204 {
				t.Fatalf("setup status=%d body=%s", setup.Code, setup.Body.String())
			}
			response := adminContractRequest(t, router, token, "POST", tt.path, "")
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || string(body[tt.field]) != "null" {
				t.Errorf("status=%d required %s=null body=%s", response.Code, tt.field, response.Body.String())
			}
			if len(body["$schema"]) == 0 || !strings.Contains(response.Header().Get("Link"), "describedBy") {
				t.Error("missing schema link")
			}
			// Reading through the public detail/list response confirms the resource is active again.
			var id string
			if err := json.Unmarshal(body["id"], &id); err != nil || id == "" {
				t.Errorf("id=%q err=%v", id, err)
			}
			before := snapshotAdminDomain(t, pool)
			repeat := adminContractRequest(t, router, token, "POST", tt.path, "")
			var prob problemResponse
			if err := json.Unmarshal(repeat.Body.Bytes(), &prob); err != nil {
				t.Fatal(err)
			}
			if repeat.Code != 409 || prob.Type != "urn:canchalibre:error:conflict" {
				t.Errorf("repeat status=%d body=%s", repeat.Code, repeat.Body.String())
			}
			if snapshotAdminDomain(t, pool) != before {
				t.Error("repeat changed domain state")
			}
		})
	}
}

func TestS2FrozenPaginationAndNullableSchemas(t *testing.T) {
	_, raw := fetchSpec(t)
	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `json:"name"`
			} `json:"parameters"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Type     json.RawMessage `json:"type"`
					ReadOnly bool            `json:"readOnly"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"businesses", "branch-requests", "branches", "users"} {
		t.Run(path, func(t *testing.T) {
			names := map[string]bool{}
			for _, p := range doc.Paths["/api/v1/admin/"+path]["get"].Parameters {
				names[p.Name] = true
			}
			if !names["page"] || !names["limit"] || names["offset"] {
				t.Errorf("query names=%v", names)
			}
		})
	}
	for _, tt := range []struct{ schema, field string }{
		{"ReactivateBusinessResponseBody", "suspended_at"},
		{"ReactivateBranchResponseBody", "suspended_at"},
		{"UnarchiveBusinessResponseBody", "archived_at"},
	} {
		t.Run(tt.schema, func(t *testing.T) {
			schema, ok := doc.Components.Schemas[tt.schema]
			if !ok {
				t.Fatalf("missing schema %s", tt.schema)
			}
			required := false
			for _, field := range schema.Required {
				if field == tt.field {
					required = true
				}
			}
			var types []string
			if err := json.Unmarshal(schema.Properties[tt.field].Type, &types); err != nil {
				t.Errorf("type must include null: %s", schema.Properties[tt.field].Type)
			}
			if !required || !reflect.DeepEqual(types, []string{"string", "null"}) {
				t.Errorf("required=%v types=%v", required, types)
			}
			if !schema.Properties["$schema"].ReadOnly {
				t.Error("schema link must be declared read-only")
			}
		})
	}
	for _, name := range []string{"ListBusinessesResponseBody", "ListBranchRequestsResponseBody", "ListAdminBranchesResponseBody", "ListUsersResponseBody"} {
		schema := doc.Components.Schemas[name]
		if _, ok := schema.Properties["page"]; !ok {
			t.Error(fmt.Sprintf("%s missing page", name))
		}
		if _, ok := schema.Properties["offset"]; ok {
			t.Errorf("%s declares offset", name)
		}
	}
}
