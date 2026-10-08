package http

import (
	"encoding/json"
	"net/http"
)

// ProblemDetails represents an RFC 7807 Problem Details response.
type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance,omitempty"`
}

// WriteJSON serializes the given value to JSON and writes it with status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteProblemDetails writes an RFC 7807 Problem Details payload.
func WriteProblemDetails(w http.ResponseWriter, status int, title, detail, instance string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ProblemDetails{
		Type:     "about:blank",
		Title:    title,
		Status:   status,
		Detail:   detail,
		Instance: instance,
	})
}
