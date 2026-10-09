package mdx

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"modelView/extract/buildConfig"
	"modelView/extract/buildinfo"
	"modelView/extract/casc"
	"modelView/extract/root"
)

func decodeHex(s string) []byte {
	d, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func configPath(f string) string { return "wardata/Data/config/" + f[0:2] + "/" + f[2:4] + "/" + f }

func lowestLOD(geosets []Geoset) uint32 {
	if len(geosets) == 0 {
		return 0
	}
	lod := geosets[0].LOD
	for _, g := range geosets {
		if g.LOD < lod {
			lod = g.LOD
		}
	}
	return lod
}

func finiteMat4(m Mat4) bool {
	for _, v := range m {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

type sampleTally struct {
	samples  int
	mismatch int
}

func quatClose(a, b [4]float32, tol float32) bool {
	diff := 0.0
	for i := range a {
		diff += math.Abs(float64(a[i]) - float64(b[i]))
	}
	if diff <= float64(tol)*4 {
		return true
	}
	diff = 0.0
	for i := range a {
		diff += math.Abs(float64(a[i]) + float64(b[i]))
	}
	return diff <= float64(tol)*4
}

func vec3Close(a, b [3]float32, tol float32) bool {
	return math.Abs(float64(a[0]-b[0])) <= float64(tol) &&
		math.Abs(float64(a[1]-b[1])) <= float64(tol) &&
		math.Abs(float64(a[2]-b[2])) <= float64(tol)
}

// dupStart returns the index of the first key in a run of equal frames
// starting at i, scanning forward from 0 (keys assumed sorted ascending).
func firstOfRun(frames []int32, i int) bool {
	return i == 0 || frames[i-1] != frames[i]
}

func TestANIM03Survey(t *testing.T) {
	if _, err := os.Stat("wardata/.build.info"); err != nil {
		t.Skip("no wardata, skipping survey")
	}

	data, err := os.ReadFile("wardata/.build.info")
	if err != nil {
		t.Fatal(err)
	}
	bInfo := buildinfo.ReadBuildInfo(data)
	cfgData, err := os.ReadFile(configPath(bInfo.GetBuildKey()))
	if err != nil {
		t.Fatal(err)
	}
	config := buildConfig.ReadBuildConfig(cfgData)
	ckey, ekey := config.GetEncoding()
	storage := casc.Open(true)
	defer storage.Close()
	encPkg, ok := storage.LookupEKey(decodeHex(ekey))
	if !ok {
		t.Fatal("no encoding")
	}
	storage.LoadEncoding(encPkg, [16]byte(decodeHex(ckey)))
	r := root.ReadRoot(storage.ReadByCKey([16]byte(decodeHex(config.GetRoot()))))

	type editionStats struct {
		models, panics     int
		nonFinite          int
		keyMismatch        int
		tally              map[string]*sampleTally
		alphaDiffNonGlobal int
		alphaDiffGlobal    int
	}
	per := map[root.Edition]*editionStats{}
	seen := map[root.Edition]map[[16]byte]bool{}
	var editions []root.Edition

	footmanBind := map[root.Edition][2][3]float32{}
	footmanStart := map[root.Edition][2][3]float32{}
	footmanMidWalk := map[root.Edition][2][3]float32{}
	footmanMeanStart := map[root.Edition]float64{}
	footmanMeanWalk := map[root.Edition]float64{}

	for _, e := range r.Entries {
		if !strings.HasSuffix(strings.ToLower(e.Name), ".mdx") {
			continue
		}
		ed := e.Variant.Edition
		if per[ed] == nil {
			per[ed] = &editionStats{tally: map[string]*sampleTally{}}
			seen[ed] = map[[16]byte]bool{}
			editions = append(editions, ed)
		}
		if seen[ed][e.CKey] {
			continue
		}
		seen[ed][e.CKey] = true
		encE, ok := storage.LookupCKey(e.CKey)
		if !ok {
			continue
		}
		ek := encE.EKeys[0]
		if _, ok := storage.LookupEKey(ek[:]); !ok {
			continue
		}
		d := storage.ReadByCKey(e.CKey)
		s := per[ed]
		isFootman := strings.EqualFold(e.Name, "Units/Human/Footman/Footman.mdx")

		func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.panics++
					fmt.Fprintln(os.Stderr, "panic", ed, e.Path, rec)
				}
			}()

			m := Read(d)
			geosets := m.Geosets()
			if m.Version != 1800 || len(geosets) == 0 {
				return
			}
			s.models++

			seqs := m.Sequences()
			globals := m.GlobalSequences()
			skel := NewSkeleton(m)

			tally := func(kind string, interp uint32) *sampleTally {
				key := fmt.Sprintf("%s/%s", kind, iname(interp))
				if s.tally[key] == nil {
					s.tally[key] = &sampleTally{}
				}
				return s.tally[key]
			}

			checkMats := func(mats []Mat4) {
				for _, mt := range mats {
					if !finiteMat4(mt) {
						s.nonFinite++
					}
				}
			}

			for _, sq := range seqs {
				if sq.End < sq.Start {
					continue
				}
				mid := sq.Start + (sq.End-sq.Start)/2
				for _, frame := range []uint32{sq.Start, mid, sq.End} {
					checkMats(skel.WorldMatrices(AnimTime{Sequence: sq, Frame: frame, Global: 0}))
					checkMats(skel.WorldMatrices(AnimTime{Sequence: sq, Frame: frame, Global: frame - sq.Start}))
				}
			}

			var nodes []Node
			for _, b := range m.Bones() {
				nodes = append(nodes, b.Node)
			}
			nodes = append(nodes, m.Helpers()...)

			// sampleIntervals returns the (sequence, start, end) ranges to test a
			// track over: one per sequence for a non-global track, or a single
			// [0, length) range (the boundary frame == length wraps to 0 on the
			// global clock, so it is excluded) for a global one.
			type sampleInterval struct {
				sq         Sequence
				start, end uint32
			}
			sampleIntervals := func(globalSeq int32) []sampleInterval {
				if globalSeq < 0 {
					var out []sampleInterval
					for _, sq := range seqs {
						if sq.End >= sq.Start {
							out = append(out, sampleInterval{sq, sq.Start, sq.End})
						}
					}
					return out
				}
				if int(globalSeq) >= len(globals) || globals[globalSeq] == 0 {
					return nil
				}
				return []sampleInterval{{Sequence{}, 0, globals[globalSeq] - 1}}
			}

			checkVec3 := func(kind string, trk *Track[[3]float32]) {
				if trk == nil {
					return
				}
				frames := trackFrames(trk.Keys)
				for _, iv := range sampleIntervals(trk.GlobalSequence) {
					for i, f := range frames {
						if !firstOfRun(frames, i) {
							continue
						}
						if uint32(f) < iv.start || uint32(f) > iv.end {
							continue
						}
						at := AnimTime{Sequence: iv.sq, Frame: uint32(f), Global: uint32(f)}
						got := sampleVec3(trk, [3]float32{}, at, globals)
						tl := tally(kind, trk.Interpolation)
						tl.samples++
						if !vec3Close(got, trk.Keys[i].Value, 1e-4) {
							tl.mismatch++
							s.keyMismatch++
							if s.keyMismatch <= 5 {
								fmt.Fprintf(os.Stderr, "mismatch %s %s kind=%s frame=%d got=%v want=%v sorted=%v\n", ed, e.Path, kind, f, got, trk.Keys[i].Value, sort.IsSorted(sort.IntSlice(int32ToInt(frames))))
							}
						}
					}
				}
			}
			checkQuat := func(trk *Track[[4]float32]) {
				if trk == nil {
					return
				}
				frames := trackFrames(trk.Keys)
				for _, iv := range sampleIntervals(trk.GlobalSequence) {
					for i, f := range frames {
						if !firstOfRun(frames, i) {
							continue
						}
						if uint32(f) < iv.start || uint32(f) > iv.end {
							continue
						}
						at := AnimTime{Sequence: iv.sq, Frame: uint32(f), Global: uint32(f)}
						got := sampleQuat(trk, [4]float32{0, 0, 0, 1}, at, globals)
						tl := tally("rotation", trk.Interpolation)
						tl.samples++
						if !quatClose(got, trk.Keys[i].Value, 1e-3) {
							tl.mismatch++
							s.keyMismatch++
							if s.keyMismatch <= 5 {
								fmt.Fprintf(os.Stderr, "mismatch %s %s kind=rotation frame=%d got=%v want=%v sorted=%v\n", ed, e.Path, f, got, trk.Keys[i].Value, sort.IsSorted(sort.IntSlice(int32ToInt(frames))))
							}
						}
					}
				}
			}
			checkFloat := func(trk *FloatTrack) {
				if trk == nil || trk.GlobalSequence >= 0 {
					return
				}
				frames := floatTrackFrames(trk.Keys)
				for _, sq := range seqs {
					if sq.End < sq.Start {
						continue
					}
					for i, f := range frames {
						if !firstOfRun(frames, i) {
							continue
						}
						if uint32(f) < sq.Start || uint32(f) > sq.End {
							continue
						}
						at := AnimTime{Sequence: sq, Frame: uint32(f), Global: 0}
						got := sampleFloat(trk, 1, at, globals)
						tl := tally("alpha", trk.Interpolation)
						tl.samples++
						if math.Abs(float64(got-trk.Keys[i].Value)) > 1e-4 {
							tl.mismatch++
							s.keyMismatch++
						}
					}
				}
			}

			for _, n := range nodes {
				checkVec3("translation", n.Translation)
				checkQuat(n.Rotation)
				checkVec3("scale", n.Scale)
			}
			anims := m.GeosetAnims()
			for _, a := range anims {
				checkFloat(a.AlphaTrack)
			}

			def, hasDef := DefaultSequence(seqs)
			if hasDef {
				for _, a := range anims {
					t := a.AlphaTrack
					if t == nil {
						continue
					}
					old := a.AlphaAt(def)
					neu := a.AlphaAtTime(AnimTime{Sequence: def, Frame: def.Start, Global: 0}, globals)
					diff := math.Abs(float64(old - neu))
					if t.GlobalSequence >= 0 {
						if diff > 1e-4 {
							s.alphaDiffGlobal++
						}
					} else {
						if diff > 1e-4 {
							s.alphaDiffNonGlobal++
						}
					}
				}
			}

			if isFootman && hasDef {
				lod := lowestLOD(geosets)
				bindBounds := [2][3]float32{{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, {-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}}
				var startBounds, walkBounds [2][3]float32
				startBounds = [2][3]float32{{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, {-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}}
				walkBounds = startBounds

				var walkSeq Sequence
				for _, sq := range seqs {
					if strings.HasPrefix(strings.ToLower(sq.Name), "walk") {
						walkSeq = sq
						break
					}
				}

				startWorld := skel.WorldMatrices(AnimTime{Sequence: def, Frame: def.Start, Global: 0})
				var midWalkWorld []Mat4
				if walkSeq.End > walkSeq.Start {
					mid := walkSeq.Start + (walkSeq.End-walkSeq.Start)/2
					midWalkWorld = skel.WorldMatrices(AnimTime{Sequence: walkSeq, Frame: mid, Global: mid - walkSeq.Start})
				}

				var sumStart, sumWalk float64
				var countStart, countWalk int
				for _, g := range geosets {
					if g.LOD != lod {
						continue
					}
					inf := g.Influences()
					vcount := g.VertexCount()
					for v := 0; v < vcount; v++ {
						px, py, pz := g.Positions[v*3], g.Positions[v*3+1], g.Positions[v*3+2]
						bx, by, bz := px, py, pz

						expand := func(bounds *[2][3]float32, x, y, z float32) {
							if x < bounds[0][0] {
								bounds[0][0] = x
							}
							if y < bounds[0][1] {
								bounds[0][1] = y
							}
							if z < bounds[0][2] {
								bounds[0][2] = z
							}
							if x > bounds[1][0] {
								bounds[1][0] = x
							}
							if y > bounds[1][1] {
								bounds[1][1] = y
							}
							if z > bounds[1][2] {
								bounds[1][2] = z
							}
						}
						expand(&bindBounds, bx, by, bz)

						skinnedStart := [3]float32{}
						skinnedWalk := [3]float32{}
						haveWalk := midWalkWorld != nil
						for sl := 0; sl < MaxInfluences; sl++ {
							w := inf.Weights[v*MaxInfluences+sl]
							if w == 0 {
								continue
							}
							node := inf.Nodes[v*MaxInfluences+sl]
							sx, sy, sz := applyMat4(startWorld[node], px, py, pz)
							skinnedStart[0] += w * sx
							skinnedStart[1] += w * sy
							skinnedStart[2] += w * sz
							if haveWalk {
								wx, wy, wz := applyMat4(midWalkWorld[node], px, py, pz)
								skinnedWalk[0] += w * wx
								skinnedWalk[1] += w * wy
								skinnedWalk[2] += w * wz
							}
						}
						expand(&startBounds, skinnedStart[0], skinnedStart[1], skinnedStart[2])
						sumStart += dist3(skinnedStart, [3]float32{bx, by, bz})
						countStart++
						if haveWalk {
							expand(&walkBounds, skinnedWalk[0], skinnedWalk[1], skinnedWalk[2])
							sumWalk += dist3(skinnedWalk, [3]float32{bx, by, bz})
							countWalk++
						}
					}
				}

				footmanBind[ed] = bindBounds
				footmanStart[ed] = startBounds
				footmanMidWalk[ed] = walkBounds
				if countStart > 0 {
					footmanMeanStart[ed] = sumStart / float64(countStart)
				}
				if countWalk > 0 {
					footmanMeanWalk[ed] = sumWalk / float64(countWalk)
				}
			}
		}()
	}

	for _, ed := range editions {
		s := per[ed]
		t.Logf("== %s: models %d panics %d nonFinite %d keyMismatch %d", ed, s.models, s.panics, s.nonFinite, s.keyMismatch)
		keys := make([]string, 0, len(s.tally))
		for k := range s.tally {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			tl := s.tally[k]
			t.Logf("  tally %s: samples %d mismatch %d", k, tl.samples, tl.mismatch)
		}
		t.Logf("  alpha diff vs AlphaAt: nonGlobal %d global %d", s.alphaDiffNonGlobal, s.alphaDiffGlobal)
		if b, ok := footmanBind[ed]; ok {
			t.Logf("  footman bind bounds min %v max %v", b[0], b[1])
			t.Logf("  footman start bounds min %v max %v mean displacement %.4f", footmanStart[ed][0], footmanStart[ed][1], footmanMeanStart[ed])
			t.Logf("  footman mid-walk bounds min %v max %v mean displacement %.4f", footmanMidWalk[ed][0], footmanMidWalk[ed][1], footmanMeanWalk[ed])
		}
	}
}

var interpName = []string{"none", "linear", "hermite", "bezier"}

func iname(i uint32) string {
	if int(i) < len(interpName) {
		return interpName[i]
	}
	return fmt.Sprint(i)
}

func applyMat4(m Mat4, x, y, z float32) (float32, float32, float32) {
	return m[0]*x + m[4]*y + m[8]*z + m[12],
		m[1]*x + m[5]*y + m[9]*z + m[13],
		m[2]*x + m[6]*y + m[10]*z + m[14]
}

func dist3(a, b [3]float32) float64 {
	dx := float64(a[0] - b[0])
	dy := float64(a[1] - b[1])
	dz := float64(a[2] - b[2])
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
