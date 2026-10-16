// Package metrics owns the instruments the private /metrics endpoint publishes:
// the HTTP request counter and duration histogram, the PostgreSQL pool gauges,
// and the Go runtime and process metrics of the running binary.
//
// Every label value it accepts is bounded by construction: the route is the
// router's template rather than a raw path, the method is a normalized verb, and
// the status is an HTTP code. No identifier, path segment or email can reach a
// metric name through this package.
package metrics

import (
	"fmt"
	"io"

	vm "github.com/VictoriaMetrics/metrics"
)

// Metric names published by the endpoint. They are constants because the order
// of the labels inside a name is part of the series identity: building the same
// name in two places with a different order would publish two series for one
// measurement.
const (
	metricHTTPRequestsTotal   = "canchalibre_http_requests_total"
	metricHTTPRequestDuration = "canchalibre_http_request_duration_seconds"
	metricDBPoolConns         = "canchalibre_db_pool_conns"
	metricDBPoolMaxConns      = "canchalibre_db_pool_max_conns"
)

// durationBuckets are the upper bounds, in seconds, of the request duration
// histogram. They are classic Prometheus buckets rather than VictoriaMetrics
// vmrange buckets so the same exposition is usable by Prometheus, Grafana and
// VictoriaMetrics, wherever the operator ends up running the scraper.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// PoolSnapshot is one reading of the database connection pool.
type PoolSnapshot struct {
	Total    int32
	Idle     int32
	Acquired int32
	Max      int32
}

// Registry owns the instruments of one process. It keeps its own metric set
// instead of the library's global one, so the whole exposition is one explicit
// object a test can render without touching process-global state.
type Registry struct {
	set *vm.Set
}

// New creates an empty registry.
func New() *Registry {
	return &Registry{set: vm.NewSet()}
}

// RecordHTTPRequest counts one served request and records its duration. route
// must be the router's template and status the answered status code; a raw path
// or an identifier here would make the number of series unbounded.
func (r *Registry) RecordHTTPRequest(method, route string, status int, seconds float64) {
	r.set.GetOrCreateCounter(fmt.Sprintf(`%s{method=%q,route=%q,status="%d"}`,
		metricHTTPRequestsTotal, method, route, status)).Inc()

	r.set.GetOrCreatePrometheusHistogramExt(fmt.Sprintf(`%s{method=%q,route=%q}`,
		metricHTTPRequestDuration, method, route), durationBuckets).Update(seconds)
}

// RegisterPoolGauges publishes the database pool gauges. snapshot is called on
// every scrape, so the gauges report the pool as it is when it is read instead of
// the value it had when the process wired them up.
func (r *Registry) RegisterPoolGauges(snapshot func() PoolSnapshot) {
	if snapshot == nil {
		return
	}

	r.set.GetOrCreateGauge(fmt.Sprintf(`%s{state="total"}`, metricDBPoolConns),
		func() float64 { return float64(snapshot().Total) })
	r.set.GetOrCreateGauge(fmt.Sprintf(`%s{state="idle"}`, metricDBPoolConns),
		func() float64 { return float64(snapshot().Idle) })
	r.set.GetOrCreateGauge(fmt.Sprintf(`%s{state="acquired"}`, metricDBPoolConns),
		func() float64 { return float64(snapshot().Acquired) })
	r.set.GetOrCreateGauge(metricDBPoolMaxConns,
		func() float64 { return float64(snapshot().Max) })
}

// WritePrometheus renders the registry followed by the Go runtime and process metrics
// the Prometheus text exposition format.
func (r *Registry) WritePrometheus(w io.Writer) {
	r.set.WritePrometheus(w)
	vm.WriteProcessMetrics(w)
}
