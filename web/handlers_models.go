package web

import "net/http"

// handleModels serves GET /api/models: every .mdx root entry (sorted by
// path, built once in New) and the 25 standard players' team texture
// references.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	resp := modelListResponse{
		Models: make([]modelListEntry, len(s.models)),
		Team:   s.team,
	}
	for i, entry := range s.models {
		resp.Models[i] = modelListEntryFor(entry)
	}
	writeJSON(w, resp)
}
