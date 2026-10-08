package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// NewRouter constructs a Chi router configured with standard middlewares.
func NewRouter() *chi.Mux {
	r := chi.NewRouter()

	// Standard production middlewares
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	// Base health endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// API v1 group
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			WriteJSON(w, http.StatusOK, map[string]string{"message": "pong"})
		})
	})

	return r
}
