package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	platformhttp "canchalibre/core/internal/platform/http"
)

const testMetricsToken = "0123456789abcdef0123456789abcdef"

// recordedRequest is one call the recording port received.
type recordedRequest struct {
	Method string
	Route  string
	Status int
}

// stubRecorder captures what the recording middleware reports, so the label a
// real registry would receive can be asserted without rendering metrics.
type stubRecorder struct {
	mu   sync.Mutex
	seen []recordedRequest
}

func (s *stubRecorder) RecordHTTPRequest(method, route string, status int, _ float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, recordedRequest{Method: method, Route: route, Status: status})
}

func (s *stubRecorder) requests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.seen...)
}

// stubWriter stands in for the metric registry so the HTTP surface can be tested
// on its own.
type stubWriter struct{ body string }

func (s stubWriter) WritePrometheus(w io.Writer) { _, _ = io.WriteString(w, s.body) }

// TestMetricsHandlerRefusesWithoutTheConfiguredBearerToken is the credential rule
// of S1: no header, a wrong token, a wrong scheme, a near miss and an unconfigured
// token all answer the catalog's 401 problem document and render no metrics.
func TestMetricsHandlerRefusesWithoutTheConfiguredBearerToken(t *testing.T) {
	t.Parallel()

	const exposition = "canchalibre_http_requests_total 1\n"

	for _, tc := range []struct {
		name    string
		header  string
		handler http.Handler
	}{
		{
			name:    "no authorization header",
			handler: platformhttp.MetricsHandler(stubWriter{body: exposition}, testMetricsToken),
		},
		{
			name:    "wrong token",
			header:  "Bearer 0123456789abcdef0123456789abcdeZ",
			handler: platformhttp.MetricsHandler(stubWriter{body: exposition}, testMetricsToken),
		},
		{
			name:    "wrong scheme",
			header:  "Basic MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
			handler: platformhttp.MetricsHandler(stubWriter{body: exposition}, testMetricsToken),
		},
		{
			name:    "token with trailing noise",
			header:  "Bearer " + testMetricsToken + "x",
			handler: platformhttp.MetricsHandler(stubWriter{body: exposition}, testMetricsToken),
		},
		{
			name:    "bearer scheme with no value",
			header:  "Bearer ",
			handler: platformhttp.MetricsHandler(stubWriter{body: exposition}, testMetricsToken),
		},
		{
			name:    "no configured token",
			header:  "Bearer " + testMetricsToken,
			handler: platformhttp.MetricsHandler(stubWriter{body: exposition}, ""),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
				t.Fatalf("expected application/problem+json, got %q", ct)
			}
			var problem platformhttp.ProblemDetail
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("unmarshal problem: %v", err)
			}
			if problem.Type != platformhttp.TypeUnauthenticated {
				t.Fatalf("expected type %q, got %q", platformhttp.TypeUnauthenticated, problem.Type)
			}
			if strings.Contains(rec.Body.String(), "canchalibre_") {
				t.Fatalf("refused request rendered metrics: %s", rec.Body.String())
			}
		})
	}
}

// TestMetricsHandlerServesTheExpositionWithTheBearerToken pins the positive half
// of the same rule: the configured token answers the Prometheus text exposition,
// and the scheme is matched case-insensitively as RFC 6750 defines it.
func TestMetricsHandlerServesTheExpositionWithTheBearerToken(t *testing.T) {
	t.Parallel()

	const exposition = "canchalibre_http_requests_total 1\n"

	for _, header := range []string{"Bearer " + testMetricsToken, "bearer " + testMetricsToken} {
		req := httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil)
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		platformhttp.MetricsHandler(stubWriter{body: exposition}, testMetricsToken).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%q: expected 200, got %d", header, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
			t.Fatalf("%q: expected the Prometheus text content type, got %q", header, ct)
		}
		if rec.Body.String() != exposition {
			t.Fatalf("%q: expected the exposition body, got %q", header, rec.Body.String())
		}
	}
}

// TestRecordRequestsLabelsTheRouteTemplateNotTheRawPath is the cardinality rule of
// S4: a parameterized route is reported as its template, so a thousand distinct
// identifiers produce one series instead of a thousand. The response itself must
// pass through the middleware unchanged.
func TestRecordRequestsLabelsTheRouteTemplateNotTheRawPath(t *testing.T) {
	t.Parallel()

	recorder := &stubRecorder{}
	router, _ := platformhttp.NewRouter(platformhttp.RouterOptions{Logger: discardLogger(), Recorder: recorder})

	// A parameterized route is registered here because this package owns the
	// router and the middleware, not the identity module: the rule under test is
	// which string the middleware reports, and the real operations exercise the
	// same chi route pattern in the composition-root tests.
	router.Method(http.MethodDelete, "/api/v1/things/{thingId}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "deleted")
	}))

	const rawThingID = "9f5d5b1a-8a3a-4a5b-9c5e-2b1f0e7d6c4a"
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/things/"+rawThingID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "deleted" {
		t.Fatalf("the recorded response changed: status %d body %q", rec.Code, rec.Body.String())
	}

	seen := recorder.requests()
	if len(seen) != 1 {
		t.Fatalf("expected exactly one recorded request, got %d (%+v)", len(seen), seen)
	}
	if seen[0].Route != "/api/v1/things/{thingId}" {
		t.Fatalf("expected the route template, got %q", seen[0].Route)
	}
	if strings.Contains(seen[0].Route, rawThingID) {
		t.Fatalf("the raw identifier reached the label value: %q", seen[0].Route)
	}
	if seen[0].Method != http.MethodDelete {
		t.Fatalf("expected DELETE, got %q", seen[0].Method)
	}
	if seen[0].Status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", seen[0].Status)
	}
}

// TestRecordRequestsNormalizesUnattributedAndUnknownMethods closes the other half
// of the cardinality rule: the method is a closed set, and a path the router never
// attributed has one fixed label instead of the requested path.
func TestRecordRequestsNormalizesUnattributedAndUnknownMethods(t *testing.T) {
	t.Parallel()

	recorder := &stubRecorder{}
	router, _ := platformhttp.NewRouter(platformhttp.RouterOptions{Logger: discardLogger(), Recorder: recorder})

	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	router.ServeHTTP(httptest.NewRecorder(), req404)

	reqUnknown := httptest.NewRequest("FOOBAR", "/health", nil)
	router.ServeHTTP(httptest.NewRecorder(), reqUnknown)

	seen := recorder.requests()
	if len(seen) != 2 {
		t.Fatalf("expected two recorded requests, got %d (%+v)", len(seen), seen)
	}
	if seen[0].Route != "unmatched" || seen[0].Status != http.StatusNotFound {
		t.Fatalf("unrouted request recorded as %+v", seen[0])
	}
	if seen[1].Method != "other" {
		t.Fatalf("unknown method recorded as %q, want %q", seen[1].Method, "other")
	}
	if seen[1].Status != http.StatusMethodNotAllowed {
		t.Fatalf("unknown method recorded status %d, want 405", seen[1].Status)
	}
}

// TestMetricsPathIsExcludedFromTheRequestCounter keeps the scrape traffic out of
// the product request series, and proves the private route is mounted when a
// handler is supplied.
func TestMetricsPathIsExcludedFromTheRequestCounter(t *testing.T) {
	t.Parallel()

	recorder := &stubRecorder{}
	handler := platformhttp.MetricsHandler(stubWriter{body: "canchalibre_http_requests_total 1\n"}, testMetricsToken)
	router, _ := platformhttp.NewRouter(platformhttp.RouterOptions{Logger: discardLogger(), Recorder: recorder, Metrics: handler})

	req := httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil)
	req.Header.Set("Authorization", "Bearer "+testMetricsToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on the mounted scrape path, got %d", rec.Code)
	}
	if seen := recorder.requests(); len(seen) != 0 {
		t.Fatalf("the scrape path was recorded: %+v", seen)
	}
}

// TestMetricsPathIsUnroutedWithoutAHandler is the S3 rule at the router seam: with
// metrics disabled the path does not exist, it is not merely forbidden.
func TestMetricsPathIsUnroutedWithoutAHandler(t *testing.T) {
	t.Parallel()

	router, _ := platformhttp.NewRouter(platformhttp.RouterOptions{Logger: discardLogger()})

	req := httptest.NewRequest(http.MethodGet, platformhttp.MetricsPath, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when metrics are disabled, got %d", rec.Code)
	}
	var problem platformhttp.ProblemDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("unmarshal problem: %v", err)
	}
	if problem.Type != platformhttp.TypeNotFound {
		t.Fatalf("expected type %q, got %q", platformhttp.TypeNotFound, problem.Type)
	}
}
