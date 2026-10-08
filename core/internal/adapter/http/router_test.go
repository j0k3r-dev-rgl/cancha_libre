package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
