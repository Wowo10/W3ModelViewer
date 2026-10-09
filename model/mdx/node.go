package mdx

import (
	"fmt"
	"strings"
)

// Node is the common header of a BONE or HELP entry: its name, id, parent
// and tracks. Translation, Rotation and Scale are nil when the node has no
// such track.
type Node struct {
	Name     string
	ObjectID uint32
	ParentID int32 // -1 for a root node
	Flags    uint32

	Translation *Track[[3]float32] // from a KGTR track
	Rotation    *Track[[4]float32] // from a KGRT track
	Scale       *Track[[3]float32] // from a KGSC track
}

// Bone is one entry of a BONE chunk: a Node plus its geoset and geoset
// animation id.
type Bone struct {
	Node
	GeosetID     int32 // -1 when not tied to a geoset
	GeosetAnimID int32 // -1 when none
}

// Bones parses the model's BONE chunk, if any, into bones. It returns nil
// when there is no BONE chunk.
//
// BONE layout in version 1800 (the only version seen in wardata): entries
// packed back to back, each a Node (see readNode) followed immediately by
// int32 geoset id and int32 geoset anim id (-1 when absent in either case;
// both are outside the node's own inclusive size).
//
// Bones panics ("mdx: BONE ...") on the errors documented by readNode, on
// truncated geoset/geoset-anim ids, and with "mdx: BONE parsing supports
// version 1800 only, got %d" on a BONE chunk in any other version.
func (m Model) Bones() []Bone {
	var boneChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "BONE" {
			boneChunk = &m.Chunks[i]
			break
		}
	}
	if boneChunk == nil {
		return nil
	}
	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: BONE parsing supports version 1800 only, got %d", m.Version))
	}

	data := boneChunk.Data
	var bones []Bone
	pos := 0
	for pos < len(data) {
		node, end := readNode(data, pos, "BONE", len(bones))

		r := reader{data: data, pos: end, limit: len(data), where: fmt.Sprintf("BONE node %d", len(bones))}
		bone := Bone{Node: node, GeosetID: r.i32(), GeosetAnimID: r.i32()}
		bones = append(bones, bone)
		pos = r.pos
	}

	return bones
}

// Helpers parses the model's HELP chunk, if any, into nodes. It returns nil
// when there is no HELP chunk.
//
// HELP layout in version 1800 (the only version seen in wardata): entries
// packed back to back, each just a Node (see readNode).
//
// Helpers panics ("mdx: HELP ...") on the errors documented by readNode, and
// with "mdx: HELP parsing supports version 1800 only, got %d" on a HELP
// chunk in any other version.
func (m Model) Helpers() []Node {
	var helpChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "HELP" {
			helpChunk = &m.Chunks[i]
			break
		}
	}
	if helpChunk == nil {
		return nil
	}
	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: HELP parsing supports version 1800 only, got %d", m.Version))
	}

	data := helpChunk.Data
	var nodes []Node
	pos := 0
	for pos < len(data) {
		node, end := readNode(data, pos, "HELP", len(nodes))
		nodes = append(nodes, node)
		pos = end
	}

	return nodes
}

// readNode parses one node starting at pos in data, inside a chunk tagged
// kind ("BONE" or "HELP"), and returns it along with its end offset (the
// inclusive size field) — the node's own size covers the header and tracks
// only, not any type-specific fields a caller reads after it (e.g. BONE's
// geoset ids).
//
// Node layout: uint32 inclusive size, char[80] name (NUL-padded), uint32
// object id, int32 parent id (-1 for a root node), uint32 flags (96 B
// total), then tracks to the node's end. A track is a 4-byte tag, uint32
// key count, uint32 interpolation, int32 global sequence id, then per key:
// int32 frame, value, plus an in- and out-tangent when interpolation > 1
// (the GEOA track layout; see readTrack). Three tags are seen: KGTR
// (translation, width 3), KGRT (rotation, width 4, quaternion x y z w),
// KGSC (scale, width 3). A duplicate track tag within one node is not
// checked for; the last one read wins.
//
// readNode panics ("mdx: %s node %d: ...") on truncated data, a node
// running past the chunk end, a size smaller than the 96-byte header, or an
// unknown track tag.
func readNode(data []byte, start int, kind string, index int) (Node, int) {
	r := reader{data: data, pos: start, limit: len(data), where: fmt.Sprintf("%s node %d", kind, index)}

	size := r.u32()
	end := start + int(size)
	if uint64(end) > uint64(len(data)) {
		panic(fmt.Sprintf("mdx: %s node %d: runs past chunk end", kind, index))
	}
	if size < 96 {
		panic(fmt.Sprintf("mdx: %s node %d: size %d smaller than header", kind, index, size))
	}
	r.limit = end

	r.need(80)
	name := string(r.data[r.pos : r.pos+80])
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	r.pos += 80

	node := Node{Name: name}
	node.ObjectID = r.u32()
	node.ParentID = r.i32()
	node.Flags = r.u32()

	for r.pos < r.limit {
		tag := r.tag()
		switch tag {
		case "KGTR":
			t := readVec3Track(&r)
			node.Translation = &t
		case "KGRT":
			t := readQuatTrack(&r)
			node.Rotation = &t
		case "KGSC":
			t := readVec3Track(&r)
			node.Scale = &t
		default:
			panic(fmt.Sprintf("mdx: %s node %d: unknown track %q", kind, index, tag))
		}
	}

	return node, end
}
