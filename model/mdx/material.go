package mdx

import "fmt"

// FilterMode is a layer's blending mode.
type FilterMode uint32

const (
	FilterNone        FilterMode = 0
	FilterTransparent FilterMode = 1
	FilterBlend       FilterMode = 2
	FilterAdditive    FilterMode = 3
	FilterAddAlpha    FilterMode = 4
	FilterModulate    FilterMode = 5
	FilterModulate2x  FilterMode = 6
)

func (f FilterMode) String() string {
	switch f {
	case FilterNone:
		return "none"
	case FilterTransparent:
		return "transparent"
	case FilterBlend:
		return "blend"
	case FilterAdditive:
		return "additive"
	case FilterAddAlpha:
		return "add alpha"
	case FilterModulate:
		return "modulate"
	case FilterModulate2x:
		return "modulate 2x"
	default:
		return fmt.Sprintf("filter %d", uint32(f))
	}
}

// Material is one entry of an MTLS chunk.
type Material struct {
	PriorityPlane int32
	Flags         uint32 // 0 in every material seen in wardata
	Layers        []Layer
}

// Layer is one layer of a Material.
type Layer struct {
	FilterMode    FilterMode
	ShadingFlags  uint32 // bit 0 unshaded, 1 sphere env map, 4 two-sided, 5 unfogged, 6 no depth test, 7 no depth set; 8-10 unnamed, reforged/definitive only
	TextureAnimID int32  // -1 when none
	CoordID       uint32
	Alpha         float32
	Textures      []LayerTexture
}

// LayerTexture is one texture slot of a Layer.
type LayerTexture struct {
	ID   uint32
	Slot uint32 // position in Textures for HD (6-texture) layers: 0 diffuse, 1 normal, 2 ORM, 3 emissive, 4 team colour, 5 environment; a few fountain/waterfall layers reuse slot 0 for a second diffuse texture
}

// Materials parses the model's MTLS chunk, if any, into materials. It
// returns nil when there is no MTLS chunk.
//
// MTLS layout in version 1800 (the only version seen in wardata): materials
// packed back to back, each: uint32 inclusive size, int32 priority plane,
// uint32 flags, tag "LAYS", uint32 layer count, then that many layers,
// which always end exactly at the material's end.
//
// A layer: uint32 inclusive size, uint32 filter mode, uint32 shading flags,
// uint32 unknown (always 0, likely the old single texture id, superseded by
// the texture list), int32 texture animation id, uint32 coord id, float32
// alpha, float32 emissive gain, float32[3] fresnel colour, float32 fresnel
// opacity, float32 fresnel team colour, uint32 unknown (0 for 1-texture
// layers, 1 for 6-texture/HD-shader layers, with a handful of other values
// seen), uint32 texture count (60 B so far), then per texture: uint32
// texture id, uint32 slot, and an optional "KMTF" track (texture-id
// flipbook: key = int32 frame + uint32 id, read as a FloatTrack of width 1
// and discarded) right after that texture's entry. After the textures,
// layer tracks run to the layer's end; only "KMTA" (alpha) and "KMTE"
// (emissive) are seen, both width 1, read as FloatTracks and discarded.
//
// Emissive gain, fresnel and the two unknown uint32s are not kept (for the
// later PBR epic).
//
// Materials panics ("mdx: MTLS ...") on truncated data, a material or layer
// running past its end, a missing "LAYS" tag, or bytes left over after a
// material's layers. It panics with "mdx: MTLS parsing supports version
// 1800 only, got %d" on an MTLS chunk in any other version.
func (m Model) Materials() []Material {
	var mtlsChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "MTLS" {
			mtlsChunk = &m.Chunks[i]
			break
		}
	}
	if mtlsChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: MTLS parsing supports version 1800 only, got %d", m.Version))
	}

	data := mtlsChunk.Data
	var mats []Material
	pos := 0
	for pos < len(data) {
		mat, n := readMaterial(data, pos, len(mats))
		mats = append(mats, mat)
		pos += n
	}

	return mats
}

// readMaterial parses one MTLS entry starting at pos in data and returns it
// along with its size in bytes (the inclusive size field).
func readMaterial(data []byte, start int, index int) (Material, int) {
	r := reader{data: data, pos: start, limit: len(data), where: fmt.Sprintf("MTLS material %d", index)}

	size := r.u32()
	end := start + int(size)
	if uint64(end) > uint64(len(data)) {
		panic(fmt.Sprintf("mdx: MTLS material %d: runs past chunk end", index))
	}
	r.limit = end

	var mat Material
	mat.PriorityPlane = r.i32()
	mat.Flags = r.u32()
	if tag := r.tag(); tag != "LAYS" {
		panic(fmt.Sprintf("mdx: MTLS material %d: want LAYS, got %q", index, tag))
	}
	layerCount := r.u32()

	mat.Layers = make([]Layer, layerCount)
	for i := range mat.Layers {
		l, n := readLayer(data, r.pos, index, i, end)
		mat.Layers[i] = l
		r.pos += n
	}

	if r.pos != end {
		panic(fmt.Sprintf("mdx: MTLS material %d: %d bytes left over", index, end-r.pos))
	}

	return mat, int(size)
}

// readLayer parses one layer starting at pos in data, bounded by matEnd (the
// enclosing material's end), and returns it along with its size in bytes
// (the inclusive size field).
func readLayer(data []byte, start int, matIndex int, layerIndex int, matEnd int) (Layer, int) {
	r := reader{data: data, pos: start, limit: matEnd, where: fmt.Sprintf("MTLS material %d layer %d", matIndex, layerIndex)}

	size := r.u32()
	end := start + int(size)
	if uint64(end) > uint64(matEnd) {
		panic(fmt.Sprintf("mdx: MTLS material %d layer %d: runs past material end", matIndex, layerIndex))
	}
	r.limit = end

	var l Layer
	l.FilterMode = FilterMode(r.u32())
	l.ShadingFlags = r.u32()
	r.skip(4) // unknown, always 0
	l.TextureAnimID = r.i32()
	l.CoordID = r.u32()
	l.Alpha = r.f32()
	r.skip(4)  // emissive gain
	r.skip(12) // fresnel colour
	r.skip(4)  // fresnel opacity
	r.skip(4)  // fresnel team colour
	r.skip(4)  // unknown, SD/HD shader marker
	texCount := r.u32()

	l.Textures = make([]LayerTexture, texCount)
	for i := range l.Textures {
		id := r.u32()
		slot := r.u32()
		l.Textures[i] = LayerTexture{ID: id, Slot: slot}

		if r.limit-r.pos >= 4 && r.peekTag() == "KMTF" {
			r.tag()
			readFloatTrack(&r, 1) // texture-id flipbook, values are uint32 texture ids; discarded
		}
	}

	for r.pos < r.limit {
		r.tag()
		readFloatTrack(&r, 1) // KMTA / KMTE, discarded
	}

	return l, int(size)
}
