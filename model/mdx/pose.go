package mdx

import "sort"

// Mat4 is a column-major 4x4 matrix (same layout as the viewer's mat4), so
// ANIM-05 can upload it directly.
type Mat4 [16]float32

// identityMat4 returns the identity matrix.
func identityMat4() Mat4 {
	return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

// mul returns a*b (a applied after b, as in GL conventions).
func (a Mat4) mul(b Mat4) Mat4 {
	var r Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			r[col*4+row] = sum
		}
	}
	return r
}

func translationMat4(v [3]float32) Mat4 {
	m := identityMat4()
	m[12], m[13], m[14] = v[0], v[1], v[2]
	return m
}

func scaleMat4(v [3]float32) Mat4 {
	m := identityMat4()
	m[0], m[5], m[10] = v[0], v[1], v[2]
	return m
}

// rotationMat4 builds a rotation matrix from a quaternion (x, y, z, w).
func rotationMat4(q [4]float32) Mat4 {
	x, y, z, w := q[0], q[1], q[2], q[3]
	xx, yy, zz := x*x, y*y, z*z
	xy, xz, yz := x*y, x*z, y*z
	wx, wy, wz := w*x, w*y, w*z

	m := identityMat4()
	m[0], m[4], m[8] = 1-2*(yy+zz), 2*(xy-wz), 2*(xz+wy)
	m[1], m[5], m[9] = 2*(xy+wz), 1-2*(xx+zz), 2*(yz-wx)
	m[2], m[6], m[10] = 2*(xz-wy), 2*(yz+wx), 1-2*(xx+yy)
	return m
}

// skeletonNode is one BONE/HELP entry kept by Skeleton, in parent-first
// order.
type skeletonNode struct {
	Node
	objectID int32 // index into pivots/world matrices
	parent   int32 // index into skeleton.nodes, -1 for a root
}

// Skeleton is a model's bones and helpers, ready to be posed with
// WorldMatrices. Built once per model by NewSkeleton.
type Skeleton struct {
	nodes   []skeletonNode
	pivots  [][3]float32
	globals []uint32
}

// NewSkeleton builds a Skeleton from the model's bones, helpers, pivots and
// global sequences, ordering nodes parent-first (by depth, computed once)
// so WorldMatrices can compose each node's world matrix from its parent's
// in a single pass. A node whose parent id doesn't name another bone or
// helper (e.g. the 46 classic effect nodes parented to an unparsed node
// type) is treated as a root.
func NewSkeleton(m Model) Skeleton {
	bones := m.Bones()
	helpers := m.Helpers()

	nodes := make([]skeletonNode, 0, len(bones)+len(helpers))
	byObjectID := make(map[int32]int)
	for _, b := range bones {
		nodes = append(nodes, skeletonNode{Node: b.Node, objectID: int32(b.ObjectID)})
		byObjectID[int32(b.ObjectID)] = len(nodes) - 1
	}
	for _, h := range helpers {
		nodes = append(nodes, skeletonNode{Node: h, objectID: int32(h.ObjectID)})
		byObjectID[int32(h.ObjectID)] = len(nodes) - 1
	}

	for i := range nodes {
		if idx, ok := byObjectID[nodes[i].ParentID]; ok && idx != i {
			nodes[i].parent = int32(idx)
		} else {
			nodes[i].parent = -1
		}
	}

	depth := make([]int, len(nodes))
	var depthOf func(i int) int
	depthOf = func(i int) int {
		if nodes[i].parent < 0 {
			return 0
		}
		return 1 + depthOf(int(nodes[i].parent))
	}
	for i := range nodes {
		depth[i] = depthOf(i)
	}
	order := make([]int, len(nodes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return depth[order[a]] < depth[order[b]] })

	sorted := make([]skeletonNode, len(nodes))
	newIndex := make([]int32, len(nodes))
	for newPos, oldIdx := range order {
		sorted[newPos] = nodes[oldIdx]
		newIndex[oldIdx] = int32(newPos)
	}
	for i := range sorted {
		if sorted[i].parent >= 0 {
			sorted[i].parent = newIndex[sorted[i].parent]
		}
	}

	return Skeleton{nodes: sorted, pivots: m.Pivots(), globals: m.GlobalSequences()}
}

// WorldMatrices returns one world matrix per object id (len(Pivots())):
// identity for ids that aren't a bone or helper, else that node's local
// transform (built around its pivot:
// T(pivot)·T(translation)·R(rotation)·S(scale)·T(-pivot), each sampled at
// at) multiplied by its parent's world matrix (a root node's world matrix
// is just its local transform). Billboarded and "don't inherit" node flags
// are ignored; such nodes are posed as ordinary nodes.
func (s Skeleton) WorldMatrices(at AnimTime) []Mat4 {
	world := make([]Mat4, len(s.pivots))
	for i := range world {
		world[i] = identityMat4()
	}

	locals := make([]Mat4, len(s.nodes))
	for i, n := range s.nodes {
		var pivot [3]float32
		if int(n.objectID) < len(s.pivots) {
			pivot = s.pivots[n.objectID]
		}
		translation := sampleVec3(n.Translation, [3]float32{}, at, s.globals)
		rotation := sampleQuat(n.Rotation, [4]float32{0, 0, 0, 1}, at, s.globals)
		scale := sampleVec3(n.Scale, [3]float32{1, 1, 1}, at, s.globals)

		local := translationMat4(pivot).
			mul(translationMat4(translation)).
			mul(rotationMat4(rotation)).
			mul(scaleMat4(scale)).
			mul(translationMat4([3]float32{-pivot[0], -pivot[1], -pivot[2]}))
		locals[i] = local
	}

	nodeWorld := make([]Mat4, len(s.nodes))
	for i, n := range s.nodes {
		if n.parent < 0 {
			nodeWorld[i] = locals[i]
		} else {
			nodeWorld[i] = nodeWorld[n.parent].mul(locals[i])
		}
		if int(n.objectID) < len(world) {
			world[n.objectID] = nodeWorld[i]
		}
	}

	return world
}
