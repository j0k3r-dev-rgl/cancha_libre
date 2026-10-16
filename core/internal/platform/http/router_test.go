package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	platformhttp "canchalibre/core/internal/platform/http"
)

func TestHealthAndPingEndpoints(t *testing.T) {
	t.Parallel()

	router, _ := platformhttp.NewRouter()

	for _, tc := range []struct {
		path string
		key  string
		want string
	}{
		{path: "/health", key: "status", want: "ok"},
		{path: "/api/v1/ping", key: "message", want: "pong"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", tc.path, rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: unmarshal: %v", tc.path, err)
		}
		if body[tc.key] != tc.want {
			t.Fatalf("%s: expected %s=%q, got %q", tc.path, tc.key, tc.want, body[tc.key])
		}
	}
}

func TestUnrouted404AndWrongMethod405ReturnRFC7807(t *testing.T) {
	t.Parallel()

	router, _ := platformhttp.NewRouter()

	// Unrouted path -> 404 urn:canchalibre:error:not-found
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	rec404 := httptest.NewRecorder()
	router.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec404.Code)
	}
	if ct := rec404.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("expected application/problem+json for 404, got %q", ct)
	}
	var prob404 platformhttp.ProblemDetail
	if err := json.Unmarshal(rec404.Body.Bytes(), &prob404); err != nil {
		t.Fatalf("unmarshal 404 problem: %v", err)
	}
	if prob404.Type != platformhttp.TypeNotFound {
		t.Fatalf("expected type %q, got %q", platformhttp.TypeNotFound, prob404.Type)
	}

	// Wrong method -> 405 urn:canchalibre:error:method-not-allowed
	req405 := httptest.NewRequest(http.MethodPost, "/api/v1/ping", nil)
	rec405 := httptest.NewRecorder()
	router.ServeHTTP(rec405, req405)
	if rec405.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec405.Code)
	}
	if ct := rec405.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("expected application/problem+json for 405, got %q", ct)
	}
	var prob405 platformhttp.ProblemDetail
	if err := json.Unmarshal(rec405.Body.Bytes(), &prob405); err != nil {
		t.Fatalf("unmarshal 405 problem: %v", err)
	}
	if prob405.Type != platformhttp.TypeMethodNotAllowed {
		t.Fatalf("expected type %q, got %q", platformhttp.TypeMethodNotAllowed, prob405.Type)
	}
}

func TestTotalFailClosedErrorOverrideNeverEmitsAboutBlank(t *testing.T) {
	t.Parallel()

	_, _ = platformhttp.NewRouter()

	for _, status := range []int{400, 401, 403, 404, 405, 409, 415, 422, 500, 418, 503} {
		errModel := platformhttp.NewStatusError(status, "test error")
		raw, err := json.Marshal(errModel)
		if err != nil {
			t.Fatalf("marshal error model for %d: %v", status, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal error model for %d: %v", status, err)
		}
		typ, _ := decoded["type"].(string)
		if typ == "" || typ == "about:blank" || !strings.HasPrefix(typ, "urn:canchalibre:error:") {
			t.Fatalf("status %d produced invalid problem type %q", status, typ)
		}
	}
}

func TestOpenAPIContractInvariants(t *testing.T) {
	t.Parallel()

	router, _ := platformhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /openapi.json, got %d", rec.Code)
	}

	var document struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title       string `json:"title"`
			Version     string `json:"version"`
			Description string `json:"description"`
		} `json:"info"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tags"`
		Paths map[string]map[string]struct {
			OperationID string                     `json:"operationId"`
			Tags        []string                   `json:"tags"`
			Responses   map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Description string `json:"description"`
				Properties  map[string]struct {
					Type     json.RawMessage `json:"type"`
					ReadOnly bool            `json:"readOnly"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("unmarshal openapi: %v", err)
	}
	if !strings.HasPrefix(document.OpenAPI, "3.1") {
		t.Fatalf("expected OpenAPI 3.1, got %q", document.OpenAPI)
	}
	if document.Info.Version != platformhttp.APIVersion || strings.TrimSpace(document.Info.Description) == "" {
		t.Fatalf("unexpected info section: %+v", document.Info)
	}
	if len(document.Servers) == 0 || document.Servers[0].URL != platformhttp.APIServerURL {
		t.Fatalf("unexpected servers: %+v", document.Servers)
	}

	for _, schemaName := range []string{"HealthOutputBody", "PingOutputBody"} {
		schema, ok := document.Components.Schemas[schemaName]
		if !ok {
			t.Fatalf("missing schema %q", schemaName)
		}
		if strings.TrimSpace(schema.Description) == "" {
			t.Fatalf("schema %q has empty description", schemaName)
		}
		f, ok := schema.Properties["$schema"]
		if !ok || string(f.Type) != `"string"` || !f.ReadOnly {
			t.Fatalf("schema %q missing readOnly string $schema", schemaName)
		}
	}

	for path, methods := range document.Paths {
		for method, op := range methods {
			hasErr := false
			for st := range op.Responses {
				if code, err := strconv.Atoi(st); err == nil && code >= 400 {
					hasErr = true
				}
			}
			if !hasErr {
				t.Fatalf("%s %s documents no error response", strings.ToUpper(method), path)
			}
		}
	}
}
