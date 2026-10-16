package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platformhttp "canchalibre/core/internal/platform/http"
)

// TestHTTPRefusalsUseTheFrozenProblemCatalog exercises the refusals that are
// decided before any database query runs: the per-request guard rejects missing
// or malformed credentials, and huma's own negotiation rejects the wrong media
// type and an unparsable body. Each case asserts the documented status and the
// exact catalog URN on the wire, because the contract test only proves what the
// document declares, not what the process serves.
func TestHTTPRefusalsUseTheFrozenProblemCatalog(t *testing.T) {
	router := buildRouterForTest(t)

	businessID := "019294f0-0000-7000-8000-000000000101"

	cases := []struct {
		name        string
		method      string
		path        string
		authorize   string
		contentType string
		body        string
		wantStatus  int
		wantType    string
	}{
		{
			name:       "logout without a credential is 401 unauthenticated",
			method:     http.MethodPost,
			path:       "/api/v1/auth/logout",
			wantStatus: http.StatusUnauthorized,
			wantType:   platformhttp.TypeUnauthenticated,
		},
		{
			name:       "own account without a credential is 401 unauthenticated",
			method:     http.MethodGet,
			path:       "/api/v1/me",
			wantStatus: http.StatusUnauthorized,
			wantType:   platformhttp.TypeUnauthenticated,
		},
		{
			name:       "admin co-owner list without a credential is 401 unauthenticated",
			method:     http.MethodPost,
			path:       "/api/v1/admin/businesses/" + businessID + "/co-owners",
			wantStatus: http.StatusUnauthorized,
			wantType:   platformhttp.TypeUnauthenticated,
		},
		{
			name:       "a malformed bearer credential is 401 unauthenticated",
			method:     http.MethodGet,
			path:       "/api/v1/me",
			authorize:  "Bearer not-a-jwt",
			wantStatus: http.StatusUnauthorized,
			wantType:   platformhttp.TypeUnauthenticated,
		},
		{
			name:        "an unsupported content type is 415 unsupported-media-type",
			method:      http.MethodPost,
			path:        "/api/v1/auth/login",
			contentType: "text/plain",
			body:        "{}",
			wantStatus:  http.StatusUnsupportedMediaType,
			wantType:    platformhttp.TypeUnsupportedMediaType,
		},
		{
			name:        "an unparsable body is 400 validation",
			method:      http.MethodPost,
			path:        "/api/v1/auth/login",
			contentType: "application/json",
			body:        `{"email":`,
			wantStatus:  http.StatusBadRequest,
			wantType:    platformhttp.TypeValidation,
		},
		{
			name:        "a wrong field type is 422 validation",
			method:      http.MethodPost,
			path:        "/api/v1/auth/login",
			contentType: "application/json",
			body:        `{"email":5}`,
			wantStatus:  http.StatusUnprocessableEntity,
			wantType:    platformhttp.TypeValidation,
		},
		{
			name:       "an unrouted path is 404 not-found",
			method:     http.MethodGet,
			path:       "/api/v1/does-not-exist",
			wantStatus: http.StatusNotFound,
			wantType:   platformhttp.TypeNotFound,
		},
		{
			name:       "a wrong method is 405 method-not-allowed",
			method:     http.MethodPost,
			path:       "/api/v1/ping",
			wantStatus: http.StatusMethodNotAllowed,
			wantType:   platformhttp.TypeMethodNotAllowed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.authorize != "" {
				req.Header.Set("Authorization", tc.authorize)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d (body %s)", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
				t.Fatalf("expected application/problem+json, got %q", contentType)
			}
			var problem platformhttp.ProblemDetail
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem detail %q: %v", rec.Body.String(), err)
			}
			if problem.Type != tc.wantType {
				t.Fatalf("expected problem type %q, got %q", tc.wantType, problem.Type)
			}
			if problem.Type == "about:blank" || problem.Status != tc.wantStatus {
				t.Fatalf("problem detail is not catalog shaped: %+v", problem)
			}
		})
	}
}
