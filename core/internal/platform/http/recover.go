package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RequestID returns the identifier Chi's RequestID middleware stored in the request
// context, or an empty string when the request never passed through it. It is what
// lets an error line in the structured log be correlated with the access-log line
// of the same request.
func RequestID(ctx context.Context) string {
	return chimiddleware.GetReqID(ctx)
}

// RecoverPanic replaces Chi's Recoverer. Chi's version keeps the process alive and
// writes a body-less 500; this one keeps the process alive, records the panic with
// its stack at ERROR level together with the request id, and answers the documented
// 500 problem document instead: `500` with `urn:canchalibre:error:internal` is
// declared by every operation, so it answers what the contract already promises.
// When the handler had already committed a response, nothing is written over it
// and the panic is only logged.
func RecoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				rvr := recover()
				if rvr == nil {
					return
				}
				if rvr == http.ErrAbortHandler {
					// The client went away. Not ours to report, and not a panic the
					// process should swallow.
					panic(rvr)
				}
				logger.ErrorContext(r.Context(), "recovered panic while serving a request",
					"error", fmt.Sprint(rvr),
					"request_id", RequestID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				if ww.Status() == 0 {
					WriteProblem(ww, http.StatusInternalServerError, TypeInternal, "Unexpected internal server error.")
				}
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

// LogTimeouts records a request whose context deadline expired, because a timeout
// is otherwise invisible: the caller sees a 504 and the logs say nothing. The 504
// answer stays Chi's transport-level response. The catalog maps a status to exactly
// one type and has no `504` slug, so publishing it as a problem document would mean
// declaring a new status on every operation, and a transport timeout is not a
// refusal an operation performs.
func LogTimeouts(logger *slog.Logger, timeout time.Duration) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				logger.ErrorContext(r.Context(), "request exceeded the server timeout",
					"request_id", RequestID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"timeout", timeout.String(),
				)
			}
		})
	}
}
