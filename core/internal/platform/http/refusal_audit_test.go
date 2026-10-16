package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// recordedRefusal is one call observed by the fake auditor.
type recordedRefusal struct {
	method      string
	path        string
	status      int
	problemType string
	detail      string
	actor       *string
}

// fakeAuditor collects the refusals the middleware reports, so the audit rule
// can be asserted directly instead of only through a live database.
type fakeAuditor struct {
	mu      sync.Mutex
	records []recordedRefusal
}

func (f *fakeAuditor) RecordHTTPRejAudit(_ context.Context, method, path string, status int, problemType, detail string, actorAccountID *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, recordedRefusal{
		method:      method,
		path:        path,
		status:      status,
		problemType: problemType,
		detail:      detail,
		actor:       actorAccountID,
	})
}

func (f *fakeAuditor) recorded() []recordedRefusal {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRefusal(nil), f.records...)
}

// TestRefusalAuditMiddlewareAuditsRefusalsAndPassesThemThrough proves the rule the
// middleware exists for: exactly one append-only audit row per refusal, written
// before the response reaches the client, and a successful response that is
// passed through byte-identical and never audited.
func TestRefusalAuditMiddlewareAuditsRefusalsAndPassesThemThrough(t *testing.T) {
	t.Parallel()

	const successBody = `{"ok":true}`

	tests := []struct {
		name        string
		handler     http.Handler
		method      string
		path        string
		wantStatus  int
		wantBody    string
		wantRecords int
		wantType    string
		wantActor   string
	}{
		{
			name: "a successful response is passed through and never audited",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(successBody))
			}),
			method:      http.MethodGet,
			path:        "/api/v1/me",
			wantStatus:  http.StatusOK,
			wantBody:    successBody,
			wantRecords: 0,
		},
		{
			name: "a problem refusal records one row with method, path, status and type",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				WriteProblem(w, http.StatusForbidden, TypeForbidden, "The authenticated actor is not permitted.")
			}),
			method:      http.MethodGet,
			path:        "/api/v1/me",
			wantStatus:  http.StatusForbidden,
			wantRecords: 1,
			wantType:    TypeForbidden,
		},
		{
			name: "the authenticated actor is attributed to the refusal row",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				SetRequestActorID(r.Context(), "019294f0-0000-7000-8000-000000000002")
				WriteProblem(w, http.StatusNotFound, TypeNotFound, "The requested resource was not found.")
			}),
			method:      http.MethodDelete,
			path:        "/api/v1/businesses/019294f0-0000-7000-8000-000000000101/memberships/x",
			wantStatus:  http.StatusNotFound,
			wantRecords: 1,
			wantType:    TypeNotFound,
			wantActor:   "019294f0-0000-7000-8000-000000000002",
		},
		{
			name: "a refusal without a problem body still records the default type for its status",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}),
			method:      http.MethodGet,
			path:        "/api/v1/me",
			wantStatus:  http.StatusInternalServerError,
			wantRecords: 1,
			wantType:    TypeInternal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			auditor := &fakeAuditor{}
			handler := RefusalAuditMiddleware(auditor)(tc.handler)

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && rec.Body.String() != tc.wantBody {
				t.Fatalf("body = %q, want %q", rec.Body.String(), tc.wantBody)
			}

			records := auditor.recorded()
			if len(records) != tc.wantRecords {
				t.Fatalf("audited refusals = %d, want %d (%+v)", len(records), tc.wantRecords, records)
			}
			if tc.wantRecords == 0 {
				return
			}

			got := records[0]
			if got.method != tc.method || got.path != tc.path || got.status != tc.wantStatus {
				t.Fatalf("audit row = %+v, want method=%s path=%s status=%d", got, tc.method, tc.path, tc.wantStatus)
			}
			if got.problemType != tc.wantType {
				t.Fatalf("audit problem type = %q, want %q", got.problemType, tc.wantType)
			}
			if strings.TrimSpace(got.detail) == "" {
				t.Fatalf("audit row carries no detail: %+v", got)
			}
			switch {
			case tc.wantActor == "" && got.actor != nil:
				t.Fatalf("audit actor = %q, want null", *got.actor)
			case tc.wantActor != "" && (got.actor == nil || *got.actor != tc.wantActor):
				t.Fatalf("audit actor = %v, want %q", got.actor, tc.wantActor)
			}
		})
	}
}

// TestRouterFallbacksAreAuditedWithTheFrozenCatalog proves the composition the
// design requires at runtime (docs/design-identity-endpoints.md section 1.4): an
// unrouted path and a wrong method both answer a problem detail and both reach
// the refusal auditor exactly once, never as plain text.
func TestRouterFallbacksAreAuditedWithTheFrozenCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantType   string
	}{
		{
			name:       "an unrouted path is 404 not-found",
			method:     http.MethodGet,
			path:       "/api/v1/nope",
			wantStatus: http.StatusNotFound,
			wantType:   TypeNotFound,
		},
		{
			name:       "a wrong method is 405 method-not-allowed",
			method:     http.MethodPost,
			path:       "/api/v1/ping",
			wantStatus: http.StatusMethodNotAllowed,
			wantType:   TypeMethodNotAllowed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			auditor := &fakeAuditor{}
			router, _ := NewRouter(auditor)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
				t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
			}

			var problem ProblemDetail
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem detail %q: %v", rec.Body.String(), err)
			}
			if problem.Type != tc.wantType {
				t.Fatalf("problem type = %q, want %q", problem.Type, tc.wantType)
			}

			records := auditor.recorded()
			if len(records) != 1 {
				t.Fatalf("audited refusals = %d, want exactly 1 (%+v)", len(records), records)
			}
			if records[0].problemType != tc.wantType || records[0].status != tc.wantStatus {
				t.Fatalf("audit row = %+v, want type=%s status=%d", records[0], tc.wantType, tc.wantStatus)
			}
		})
	}
}

// TestNewRouterWithoutAnAuditorStillServesTheProbes proves the variadic wiring of
// NewRouter keeps the router usable when no auditor is supplied.
func TestNewRouterWithoutAnAuditorStillServesTheProbes(t *testing.T) {
	t.Parallel()

	router, _ := NewRouter()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("health probe status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}
