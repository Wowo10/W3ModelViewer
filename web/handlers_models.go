package web

import "net/http"

// handleModels serves GET /api/models: every .mdx root entry (sorted by
// path, built once in New) with its geoset count once scanned, the scan's
// progress and the 25 standard players' team texture references.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	resp := modelListResponse{
		Models: make([]modelListEntry, len(s.models)),
		Team:   s.team,
		Scan:   s.scanProgress(),
	}
	geosets := s.geosetCounts()
	for i, entry := range s.models {
		resp.Models[i] = modelListEntryFor(entry)
		resp.Models[i].Geosets = geosets[i]
	}
	writeJSON(w, resp)
}

// handleScan serves GET /api/scan: the scan's progress and every model's
// geoset count (null until scanned), parallel to /api/models.
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	// progress first: counts read after it can only be newer
	resp := scanResponse{scanProgress: s.scanProgress()}
	resp.Geosets = s.geosetCounts()
	writeJSON(w, resp)
}

// scanProgress reads the catalog's progress.
func (s *Server) scanProgress() scanProgress {
	done, total, finished := s.catalog.Progress()
	return scanProgress{Done: done, Total: total, Finished: finished}
}

// geosetCounts returns each model's geoset count from the catalog, nil
// where it isn't known yet; parallel to s.models.
func (s *Server) geosetCounts() []*int {
	counts := make([]*int, len(s.models))
	for i, entry := range s.models {
		if info, ok := s.catalog.Lookup(entry.CKey); ok {
			n := info.Geosets
			counts[i] = &n
		}
	}
	return counts
}
