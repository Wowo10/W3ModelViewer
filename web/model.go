package web

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"net/http"
	"net/url"

	"modelView/extract/root"
	"modelView/model/mdx"
	"modelView/model/texture"
)

// loaded is a model's parsed content, keyed by content key: everything
// needed to answer /api/model, /api/mesh and /api/pose without re-reading
// CASC or re-parsing MDX. Its fields depend only on content (not on which
// root path/edition the caller asked for), so it's shared across any
// paths with the same content key.
type loaded struct {
	version     uint32
	chunks      int
	geosets     []mdx.Geoset
	layers      [][]texture.Layer // one slice per geoset, built from this entry's mod chain
	priority    []int32           // one per geoset, its material's priority plane
	skeleton    mdx.Skeleton
	objects     int // len(Pivots()), the pose's matrix count
	sequences   []mdx.Sequence
	geosetAnims []mdx.GeosetAnim
	globals     []uint32

	lod         uint32
	mesh        []byte
	meshOffsets []*meshOffsets // one per geoset, nil for geosets not at lod
}

// load reads and parses the model at entry, building everything load's
// callers need. Panics from mdx.Read/Geosets etc. (e.g. the 2 models with
// known parsing bugs) propagate; net/http recovers them per request.
func (s *Server) load(idx int) *loaded {
	entry := s.models[idx]
	data := s.read(entry.CKey)
	m := mdx.Read(data)
	geosets := m.Geosets()

	l := &loaded{version: m.Version, chunks: len(m.Chunks), geosets: geosets}

	if len(geosets) > 0 {
		materials := m.Materials()
		textures := m.Textures()
		l.layers = make([][]texture.Layer, len(geosets))
		l.priority = make([]int32, len(geosets))
		for i, g := range geosets {
			if g.MaterialID < uint32(len(materials)) {
				l.priority[i] = materials[g.MaterialID].PriorityPlane
			}
			l.layers[i] = s.resolver.GeosetLayers(entry, materials, textures, g)
		}

		l.skeleton = mdx.NewSkeleton(m)
		l.objects = len(m.Pivots())
		l.sequences = m.Sequences()
		l.geosetAnims = m.GeosetAnims()
		l.globals = m.GlobalSequences()
	}

	l.lod = mdx.LowestLOD(geosets)
	l.mesh, l.meshOffsets = buildMesh(geosets, l.lod)

	return l
}

// getLoaded returns the cached parsed model for entry's content key,
// parsing and caching it on a miss.
func (s *Server) getLoaded(idx int) *loaded {
	entry := s.models[idx]
	if m, ok := s.cache.get(entry.CKey); ok {
		return m
	}
	m := s.load(idx)
	s.cache.put(entry.CKey, m, int64(len(m.mesh)), s.CacheBound)
	return m
}

// buildMesh lays out every geoset at lod (visible or not) into one buffer,
// in geoset order: positions, normals (zero-filled when the geoset has
// none), each UV set, weights, node ids (Influences()), then indices
// padded to 4 bytes. It returns the buffer and, per geoset, its offsets
// into it (nil for a geoset not at lod).
func buildMesh(geosets []mdx.Geoset, lod uint32) ([]byte, []*meshOffsets) {
	var buf []byte
	offsets := make([]*meshOffsets, len(geosets))

	for i, g := range geosets {
		if g.LOD != lod {
			continue
		}

		v := g.VertexCount()
		off := &meshOffsets{}

		off.Positions = len(buf)
		buf = appendFloat32s(buf, g.Positions)

		off.Normals = len(buf)
		if len(g.Normals) == 3*v {
			buf = appendFloat32s(buf, g.Normals)
		} else {
			buf = append(buf, make([]byte, 12*v)...)
		}

		off.UVs = make([]int, len(g.UVs))
		for u, uv := range g.UVs {
			off.UVs[u] = len(buf)
			buf = appendFloat32s(buf, uv)
		}

		influences := g.Influences()

		off.Weights = len(buf)
		buf = appendFloat32s(buf, influences.Weights)

		off.Nodes = len(buf)
		buf = appendUint32s(buf, influences.Nodes)

		off.Indices = len(buf)
		buf = appendUint16sPadded(buf, g.Indices)

		offsets[i] = off
	}

	return buf, offsets
}

func appendFloat32s(buf []byte, vals []float32) []byte {
	out := make([]byte, 4*len(vals))
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(v))
	}
	return append(buf, out...)
}

func appendUint32s(buf []byte, vals []uint32) []byte {
	out := make([]byte, 4*len(vals))
	for i, v := range vals {
		binary.LittleEndian.PutUint32(out[4*i:], v)
	}
	return append(buf, out...)
}

// appendUint16sPadded writes vals as little-endian uint16s, padded with a
// trailing zero uint16 when len(vals) is odd, so the block is 4-byte
// aligned.
func appendUint16sPadded(buf []byte, vals []uint16) []byte {
	n := len(vals)
	size := 2 * n
	if n%2 != 0 {
		size += 2
	}
	out := make([]byte, size)
	for i, v := range vals {
		binary.LittleEndian.PutUint16(out[2*i:], v)
	}
	return append(buf, out...)
}

// handleModel serves GET /api/model?path=.
func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	idx, ok := s.lookupModel(w, r)
	if !ok {
		return
	}
	entry := s.models[idx]
	m := s.getLoaded(idx)

	defSeq := mdx.DefaultSequenceIndex(m.sequences)
	ok2 := defSeq != -1
	var at mdx.AnimTime
	if ok2 {
		seq := m.sequences[defSeq]
		at = mdx.AnimTime{Sequence: seq, Frame: seq.Start, Global: 0}
	}
	alphas := mdx.GeosetAlphasAtTime(len(m.geosets), m.geosetAnims, m.globals, at, ok2)

	resp := modelResponse{
		Path:            entry.Path,
		Name:            entry.Name,
		Edition:         string(entry.Variant.Edition),
		Overlays:        overlaysOrEmpty(entry.Variant.Overlays),
		CKey:            hex.EncodeToString(entry.CKey[:]),
		Version:         m.version,
		Chunks:          m.chunks,
		Objects:         m.objects,
		LOD:             m.lod,
		Geosets:         make([]geosetRef, len(m.geosets)),
		Sequences:       make([]sequenceRef, len(m.sequences)),
		DefaultSequence: defSeq,
		GlobalSequences: m.globals,
		Mesh:            meshRef{URL: "/api/mesh?path=" + url.QueryEscape(entry.Path), Size: len(m.mesh)},
	}

	for i, g := range m.geosets {
		layers := make([]layerRef, len(m.layers[i]))
		for j, l := range m.layers[i] {
			layers[j] = s.layerRefFor(l)
		}

		var mesh *meshOffsets
		if m.meshOffsets[i] != nil {
			mesh = m.meshOffsets[i]
		}

		resp.Geosets[i] = geosetRef{
			Material:  g.MaterialID,
			LOD:       g.LOD,
			Vertices:  g.VertexCount(),
			Triangles: g.TriangleCount(),
			Priority:  m.priority[i],
			Alpha:     alphas[i],
			Layers:    layers,
			Mesh:      mesh,
		}
		resp.Vertices += g.VertexCount()
		resp.Triangles += g.TriangleCount()
	}

	for i, sq := range m.sequences {
		resp.Sequences[i] = sequenceRefFor(sq)
	}

	variants := s.root.Variants(entry.Name)
	root.SortVariants(variants)
	resp.Editions = make([]editionRef, len(variants))
	for i, v := range variants {
		resp.Editions[i] = editionRefFor(v)
	}

	writeJSON(w, resp)
}

// handleMesh serves GET /api/mesh?path=: the raw mesh buffer built by
// buildMesh (cached alongside the rest of the parsed model).
func (s *Server) handleMesh(w http.ResponseWriter, r *http.Request) {
	idx, ok := s.lookupModel(w, r)
	if !ok {
		return
	}
	m := s.getLoaded(idx)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(m.mesh)
}
