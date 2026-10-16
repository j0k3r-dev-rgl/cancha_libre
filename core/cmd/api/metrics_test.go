package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/platform/config"
	platformhttp "canchalibre/core/internal/platform/http"
)

// metricsTestToken is a 32-byte token, the minimum the configuration accepts.
const metricsTestToken = "0123456789abcdef0123456789abcdef"

// newMetricsBackedRouter builds the real composition root with metrics enabled, so
// the endpoint is exercised through the same wiring the process serves.
func newMetricsBackedRouter(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()

	router, _ := newRouter(pool, config.Config{
		JWTSecret:         apiTestJWTSecret,
		InvitationURLBase: "http://localhost:3000/auth/activate",
		MetricsToken:      metricsTestToken,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return router
}

// scrapeMetrics performs one authenticated scrape and returns the exposition.
func scrapeMetrics(t *testing.T, router http.Handler) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil)
	req.Header.Set("Authorization", "Bearer "+metricsTestToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from the scrape, got %d", rec.Code)
	}
	return rec.Body.String()
}

// TestMetricsEndpointServesTheRealRegistry is the end-to-end rule of S1 and S4
// through the composition root: the private path demands the token, counts a
// request that travelled the real router under the real route template, reports
// the real PostgreSQL pool as it is at scrape time, never counts its own scrape,
// and stays out of the published contract even while it is mounted.
func TestMetricsEndpointServesTheRealRegistry(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newMetricsBackedRouter(t, pool)

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

	// The gauges must describe the pool as it is now, not as it was at wiring
	// time: the assertion compares against a reading taken by this test.
	wantTotal := fmt.Sprintf(`canchalibre_db_pool_conns{state="total"} %d`, pool.Stat().TotalConns())
	if !strings.Contains(body, wantTotal) {
		t.Fatalf("the pool gauge is not the live snapshot: want %q\n--- exposition ---\n%s", wantTotal, body)
	}
	if strings.Contains(body, `route="/metrics"`) {
		t.Fatalf("the scrape counted itself\n--- exposition ---\n%s", body)
	}

	// Mounted, the private path still must not appear in the document clients
	// read. The unconfigured case is covered by TestOpenAPISpecNeverPublishesThePrivateMetricsPath;
	// this one covers the case where the route really exists.
	spec := httptest.NewRecorder()
	router.ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if spec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /openapi.json, got %d", spec.Code)
	}
	if strings.Contains(spec.Body.String(), `"/metrics"`) {
		t.Fatal("the served contract publishes the private scrape path while it is mounted")
	}
}

// TestMetricsEndpointLabelsStayBoundedThroughTheRealRouter attacks the label rule
// on the served surface itself: a real identifier, an unknown path, an email in a
// query string and an invented method must leave the exposition untouched.
func TestMetricsEndpointLabelsStayBoundedThroughTheRealRouter(t *testing.T) {
	pool := setupAPITestDatabase(t)
	router := newMetricsBackedRouter(t, pool)

	const accountID = "9f5d5b1a-8a3a-4a5b-9c5e-2b1f0e7d6c4a"
	const victimEmail = "victim@example.com"

	for _, probe := range []struct {
		method string
		target string
	}{
		{http.MethodDelete, "/api/v1/admin/support-operators/" + accountID},
		{http.MethodGet, "/no/such/route/" + accountID + "/" + victimEmail},
		{http.MethodGet, "/health?email=" + victimEmail + "&token=abc123"},
		{"FROBNICATE", "/health"},
	} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(probe.method, probe.target, nil))
	}

	body := scrapeMetrics(t, router)

	for _, want := range []string{
		`route="/api/v1/admin/support-operators/{accountId}"`,
		`route="unmatched"`,
		`method="other"`,
		`route="/health"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected the bounded label %s\n--- exposition ---\n%s", want, body)
		}
	}
	for _, leaked := range []string{accountID, victimEmail, "victim", "abc123", "FROBNICATE", "Bearer"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("the exposition leaked %q\n--- exposition ---\n%s", leaked, body)
		}
	}
}

// TestMetricsEndpointIsAbsentWithoutAToken is the S3 rule at the composition-root
// seam: an unset METRICS_TOKEN leaves the path unrouted instead of mounting a
// weaker endpoint, and leaves no recorder behind either.
func TestMetricsEndpointIsAbsentWithoutAToken(t *testing.T) {
	// buildRouterForTest is the same composition root with no METRICS_TOKEN set.
	router := buildRouterForTest(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 with no METRICS_TOKEN, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("expected the unrouted problem document, got %q", ct)
	}
	if strings.Contains(rec.Body.String(), "canchalibre_") {
		t.Fatalf("the disabled endpoint rendered metrics: %s", rec.Body.String())
	}
}
