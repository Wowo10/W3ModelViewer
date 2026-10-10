package web

import (
	"encoding/hex"
	"net/http"
)

// handleTexture serves GET /api/texture?ckey=: the decoded .dds bytes of
// the root entry with that content key, which must end in .dds. The
// response is cacheable forever since content keys are content-addressed.
func (s *Server) handleTexture(w http.ResponseWriter, r *http.Request) {
	hexKey := r.URL.Query().Get("ckey")
	if len(hexKey) != 32 {
		badRequest(w, "ckey must be 32 hex characters")
		return
	}
	decoded, err := hex.DecodeString(hexKey)
	if err != nil {
		badRequest(w, "ckey must be 32 hex characters")
		return
	}
	var ckey [16]byte
	copy(ckey[:], decoded)

	if !s.ddsSet[ckey] {
		notFound(w, "unknown .dds content key")
		return
	}
	if !s.stored(ckey) {
		notFound(w, "content not stored locally")
		return
	}

	data := s.read(ckey)
	w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}
