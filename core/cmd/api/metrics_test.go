package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"canchalibre/core/internal/platform/config"
	platformhttp "canchalibre/core/internal/platform/http"
)

// metricsTestToken is a 32-byte token, the minimum the configuration accepts.
const metricsTestToken = "0123456789abcdef0123456789abcdef"

// newMetricsBackedRouter builds the real composition root with metrics enabled, so
// the endpoint is exercised through the same wiring the process serves.
func newMetricsBackedRouter(t *testing.T) http.Handler {
	t.Helper()

	pool := setupAPITestDatabase(t)

	router, _ := newRouter(pool, config.Config{
		JWTSecret:         apiTestJWTSecret,
		InvitationURLBase: "http://localhost:3000/auth/activate",
		MetricsToken:      metricsTestToken,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return router
}

// TestMetricsEndpointServesTheRealRegistry is the end-to-end rule of S1 and S4
// through the composition root: the private path demands the token, counts a
// request that travelled the real router under the real route template, reports
// the real PostgreSQL pool, and never counts its own scrape.
func TestMetricsEndpointServesTheRealRegistry(t *testing.T) {
	router := newMetricsBackedRouter(t)

	// One product request through the same router, so the counter has a real
	// observation whose label comes from the real route tree.
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	// Without the token the private path refuses, and it refuses as a problem
	// document rather than a body-less 401.
	refused := httptest.NewRecorder()
	router.ServeHTTP(refused, httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil))
	if refused.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without the token, got %d", refused.Code)
	}
	if ct := refused.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("expected application/problem+json without the token, got %q", ct)
	}

	scrape := httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil)
	scrape.Header.Set("Authorization", "Bearer "+metricsTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, scrape)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with the token, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("expected the Prometheus text content type, got %q", ct)
	}

	body := rec.Body.String()

	if !strings.Contains(body, `canchalibre_http_requests_total{method="GET",route="/health",status="200"} 1`) {
		t.Fatalf("the served health request was not counted under its route template\n--- exposition ---\n%s", body)
	}
	if !strings.Contains(body, `canchalibre_db_pool_conns{state="total"}`) {
		t.Fatalf("the pool gauges are missing from the exposition\n--- exposition ---\n%s", body)
	}
	if strings.Contains(body, `route="/metrics"`) {
		t.Fatalf("the scrape counted itself\n--- exposition ---\n%s", body)
	}
}

// TestMetricsEndpointIsAbsentWithoutAToken is the S3 rule at the composition-root
// seam: an unset METRICS_TOKEN leaves the path unrouted instead of mounting a
// weaker endpoint.
func TestMetricsEndpointIsAbsentWithoutAToken(t *testing.T) {
	// buildRouterForTest is the same composition root with no METRICS_TOKEN set.
	router := buildRouterForTest(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with no METRICS_TOKEN, got %d", rec.Code)
	}
}
