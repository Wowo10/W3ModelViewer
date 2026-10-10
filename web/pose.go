package web

import (
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"modelView/model/mdx"
)

// identityMat4 is the bind-pose matrix used for every object id when a
// model has no sequences (seq == -1).
var identityMat4 = mdx.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

// handlePose serves GET /api/pose?path=&seq=&frame=&global=: the cached
// model's world matrices and geoset alphas at (seq, frame, global).
func (s *Server) handlePose(w http.ResponseWriter, r *http.Request) {
	idx, ok := s.lookupModel(w, r)
	if !ok {
		return
	}
	m := s.getLoaded(idx)

	q := r.URL.Query()
	seq, err := strconv.Atoi(q.Get("seq"))
	if err != nil {
		badRequest(w, "seq must be an integer")
		return
	}
	if seq < -1 || seq >= len(m.sequences) {
		badRequest(w, fmt.Sprintf("seq %d out of range [-1, %d)", seq, len(m.sequences)))
		return
	}
	frame, err := strconv.ParseUint(q.Get("frame"), 10, 32)
	if err != nil {
		badRequest(w, "frame must be a non-negative integer")
		return
	}
	global, err := strconv.ParseUint(q.Get("global"), 10, 32)
	if err != nil {
		badRequest(w, "global must be a non-negative integer")
		return
	}

	hasSeq := seq != -1
	var at mdx.AnimTime
	var matrices []mdx.Mat4
	if hasSeq {
		at = mdx.AnimTime{Sequence: m.sequences[seq], Frame: uint32(frame), Global: uint32(global)}
		matrices = m.skeleton.WorldMatrices(at)
	} else {
		matrices = make([]mdx.Mat4, m.objects)
		for i := range matrices {
			matrices[i] = identityMat4
		}
	}
	alphas := mdx.GeosetAlphasAtTime(len(m.geosets), m.geosetAnims, m.globals, at, hasSeq)

	buf := make([]byte, 8+16*4*len(matrices)+4*len(alphas))
	binary.LittleEndian.PutUint32(buf[0:], uint32(len(matrices)))
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(alphas)))

	pos := 8
	for _, mat := range matrices {
		for _, f := range mat {
			binary.LittleEndian.PutUint32(buf[pos:], math.Float32bits(f))
			pos += 4
		}
	}
	for _, a := range alphas {
		binary.LittleEndian.PutUint32(buf[pos:], math.Float32bits(a))
		pos += 4
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(buf)
}
