package mdx

import "fmt"

// MaxInfluences is the most node ids/weights any vertex in wardata carries:
// 4 for every Skin vertex and every matrix group, but up to 8 for a few
// classic matrix groups the engine averages equally.
const MaxInfluences = 8

// Influences is the per-vertex skinning form: per vertex, MaxInfluences
// node object ids and weights summing to 1; unused slots have node 0,
// weight 0. Used slots come first.
type Influences struct {
	Nodes   []uint32  // MaxInfluences per vertex
	Weights []float32 // MaxInfluences per vertex
}

// Influences converts the geoset's matrix groups or Skin data (whichever is
// present; see Geoset) to one per-vertex form: up to MaxInfluences node
// object ids (Node.ObjectID, not indices into Bones()) and weights summing
// to 1, exact for every vertex seen in wardata.
//
// Influences panics ("mdx: geoset skin: ...") on a vertex count that
// doesn't match len(Skin) or len(VertexGroups), an index or group out of
// range, inconsistent group sizes, or a group with 0 or more than
// MaxInfluences nodes — none seen in wardata.
func (g Geoset) Influences() Influences {
	vertices := g.VertexCount()
	out := Influences{
		Nodes:   make([]uint32, vertices*MaxInfluences),
		Weights: make([]float32, vertices*MaxInfluences),
	}

	if g.Skin != nil {
		if len(g.Skin) != 8*vertices {
			panic(fmt.Sprintf("mdx: geoset skin: Skin length %d != 8 x %d vertices", len(g.Skin), vertices))
		}
		for v := 0; v < vertices; v++ {
			base := v * 8
			slot := 0
			for i := 0; i < 4; i++ {
				weight := g.Skin[base+4+i]
				if weight == 0 {
					continue
				}
				idx := int(g.Skin[base+i])
				if idx >= len(g.GroupNodes) {
					panic(fmt.Sprintf("mdx: geoset skin: vertex %d: index %d >= %d nodes", v, idx, len(g.GroupNodes)))
				}
				out.Nodes[v*MaxInfluences+slot] = g.GroupNodes[idx]
				out.Weights[v*MaxInfluences+slot] = float32(weight) / 255
				slot++
			}
		}
		return out
	}

	if len(g.VertexGroups) != vertices {
		panic(fmt.Sprintf("mdx: geoset skin: VertexGroups length %d != %d vertices", len(g.VertexGroups), vertices))
	}

	sum := uint32(0)
	starts := make([]uint32, len(g.GroupSizes))
	for i, size := range g.GroupSizes {
		starts[i] = sum
		sum += size
	}
	if sum != uint32(len(g.GroupNodes)) {
		panic(fmt.Sprintf("mdx: geoset skin: sum of group sizes %d != %d nodes", sum, len(g.GroupNodes)))
	}

	for v := 0; v < vertices; v++ {
		group := int(g.VertexGroups[v])
		if group >= len(g.GroupSizes) {
			panic(fmt.Sprintf("mdx: geoset skin: vertex %d: group %d >= %d groups", v, group, len(g.GroupSizes)))
		}
		size := g.GroupSizes[group]
		if size == 0 || size > MaxInfluences {
			panic(fmt.Sprintf("mdx: geoset skin: vertex %d: group %d has %d nodes", v, group, size))
		}
		start := starts[group]
		weight := 1 / float32(size)
		for i := uint32(0); i < size; i++ {
			out.Nodes[v*MaxInfluences+int(i)] = g.GroupNodes[start+i]
			out.Weights[v*MaxInfluences+int(i)] = weight
		}
	}

	return out
}
