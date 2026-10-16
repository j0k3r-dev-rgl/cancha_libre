package http

import (
	"crypto/subtle"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// MetricsPath is the private scrape path. It is mounted on the raw router rather
// than registered with huma, so it is served without ever entering the published
// OpenAPI contract.
const MetricsPath = "/metrics"

// metricsContentType is the Prometheus text exposition content type, version
// 0.0.4, as the scraper's parser expects it.
const metricsContentType = "text/plain; version=0.0.4; charset=utf-8"

// Label values used when the router could not attribute a request to an
// operation. They are fixed so that an unmatched path or an invented method
// cannot multiply the series.
const (
	routeUnmatched = "unmatched"
	methodOther    = "other"
)

// RequestRecorder receives one observation per served request. The route is the
// router's template (for example /api/v1/admin/support-operators/{accountId}) and
// never the requested path, so its value set stays bounded.
type RequestRecorder interface {
	RecordHTTPRequest(method, route string, status int, seconds float64)
}

// MetricsWriter renders the current metric values in the Prometheus text
// exposition format.
type MetricsWriter interface {
	WritePrometheus(w io.Writer)
}

// RecordRequests reports every served request to recorder with bounded labels:
// the matched route template, a normalized method and the answered status. The
// scrape path itself is excluded, because monitoring traffic is not product
// traffic and would otherwise dominate the request series.
func RecordRequests(recorder RequestRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if recorder == nil {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(wrapped, r)

			route := routeTemplate(r)
			if route == MetricsPath {
				return
			}
			recorder.RecordHTTPRequest(normalizeMethod(r.Method), route, responseStatus(wrapped), time.Since(start).Seconds())
		})
	}
}

// MetricsHandler answers the private scrape path. The configured token is
// mandatory: without a matching Authorization bearer header the request is
// refused with the catalog's 401 problem document and no metrics are rendered.
// An unconfigured token refuses everything, so the handler fails closed even if a
// future caller forgets the "no token means no route" rule.
func MetricsHandler(source MetricsWriter, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasBearerToken(r, token) {
			WriteProblem(w, http.StatusUnauthorized, TypeUnauthenticated,
				"The metrics endpoint requires a valid bearer token.")
			return
		}

		w.Header().Set("Content-Type", metricsContentType)
		source.WritePrometheus(w)
	})
}

// hasBearerToken reports whether the request carries exactly the configured token
// under the bearer scheme. The comparison is constant time, and the scheme is
// matched case-insensitively as RFC 6750 defines it.
func hasBearerToken(r *http.Request, token string) bool {
	if token == "" {
		return false
	}

	scheme, presented, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}

// routeTemplate returns the route the router matched, or the fixed value for a
// request it never attributed. It must be read after the handler ran, while the
// routing context of this request is still alive.
func routeTemplate(r *http.Request) string {
	if pattern := chi.RouteContext(r.Context()).RoutePattern(); pattern != "" {
		return pattern
	}
	return routeUnmatched
}

// responseStatus returns the answered status, defaulting to 200 the way net/http
// does when a handler writes a body without ever calling WriteHeader.
func responseStatus(w chimiddleware.WrapResponseWriter) int {
	if status := w.Status(); status != 0 {
		return status
	}
	return http.StatusOK
}

// normalizeMethod keeps the method label inside a closed set: an arbitrary or
// misspelled verb is reported as one value instead of creating a series per
// spelling.
func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return methodOther
	}
}
