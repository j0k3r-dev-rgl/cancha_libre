package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	platformhttp "canchalibre/core/internal/platform/http"
)

// discardLogger returns a logger whose output is thrown away, for the tests that
// build a router only to assert its responses.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRecoverPanicAnswersTheDocumentedProblemAndLogsTheRequestID pins the rule
// this middleware exists for: a panic must not take the process down, must answer
// the RFC 7807 problem document the contract already declares for 500 instead of
// a bare body-less 500, and must leave an ERROR line that carries the request id,
// so the failing request can be correlated with its access-log line.
func TestRecoverPanicAnswersTheDocumentedProblemAndLogsTheRequestID(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	handler := platformhttp.RecoverPanic(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom from the handler")
	}))

	// Chi's RequestID middleware is what puts the identifier in the context. It
	// reuses the incoming X-Request-Id header, so the value asserted below is the
	// one this test chose rather than a generated one.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("X-Request-Id", "panic-request-id")
	rec := httptest.NewRecorder()
	chimiddleware.RequestID(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status after a panic = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Fatalf("panic response Content-Type = %q, want application/problem+json", ct)
	}

	var problem struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("panic response body is not a problem document: %v (%q)", err, rec.Body.String())
	}
	if problem.Type != platformhttp.TypeInternal {
		t.Fatalf("panic response type = %q, want %q", problem.Type, platformhttp.TypeInternal)
	}

	line := logs.String()
	for _, want := range []string{"recovered panic", "boom from the handler", "request_id", "stack", "panic-request-id"} {
		if !strings.Contains(line, want) {
			t.Fatalf("panic log line %q does not mention %q", line, want)
		}
	}
}

// TestRecoverPanicPassesANormalResponseThroughUnchanged proves the middleware
// wraps the response writer without altering a successful response, and that it
// logs nothing when nothing panicked.
func TestRecoverPanicPassesANormalResponseThroughUnchanged(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	const body = `{"ok":true}`
	handler := platformhttp.RecoverPanic(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != body {
		t.Fatalf("body = %q, want %q", rec.Body.String(), body)
	}
	if logs.Len() != 0 {
		t.Fatalf("a successful request logged %q, want nothing", logs.String())
	}
}

// TestLogTimeoutsRecordsAnExpiredDeadline pins the observability half of the
// timeout path: the 504 answer stays Chi's transport response, but the request
// that exceeded the deadline must leave an ERROR line naming its request id,
// method and path, because a timeout is otherwise invisible in the logs.
func TestLogTimeoutsRecordsAnExpiredDeadline(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	handler := platformhttp.LogTimeouts(logger, time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The middleware must not change the response the handler produced.
		if got := platformhttp.RequestID(r.Context()); got != "timeout-request-id" {
			t.Fatalf("RequestID(ctx) = %q, want the id Chi stored", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	ctx = context.WithValue(ctx, chimiddleware.RequestIDKey, "timeout-request-id")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil).WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want the handler's %d: the logger must not rewrite the response", rec.Code, http.StatusNoContent)
	}
	line := logs.String()
	for _, want := range []string{"exceeded the server timeout", "timeout-request-id", "/api/v1/me"} {
		if !strings.Contains(line, want) {
			t.Fatalf("timeout log line %q does not mention %q", line, want)
		}
	}
}
