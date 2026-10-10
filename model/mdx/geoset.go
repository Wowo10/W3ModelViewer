package mdx

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Geoset is one geoset of a GEOS chunk. See the GEOS layout comment in
// Geosets for the fields skipped here.
type Geoset struct {
	Positions  []float32   // x, y, z per vertex
	Normals    []float32   // x, y, z per vertex; empty in a few classic geosets
	UVs        [][]float32 // one slice per UV set: u, v per vertex
	Indices    []uint16    // triangle list, 3 per triangle
	MaterialID uint32
	LOD        uint32

	VertexGroups []uint8  // GNDX: matrix group per vertex; empty when Skin is set
	GroupSizes   []uint32 // MTGC: node count per matrix group
	GroupNodes   []uint32 // MATS: node object ids, GroupSizes[i] per group in order
	Skin         []uint16 // SKIN: per vertex 4 indices into GroupNodes, then 4 weights (sum 255); nil when absent
}

// LowestLOD returns the lowest LOD value present among geosets, or 0 if
// there are none.
func LowestLOD(geosets []Geoset) uint32 {
	if len(geosets) == 0 {
		return 0
	}
	lod := geosets[0].LOD
	for _, g := range geosets[1:] {
		if g.LOD < lod {
			lod = g.LOD
		}
	}
	return lod
}

// VertexCount returns the number of vertices in the geoset.
func (g Geoset) VertexCount() int { return len(g.Positions) / 3 }

// TriangleCount returns the number of triangles in the geoset.
func (g Geoset) TriangleCount() int { return len(g.Indices) / 3 }

// Geosets parses the model's GEOS chunk, if any, into geosets. It returns
// nil when there is no GEOS chunk.
//
// GEOS layout in version 1800 (the only version with a GEOS chunk seen in
// wardata): geosets packed back to back, each:
//  1. uint32 inclusive size (counts itself);
//  2. VRTX uint32 n, float32[3n]; NRMS n, float32[3n]; PTYP n, uint32[n];
//     PCNT n, uint32[n]; PVTX n, uint16[n]; GNDX n, uint8[n]; MTGC n,
//     uint32[n]; MATS n, uint32[n];
//  3. uint32 material id, uint32 selection group, uint32 selection flags,
//     uint32 LOD, char[80] LOD name (NUL-padded);
//  4. extent (float32 radius + float32[3] min + float32[3] max = 28 B),
//     uint32 n sequence extents of 28 B each;
//  5. optional TANG n, float32[4n]; then optional SKIN n, uint16[n] — the
//     count is in 2-byte units (n = 8 x vertices: per vertex, 4 uint16
//     indices into MATS then 4 uint16 weights, summing to 255);
//  6. UVAS uint32 sets, then per set UVBS n, float32[2n].
//
// Selection group/flags, LOD name, extents and TANG are skipped (by size);
// PTYP and PCNT are only checked (one group of type 4, summing to the PVTX
// count); GNDX, MTGC, MATS and SKIN are kept (see VertexGroups, GroupSizes,
// GroupNodes and Skin); the rest is kept.
//
// Geosets panics (mdx: GEOS geoset %d: ...) on truncated data, a geoset
// running past the chunk, an unexpected subchunk tag, a face type other
// than 4, or bytes left over at the end of a geoset. It panics with "mdx:
// GEOS parsing supports version 1800 only, got %d" on a GEOS chunk in any
// other version.
func (m Model) Geosets() []Geoset {
	var geosChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "GEOS" {
			geosChunk = &m.Chunks[i]
			break
		}
	}
	if geosChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: GEOS parsing supports version 1800 only, got %d", m.Version))
	}

	data := geosChunk.Data
	var geosets []Geoset
	pos := 0
	for pos < len(data) {
		g, n := readGeoset(data, pos, len(geosets))
		geosets = append(geosets, g)
		pos += n
	}

	return geosets
}

// readGeoset parses one geoset starting at pos in data and returns it along
// with its size in bytes (the inclusive size field).
func readGeoset(data []byte, start int, index int) (Geoset, int) {
	r := geosReader{data: data, pos: start, limit: len(data), geoset: index}

	size := r.u32()
	end := start + int(size)
	if uint64(end) > uint64(len(data)) {
		panic(fmt.Sprintf("mdx: GEOS geoset %d: runs past chunk end", index))
	}
	r.limit = end

	var g Geoset

	n := r.subchunk("VRTX")
	g.Positions = r.float32s(3 * n)

	n = r.subchunk("NRMS")
	g.Normals = r.float32s(3 * n)

	n = r.subchunk("PTYP")
	faceTypes := r.uint32s(n)
	for _, t := range faceTypes {
		if t != 4 {
			panic(fmt.Sprintf("mdx: GEOS geoset %d: face type %d, want 4", index, t))
		}
	}

	n = r.subchunk("PCNT")
	faceCounts := r.uint32s(n)
	indexCount := uint64(0)
	for _, c := range faceCounts {
		indexCount += uint64(c)
	}

	n = r.subchunk("PVTX")
	if uint64(n) != indexCount {
		panic(fmt.Sprintf("mdx: GEOS geoset %d: PVTX count %d != PCNT total %d", index, n, indexCount))
	}
	g.Indices = r.uint16s(n)

	n = r.subchunk("GNDX")
	g.VertexGroups = r.uint8s(n)

	n = r.subchunk("MTGC")
	g.GroupSizes = r.uint32s(n)

	n = r.subchunk("MATS")
	g.GroupNodes = r.uint32s(n)

	g.MaterialID = r.u32()
	r.skip(4) // selection group
	r.skip(4) // selection flags
	g.LOD = r.u32()
	r.skip(80) // LOD name

	r.skip(28) // extent
	extents := r.u32()
	r.skip(int(extents) * 28)

	if r.pos < r.limit {
		tag := r.peekTag()
		if tag == "TANG" {
			n = r.subchunk("TANG")
			r.skip(4 * 4 * n) // float32[4n]
		}
	}
	if r.pos < r.limit {
		tag := r.peekTag()
		if tag == "SKIN" {
			n = r.subchunk("SKIN")
			g.Skin = r.uint16s(n)
		}
	}

	sets := r.subchunk("UVAS")
	g.UVs = make([][]float32, sets)
	for i := 0; i < sets; i++ {
		n = r.subchunk("UVBS")
		g.UVs[i] = r.float32s(2 * n)
	}

	if r.pos != r.limit {
		panic(fmt.Sprintf("mdx: GEOS geoset %d: %d bytes left over", index, r.limit-r.pos))
	}

	return g, int(size)
}

// geosReader is a bounds-checked cursor over one geoset's bytes.
type geosReader struct {
	data   []byte
	pos    int
	limit  int
	geoset int
}

func (r *geosReader) need(n int) {
	if n < 0 || uint64(r.pos)+uint64(n) > uint64(r.limit) {
		panic(fmt.Sprintf("mdx: GEOS geoset %d: truncated at offset %d", r.geoset, r.pos))
	}
}

func (r *geosReader) u32() uint32 {
	r.need(4)
	v := binary.LittleEndian.Uint32(r.data[r.pos : r.pos+4])
	r.pos += 4
	return v
}

// peekTag returns the next 4-byte tag without advancing.
func (r *geosReader) peekTag() string {
	r.need(4)
	return string(r.data[r.pos : r.pos+4])
}

// subchunk reads a 4-byte tag (must equal want), then a uint32 count, and
// returns the count.
func (r *geosReader) subchunk(want string) int {
	tag := r.peekTag()
	if tag != want {
		panic(fmt.Sprintf("mdx: GEOS geoset %d: want %s, got %q", r.geoset, want, tag))
	}
	r.pos += 4
	return int(r.u32())
}

func (r *geosReader) float32s(n int) []float32 {
	r.need(4 * n)
	out := make([]float32, n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(r.data[r.pos : r.pos+4]))
		r.pos += 4
	}
	return out
}

func (r *geosReader) uint32s(n int) []uint32 {
	r.need(4 * n)
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(r.data[r.pos : r.pos+4])
		r.pos += 4
	}
	return out
}

func (r *geosReader) uint8s(n int) []uint8 {
	r.need(n)
	out := make([]uint8, n)
	copy(out, r.data[r.pos:r.pos+n])
	r.pos += n
	return out
}

func (r *geosReader) uint16s(n int) []uint16 {
	r.need(2 * n)
	out := make([]uint16, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16(r.data[r.pos : r.pos+2])
		r.pos += 2
	}
	return out
}

func (r *geosReader) skip(n int) {
	r.need(n)
	r.pos += n
}
