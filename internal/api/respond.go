package api

import (
	"encoding/json"
	"net/http"
)

// errorBody is the single error response shape used across every API
// surface: {"error": ["message", ...]}. Unrouted 404/405s are converted to
// this shape too, by withJSONErrors.
type errorBody struct {
	Error []string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, messages ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: messages})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
