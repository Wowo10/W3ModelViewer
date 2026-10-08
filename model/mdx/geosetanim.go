package mdx

import "fmt"

// GeosetAnim is one entry of a GEOA chunk: the static alpha and (optional)
// alpha track of the geoset named by GeosetID.
type GeosetAnim struct {
	GeosetID   uint32
	Alpha      float32
	AlphaTrack *FloatTrack // from a KGAO track; nil when the entry has none
}

// FloatTrack is a keyframe track of float32 values (width 1 for KGAO; wider
// tracks, e.g. KGAC, are skipped by GeosetAnims).
type FloatTrack struct {
	Interpolation  uint32
	GlobalSequence int32
	Keys           []FloatKey
}

// FloatKey is one key of a FloatTrack; in- and out-tangents are read (when
// present) but not kept, as nothing needs them yet.
type FloatKey struct {
	Frame int32
	Value float32
}

// GeosetAnims parses the model's GEOA chunk, if any, into geoset animations.
// It returns nil when there is no GEOA chunk.
//
// GEOA layout in version 1800 (the only version seen in wardata): entries
// packed back to back, each: uint32 inclusive size, float32 static alpha,
// uint32 flags, float32[3] colour, uint32 geoset id (28 B), then tracks
// until the entry's end. A track is: a 4-byte tag, uint32 key count, uint32
// interpolation, int32 global sequence id, then per key: int32 frame, value
// (float32 x the track's width), plus an in- and an out-tangent (same
// width as the value) when interpolation > 1. Two track tags are seen:
// KGAO (alpha, width 1, kept as AlphaTrack) and KGAC (colour, width 3,
// skipped by size). Flags and colour are skipped (not needed yet).
//
// GeosetAnims panics ("mdx: GEOA entry %d: ...") on truncated data, an
// entry running past the chunk, or an unknown track tag. It panics with
// "mdx: GEOA parsing supports version 1800 only, got %d" on a GEOA chunk in
// any other version.
func (m Model) GeosetAnims() []GeosetAnim {
	var geoaChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "GEOA" {
			geoaChunk = &m.Chunks[i]
			break
		}
	}
	if geoaChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: GEOA parsing supports version 1800 only, got %d", m.Version))
	}

	data := geoaChunk.Data
	var anims []GeosetAnim
	pos := 0
	for pos < len(data) {
		a, n := readGeosetAnim(data, pos, len(anims))
		anims = append(anims, a)
		pos += n
	}

	return anims
}

// readGeosetAnim parses one GEOA entry starting at pos in data and returns
// it along with its size in bytes (the inclusive size field).
func readGeosetAnim(data []byte, start int, index int) (GeosetAnim, int) {
	r := reader{data: data, pos: start, limit: len(data), where: fmt.Sprintf("GEOA entry %d", index)}

	size := r.u32()
	end := start + int(size)
	if uint64(end) > uint64(len(data)) {
		panic(fmt.Sprintf("mdx: GEOA entry %d: runs past chunk end", index))
	}
	r.limit = end

	var a GeosetAnim
	a.Alpha = r.f32()
	r.skip(4)  // flags
	r.skip(12) // colour
	a.GeosetID = r.u32()

	for r.pos < r.limit {
		tag := r.tag()
		switch tag {
		case "KGAO":
			t := readFloatTrack(&r, 1)
			a.AlphaTrack = &t
		case "KGAC":
			readFloatTrack(&r, 3)
		default:
			panic(fmt.Sprintf("mdx: GEOA entry %d: unknown track %q", index, tag))
		}
	}

	return a, int(size)
}

// AlphaAt returns the geoset's alpha at the start of sequence s: the static
// alpha when there is no alpha track or it runs on a global sequence
// (unaffected by s), else the value of the first key whose frame is in
// [s.Start, s.End] (a key at the start, or the first key in the interval
// held back to it, since no interpolation is needed before the first key);
// the static alpha when no key falls in the interval.
func (a GeosetAnim) AlphaAt(s Sequence) float32 {
	t := a.AlphaTrack
	if t == nil || t.GlobalSequence >= 0 {
		return a.Alpha
	}
	for _, k := range t.Keys {
		if int64(k.Frame) >= int64(s.Start) && int64(k.Frame) <= int64(s.End) {
			return k.Value
		}
	}
	return a.Alpha
}

// GeosetAlphas returns one alpha per geoset (0..geosets-1), evaluated at the
// start of sequence s via AlphaAt for geosets with a GeosetAnim; geosets
// without one, or when ok is false (no sequences), get alpha 1.
func GeosetAlphas(geosets int, anims []GeosetAnim, s Sequence, ok bool) []float32 {
	alphas := make([]float32, geosets)
	for i := range alphas {
		alphas[i] = 1
	}
	if !ok {
		return alphas
	}
	for _, a := range anims {
		if int(a.GeosetID) < len(alphas) {
			alphas[a.GeosetID] = a.AlphaAt(s)
		}
	}
	return alphas
}

// readFloatTrack parses a track body (after its tag) with values of the
// given width (number of float32 per key, also used for tangents).
func readFloatTrack(r *reader, width int) FloatTrack {
	count := r.u32()
	t := FloatTrack{Interpolation: r.u32(), GlobalSequence: r.i32()}

	keySize := 4 * (1 + width)
	if t.Interpolation > 1 {
		keySize += 8 * width
	}
	if uint64(count)*uint64(keySize) > uint64(r.limit-r.pos) {
		panic(fmt.Sprintf("mdx: %s: truncated at offset %d", r.where, r.pos))
	}

	t.Keys = make([]FloatKey, count)
	for i := range t.Keys {
		frame := r.i32()
		value := r.f32()
		for w := 1; w < width; w++ {
			r.f32()
		}
		t.Keys[i] = FloatKey{Frame: frame, Value: value}

		if t.Interpolation > 1 {
			r.skip(4 * width) // in-tangent
			r.skip(4 * width) // out-tangent
		}
	}

	return t
}
