package mdx

import "fmt"

// Interpolation constants for a track's Interpolation field (typed as plain
// uint32, like FloatTrack.Interpolation).
const (
	InterpNone    uint32 = 0
	InterpLinear  uint32 = 1
	InterpHermite uint32 = 2
	InterpBezier  uint32 = 3
)

// Track is a keyframe track of width-W values (vectors, quaternions, ...).
type Track[T any] struct {
	Interpolation  uint32
	GlobalSequence int32
	Keys           []Key[T]
}

// Key is one key of a Track. InTan and OutTan are zero when Interpolation is
// InterpNone or InterpLinear (no tangents stored).
type Key[T any] struct {
	Frame                int32
	Value, InTan, OutTan T
}

// readVec3Track parses a track body (after its tag) of float32[3] values,
// as used by KGTR (translation) and KGSC (scale).
func readVec3Track(r *reader) Track[[3]float32] {
	return readTrack(r, 3, func(r *reader) [3]float32 {
		return [3]float32{r.f32(), r.f32(), r.f32()}
	})
}

// readQuatTrack parses a track body (after its tag) of float32[4]
// (quaternion x y z w) values, as used by KGRT (rotation).
func readQuatTrack(r *reader) Track[[4]float32] {
	return readTrack(r, 4, func(r *reader) [4]float32 {
		return [4]float32{r.f32(), r.f32(), r.f32(), r.f32()}
	})
}

// readTrack parses a track body shared by readVec3Track and readQuatTrack:
// uint32 key count, uint32 interpolation, int32 global sequence id, then per
// key int32 frame + value (width floats via readValue), plus an in- and an
// out-tangent (same width) when interpolation > InterpLinear.
func readTrack[T any](r *reader, width int, readValue func(*reader) T) Track[T] {
	count := r.u32()
	t := Track[T]{Interpolation: r.u32(), GlobalSequence: r.i32()}

	keySize := 4 + 4*width
	if t.Interpolation > InterpLinear {
		keySize += 2 * 4 * width
	}
	if uint64(count)*uint64(keySize) > uint64(r.limit-r.pos) {
		panic(fmt.Sprintf("mdx: %s: truncated at offset %d", r.where, r.pos))
	}

	t.Keys = make([]Key[T], count)
	for i := range t.Keys {
		k := Key[T]{Frame: r.i32(), Value: readValue(r)}
		if t.Interpolation > InterpLinear {
			k.InTan = readValue(r)
			k.OutTan = readValue(r)
		}
		t.Keys[i] = k
	}

	return t
}
