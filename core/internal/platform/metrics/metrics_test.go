package metrics_test

import (
	"strings"
	"sync"
	"testing"

	"canchalibre/core/internal/platform/metrics"
)

// exposition renders the registry to text the way the endpoint does.
func exposition(t *testing.T, registry *metrics.Registry) string {
	t.Helper()

	var sb strings.Builder
	registry.WritePrometheus(&sb)
	return sb.String()
}

// mustHaveLine asserts the exposition carries one exact sample line. Comparing
// whole lines rather than substrings keeps a metadata or comment line from
// satisfying an assertion about a measurement.
func mustHaveLine(t *testing.T, out, want string) {
	t.Helper()

	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return
		}
	}
	t.Fatalf("exposition does not contain the line %q\n--- exposition ---\n%s", want, out)
}

func mustNotHaveLine(t *testing.T, out, unwanted string) {
	t.Helper()

	for _, line := range strings.Split(out, "\n") {
		if line == unwanted {
			t.Fatalf("exposition contains the unwanted line %q", unwanted)
		}
	}
}

// TestRecordHTTPRequestExposesTheCounterAndTheDurationBuckets pins the two HTTP
// instruments and the histogram shape a Grafana latency panel reads: a real
// cumulative `le` series, `_count` and the `+Inf` bucket.
func TestRecordHTTPRequestExposesTheCounterAndTheDurationBuckets(t *testing.T) {
	registry := metrics.New()
	registry.RecordHTTPRequest("GET", "/api/v1/me", 200, 0.1)

	out := exposition(t, registry)

	mustHaveLine(t, out, `canchalibre_http_requests_total{method="GET",route="/api/v1/me",status="200"} 1`)
	mustHaveLine(t, out, `canchalibre_http_request_duration_seconds_bucket{method="GET",route="/api/v1/me",le="0.05"} 0`)
	mustHaveLine(t, out, `canchalibre_http_request_duration_seconds_bucket{method="GET",route="/api/v1/me",le="0.1"} 1`)
	mustHaveLine(t, out, `canchalibre_http_request_duration_seconds_bucket{method="GET",route="/api/v1/me",le="+Inf"} 1`)
	mustHaveLine(t, out, `canchalibre_http_request_duration_seconds_count{method="GET",route="/api/v1/me"} 1`)
}

// TestRecordHTTPRequestKeepsLabelCombinationsApart proves the status and the
// route template really are labels: two statuses on one route are two counters,
// never one counter silently merged.
func TestRecordHTTPRequestKeepsLabelCombinationsApart(t *testing.T) {
	registry := metrics.New()
	registry.RecordHTTPRequest("GET", "/api/v1/me", 200, 0.01)
	registry.RecordHTTPRequest("GET", "/api/v1/me", 401, 0.01)
	registry.RecordHTTPRequest("GET", "/api/v1/me", 401, 0.01)

	out := exposition(t, registry)

	mustHaveLine(t, out, `canchalibre_http_requests_total{method="GET",route="/api/v1/me",status="200"} 1`)
	mustHaveLine(t, out, `canchalibre_http_requests_total{method="GET",route="/api/v1/me",status="401"} 2`)
}

// TestRecordHTTPRequestIsConcurrencySafe is the race-detector case: every
// recorded request must be counted exactly once.
func TestRecordHTTPRequestIsConcurrencySafe(t *testing.T) {
	const goroutines = 8
	const perGoroutine = 50

	registry := metrics.New()

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				registry.RecordHTTPRequest("POST", "/api/v1/auth/login", 200, 0.02)
			}
		}()
	}
	wg.Wait()

	out := exposition(t, registry)

	mustHaveLine(t, out, `canchalibre_http_requests_total{method="POST",route="/api/v1/auth/login",status="200"} 400`)
	mustHaveLine(t, out, `canchalibre_http_request_duration_seconds_count{method="POST",route="/api/v1/auth/login"} 400`)
}

// TestPoolGaugesAreReadOnEveryScrape is what makes the pool gauges useful: they
// must report the pool as it is at scrape time, not as it was when the process
// wired them up.
func TestPoolGaugesAreReadOnEveryScrape(t *testing.T) {
	registry := metrics.New()

	state := metrics.PoolSnapshot{Total: 3, Idle: 1, Acquired: 2, Max: 10}
	registry.RegisterPoolGauges(func() metrics.PoolSnapshot { return state })

	first := exposition(t, registry)
	mustHaveLine(t, first, `canchalibre_db_pool_conns{state="total"} 3`)
	mustHaveLine(t, first, `canchalibre_db_pool_conns{state="idle"} 1`)
	mustHaveLine(t, first, `canchalibre_db_pool_conns{state="acquired"} 2`)
	mustHaveLine(t, first, `canchalibre_db_pool_max_conns 10`)

	state = metrics.PoolSnapshot{Total: 5, Idle: 0, Acquired: 5, Max: 10}

	second := exposition(t, registry)
	mustHaveLine(t, second, `canchalibre_db_pool_conns{state="total"} 5`)
	mustHaveLine(t, second, `canchalibre_db_pool_conns{state="acquired"} 5`)
	mustNotHaveLine(t, second, `canchalibre_db_pool_conns{state="total"} 3`)
}

// TestWritePrometheusNamesTheProcessBudget documents that the exposition also carries the
// Go runtime and process numbers the 2 vCPU / memory budget is read from.
func TestWritePrometheusNamesTheProcessBudget(t *testing.T) {
	out := exposition(t, metrics.New())

	if !strings.Contains(out, "go_goroutines") {
		t.Fatalf("exposition does not carry go_goroutines\n--- exposition ---\n%s", out)
	}
	if !strings.Contains(out, "process_resident_memory_bytes") {
		t.Fatalf("exposition does not carry process_resident_memory_bytes\n--- exposition ---\n%s", out)
	}
}
