package mdx

import (
	"math"
	"sort"
)

// AnimTime is a point in a model's animation: a frame inside a sequence,
// plus the global clock used by tracks on a global sequence. Frame is
// absolute (not relative to Sequence.Start); the sampler clamps it to the
// sequence's interval.
type AnimTime struct {
	Sequence Sequence
	Frame    uint32
	Global   uint32
}

// interval returns the [start, end] frame range a track with the given
// global sequence id is evaluated over, and the frame within it: at.Frame
// clamped to [Sequence.Start, Sequence.End] for a non-global track
// (globalSeq < 0), else at.Global mod the global sequence's length (length
// 0, or an out-of-range id, behaves as length 0, i.e. always frame 0) in
// [0, length].
func interval(at AnimTime, globalSeq int32, globals []uint32) (start, end, frame uint32) {
	if globalSeq < 0 {
		return at.Sequence.Start, at.Sequence.End, at.Frame
	}
	var length uint32
	if int(globalSeq) < len(globals) {
		length = globals[globalSeq]
	}
	if length == 0 {
		return 0, 0, 0
	}
	return 0, length, at.Global % length
}

// bracket returns the indices of the two keys bracketing frame within
// [start, end] among frames (sorted ascending, as every track in wardata
// but one is): the index of the last key with Frame <= frame among those in
// range (or -1 when none is), and the index of the first key with
// Frame > that one's frame (duplicate-frame runs collapse to one key) and
// Frame <= end (or -1 when there is none, i.e. k0 is the last in-range
// key). Keys outside [start, end] are ignored.
func bracket(frames []int32, start, end, frame uint32) (k0, k1 int) {
	lo := sort.Search(len(frames), func(i int) bool { return frames[i] >= int32(start) })
	hi := sort.Search(len(frames), func(i int) bool { return frames[i] > int32(end) })

	k0 = -1
	for i := lo; i < hi; i++ {
		if uint32(frames[i]) <= frame {
			k0 = i
		} else {
			break
		}
	}
	if k0 < 0 {
		if lo < hi {
			return -1, lo
		}
		return -1, -1
	}
	k1 = -1
	for i := k0 + 1; i < hi; i++ {
		if frames[i] > frames[k0] {
			k1 = i
			break
		}
	}
	return k0, k1
}

// sampleT returns t in [0, 1] for frame between keys at f0 and f1 (k1's
// value when f1 == f0, as seen in a few duplicate-frame tracks).
func sampleT(frame, f0, f1 int32) float32 {
	if f1 == f0 {
		return 1
	}
	return float32(frame-f0) / float32(f1-f0)
}

// sampleVec3 samples a [3]float32 track at at, returning def when t is nil.
// Key selection and interpolation follow the rules in ANIM-03's plan:
// none/linear/hermite/bezier, held at the first or last in-range key
// outside it.
func sampleVec3(t *Track[[3]float32], def [3]float32, at AnimTime, globals []uint32) [3]float32 {
	if t == nil || len(t.Keys) == 0 {
		return def
	}
	start, end, frame := interval(at, t.GlobalSequence, globals)
	frames := trackFrames(t.Keys)
	k0, k1 := bracket(frames, start, end, frame)

	switch {
	case k0 < 0 && k1 < 0:
		return def
	case k0 < 0:
		return t.Keys[k1].Value
	case k1 < 0:
		return t.Keys[k0].Value
	}

	tt := sampleT(int32(frame), t.Keys[k0].Frame, t.Keys[k1].Frame)
	a, b := t.Keys[k0], t.Keys[k1]
	switch t.Interpolation {
	case InterpNone:
		return a.Value
	case InterpLinear:
		return lerp3(a.Value, b.Value, tt)
	case InterpHermite:
		return hermite3(a.Value, a.OutTan, b.InTan, b.Value, tt)
	default: // InterpBezier
		return bezier3(a.Value, a.OutTan, b.InTan, b.Value, tt)
	}
}

// sampleQuat samples a [4]float32 (quaternion x y z w) track at at,
// returning def when t is nil. Hermite and bezier use squad.
func sampleQuat(t *Track[[4]float32], def [4]float32, at AnimTime, globals []uint32) [4]float32 {
	if t == nil || len(t.Keys) == 0 {
		return def
	}
	start, end, frame := interval(at, t.GlobalSequence, globals)
	frames := trackFrames(t.Keys)
	k0, k1 := bracket(frames, start, end, frame)

	switch {
	case k0 < 0 && k1 < 0:
		return def
	case k0 < 0:
		return t.Keys[k1].Value
	case k1 < 0:
		return t.Keys[k0].Value
	}

	tt := sampleT(int32(frame), t.Keys[k0].Frame, t.Keys[k1].Frame)
	a, b := t.Keys[k0], t.Keys[k1]
	switch t.Interpolation {
	case InterpNone:
		return a.Value
	case InterpLinear:
		return slerp(a.Value, b.Value, tt)
	default: // InterpHermite, InterpBezier
		return squad(a.Value, a.OutTan, b.InTan, b.Value, tt)
	}
}

// sampleFloat samples a FloatTrack at at, returning def when t is nil.
func sampleFloat(t *FloatTrack, def float32, at AnimTime, globals []uint32) float32 {
	if t == nil || len(t.Keys) == 0 {
		return def
	}
	start, end, frame := interval(at, t.GlobalSequence, globals)
	frames := floatTrackFrames(t.Keys)
	k0, k1 := bracket(frames, start, end, frame)

	switch {
	case k0 < 0 && k1 < 0:
		return def
	case k0 < 0:
		return t.Keys[k1].Value
	case k1 < 0:
		return t.Keys[k0].Value
	}

	tt := sampleT(int32(frame), t.Keys[k0].Frame, t.Keys[k1].Frame)
	a, b := t.Keys[k0], t.Keys[k1]
	switch t.Interpolation {
	case InterpNone:
		return a.Value
	case InterpLinear:
		return lerp(a.Value, b.Value, tt)
	case InterpHermite:
		return hermite(a.Value, a.OutTan, b.InTan, b.Value, tt)
	default: // InterpBezier
		return bezier(a.Value, a.OutTan, b.InTan, b.Value, tt)
	}
}

func trackFrames[T any](keys []Key[T]) []int32 {
	frames := make([]int32, len(keys))
	for i, k := range keys {
		frames[i] = k.Frame
	}
	return frames
}

func floatTrackFrames(keys []FloatKey) []int32 {
	frames := make([]int32, len(keys))
	for i, k := range keys {
		frames[i] = k.Frame
	}
	return frames
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func lerp3(a, b [3]float32, t float32) [3]float32 {
	return [3]float32{lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)}
}

// hermite evaluates the Hermite curve through v0 (out-tangent out0) to v1
// (in-tangent in1) at t.
func hermite(v0, out0, in1, v1, t float32) float32 {
	t2 := t * t
	t3 := t2 * t
	return v0*(2*t3-3*t2+1) + out0*(t3-2*t2+t) + in1*(t3-t2) + v1*(3*t2-2*t3)
}

func hermite3(v0, out0, in1, v1 [3]float32, t float32) [3]float32 {
	return [3]float32{
		hermite(v0[0], out0[0], in1[0], v1[0], t),
		hermite(v0[1], out0[1], in1[1], v1[1], t),
		hermite(v0[2], out0[2], in1[2], v1[2], t),
	}
}

// bezier evaluates the cubic Bezier curve with control points v0, v0+out0,
// v1+in1, v1 at t (the engine stores out0/in1 as the tangent, not an
// absolute control point).
func bezier(v0, out0, in1, v1, t float32) float32 {
	u := 1 - t
	return v0*u*u*u + out0*3*t*u*u + in1*3*t*t*u + v1*t*t*t
}

func bezier3(v0, out0, in1, v1 [3]float32, t float32) [3]float32 {
	return [3]float32{
		bezier(v0[0], out0[0], in1[0], v1[0], t),
		bezier(v0[1], out0[1], in1[1], v1[1], t),
		bezier(v0[2], out0[2], in1[2], v1[2], t),
	}
}

func dotQuat(a, b [4]float32) float32 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] + a[3]*b[3]
}

func normalizeQuat(q [4]float32) [4]float32 {
	n := float32(math.Sqrt(float64(dotQuat(q, q))))
	if n == 0 {
		return q
	}
	return [4]float32{q[0] / n, q[1] / n, q[2] / n, q[3] / n}
}

// slerp spherically interpolates quaternions a and b at t (shortest path;
// falls back to nlerp when they are nearly parallel, to avoid a division by
// ~0).
func slerp(a, b [4]float32, t float32) [4]float32 {
	d := dotQuat(a, b)
	if d < 0 {
		b = [4]float32{-b[0], -b[1], -b[2], -b[3]}
		d = -d
	}
	if d > 0.9995 {
		return normalizeQuat([4]float32{
			lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t), lerp(a[3], b[3], t),
		})
	}
	theta0 := math.Acos(float64(d))
	theta := theta0 * float64(t)
	sinTheta0 := math.Sin(theta0)
	s0 := float32(math.Cos(theta) - float64(d)*math.Sin(theta)/sinTheta0)
	s1 := float32(math.Sin(theta) / sinTheta0)
	return [4]float32{
		a[0]*s0 + b[0]*s1,
		a[1]*s0 + b[1]*s1,
		a[2]*s0 + b[2]*s1,
		a[3]*s0 + b[3]*s1,
	}
}

// squad evaluates the spherical cubic spline through quaternions q0
// (out-tangent out0) to q1 (in-tangent in1) at t, as
// slerp(slerp(q0, q1, t), slerp(out0, in1, t), 2t(1-t)).
func squad(q0, out0, in1, q1 [4]float32, t float32) [4]float32 {
	return slerp(slerp(q0, q1, t), slerp(out0, in1, t), 2*t*(1-t))
}
