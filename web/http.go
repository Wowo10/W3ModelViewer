package web

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v as a JSON response body with the right content type.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

// badRequest writes a 400 with msg as a plain-text body.
func badRequest(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusBadRequest)
}

// notFound writes a 404 with msg as a plain-text body.
func notFound(w http.ResponseWriter, msg string) {
	http.Error(w, msg, http.StatusNotFound)
}

// lookupModel resolves a required "path" query parameter to its root
// entry, writing a 400 when it's missing, a 404 when it doesn't name a
// known .mdx model, and a 404 when its content isn't stored locally. It
// returns ok=false when it already wrote a response.
func (s *Server) lookupModel(w http.ResponseWriter, r *http.Request) (entry int, ok bool) {
	path := r.URL.Query().Get("path")
	if path == "" {
		badRequest(w, "missing path parameter")
		return 0, false
	}
	idx, exists := s.byPath[path]
	if !exists {
		notFound(w, "unknown model path")
		return 0, false
	}
	if !s.stored(s.models[idx].CKey) {
		notFound(w, "content not stored locally")
		return 0, false
	}
	return idx, true
}
