package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	adapterhttp "canchalibre/core/internal/adapter/http"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status 'ok', got '%s'", body["status"])
	}
}

func TestPingEndpoint(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if body["message"] != "pong" {
		t.Fatalf("expected message 'pong', got '%s'", body["message"])
	}
}

type openAPIOperation struct {
	OperationID string                     `json:"operationId"`
	Responses   map[string]json.RawMessage `json:"responses"`
}

func TestOpenAPIContractIsServedAsVersion31(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected the OpenAPI document to be served, got status %d", rec.Code)
	}

	var document struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title   string `json:"title"`
			Version string `json:"version"`
		} `json:"info"`
		Paths map[string]map[string]openAPIOperation `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("failed to parse the OpenAPI document: %v", err)
	}

	if !strings.HasPrefix(document.OpenAPI, "3.1") {
		t.Fatalf("expected an OpenAPI 3.1 document, got %q", document.OpenAPI)
	}

	if document.Info.Version != adapterhttp.APIVersion {
		t.Fatalf("expected the contract to report API version %q, got %q", adapterhttp.APIVersion, document.Info.Version)
	}

	for _, target := range []struct {
		path   string
		method string
	}{
		{path: "/health", method: "get"},
		{path: "/api/v1/ping", method: "get"},
	} {
		operation, documented := document.Paths[target.path][target.method]
		if !documented {
			t.Fatalf("expected %s %s to be documented in the contract", strings.ToUpper(target.method), target.path)
		}

		if _, ok := operation.Responses["200"]; !ok {
			t.Fatalf("expected %s %s to document its success response", strings.ToUpper(target.method), target.path)
		}

		if !documentsAnErrorResponse(operation.Responses) {
			t.Fatalf("expected %s %s to document an error response", strings.ToUpper(target.method), target.path)
		}
	}
}

// TestResponseCarriesTheSchemaLinkConvention locks the decision recorded in
// docs/roadmap-backend.md (block T0) to keep huma's default schema link
// transformer. Every struct response body therefore carries a `$schema` field
// pointing at its schema document, plus the matching `Link` header.
func TestResponseCarriesTheSchemaLinkConvention(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	const schemaPath = "/schemas/HealthOutputBody.json"

	if body["$schema"] == "" {
		t.Fatalf("expected the response body to carry a non-empty $schema field, got %q", body["$schema"])
	}

	if !strings.HasSuffix(body["$schema"], schemaPath) {
		t.Fatalf("expected $schema to point at %q, got %q", schemaPath, body["$schema"])
	}

	wantLink := "<" + schemaPath + `>; rel="describedBy"`
	if got := rec.Header().Get("Link"); got != wantLink {
		t.Fatalf("expected Link header %q, got %q", wantLink, got)
	}
}

// TestPingResponseCarriesTheSchemaLinkConvention locks the second documented
// response DTO, so both schemas serviced today carry the convention and not
// only the one exercised by the health probe.
func TestPingResponseCarriesTheSchemaLinkConvention(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	const schemaPath = "/schemas/PingOutputBody.json"

	if !strings.HasSuffix(body["$schema"], schemaPath) {
		t.Fatalf("expected $schema to point at %q, got %q", schemaPath, body["$schema"])
	}

	wantLink := "<" + schemaPath + `>; rel="describedBy"`
	if got := rec.Header().Get("Link"); got != wantLink {
		t.Fatalf("expected Link header %q, got %q", wantLink, got)
	}
}

// TestContractDeclaresTheSchemaLinkField proves the extra `$schema` field is
// part of the published contract, so a response can never drift from it. This
// is why the field is a convention and not contract drift.
func TestContractDeclaresTheSchemaLinkField(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					// OpenAPI 3.1 allows `type` to be an array, so it is decoded raw.
					Type     json.RawMessage `json:"type"`
					ReadOnly bool            `json:"readOnly"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("failed to parse the OpenAPI document: %v", err)
	}

	for _, schemaName := range []string{"HealthOutputBody", "PingOutputBody"} {
		schema, published := document.Components.Schemas[schemaName]
		if !published {
			t.Fatalf("expected schema %q to be published in the contract", schemaName)
		}

		field, declared := schema.Properties["$schema"]
		if !declared {
			t.Fatalf("expected schema %q to declare the $schema property", schemaName)
		}

		if string(field.Type) != `"string"` || !field.ReadOnly {
			t.Fatalf("expected %s.$schema to be a read-only string, got type %s readOnly %t", schemaName, field.Type, field.ReadOnly)
		}
	}
}

// TestContractDescribesItself locks the contract-information rule: the served
// document must describe what the API is, not only its title.
func TestContractDescribesItself(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var document struct {
		Info struct {
			Description string `json:"description"`
		} `json:"info"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("failed to parse the OpenAPI document: %v", err)
	}

	if strings.TrimSpace(document.Info.Description) == "" {
		t.Fatal("expected the contract info section to carry a description")
	}
}

// TestContractDeclaresItsGlobalTags locks the tag rule: a tag used by an
// operation must be declared once at the document root with a description.
func TestContractDeclaresItsGlobalTags(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var document struct {
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tags"`
		Paths map[string]map[string]struct {
			Tags []string `json:"tags"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("failed to parse the OpenAPI document: %v", err)
	}

	declared := make(map[string]bool, len(document.Tags))
	for _, tag := range document.Tags {
		if strings.TrimSpace(tag.Description) == "" {
			t.Fatalf("expected the global tag %q to carry a description", tag.Name)
		}
		declared[tag.Name] = true
	}

	for path, methods := range document.Paths {
		for method, operation := range methods {
			for _, tag := range operation.Tags {
				if !declared[tag] {
					t.Fatalf("expected tag %q used by %s %s to be declared globally", tag, strings.ToUpper(method), path)
				}
			}
		}
	}
}

// TestContractDeclaresTheDevelopmentServer locks the server decision: the
// contract declares the local development base URL, which is also what makes
// the documented `$schema` example a truthful URL instead of a placeholder.
func TestContractDeclaresTheDevelopmentServer(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var document struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("failed to parse the OpenAPI document: %v", err)
	}

	if len(document.Servers) == 0 {
		t.Fatal("expected the contract to declare at least one server")
	}

	if document.Servers[0].URL != adapterhttp.APIServerURL {
		t.Fatalf("expected the contract to declare server %q, got %q", adapterhttp.APIServerURL, document.Servers[0].URL)
	}
}

// TestContractDescribesItsPublishedSchemas guards the ruleset silence: the
// `component-description` rule is off because huma's error schemas cannot be
// described by us, so a regression that left our own schemas undescribed would
// otherwise go unnoticed.
func TestContractDescribesItsPublishedSchemas(t *testing.T) {
	t.Parallel()

	router := adapterhttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var document struct {
		Components struct {
			Schemas map[string]struct {
				Description string `json:"description"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("failed to parse the OpenAPI document: %v", err)
	}

	for _, schemaName := range []string{"HealthOutputBody", "PingOutputBody"} {
		schema, published := document.Components.Schemas[schemaName]
		if !published {
			t.Fatalf("expected schema %q to be published in the contract", schemaName)
		}

		if strings.TrimSpace(schema.Description) == "" {
			t.Fatalf("expected schema %q to carry a description", schemaName)
		}
	}
}

// documentsAnErrorResponse reports whether the operation documents any error
// response, either through an explicit status code of 400 or above or through
// a default response.
func documentsAnErrorResponse(responses map[string]json.RawMessage) bool {
	for status := range responses {
		if status == "default" {
			return true
		}
		if code, err := strconv.Atoi(status); err == nil && code >= 400 {
			return true
		}
	}
	return false
}
