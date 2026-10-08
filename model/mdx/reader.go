package mdx

import (
	"encoding/binary"
	"fmt"
	"math"
)

// reader is a bounds-checked cursor over a chunk's bytes, used by parsers
// other than Geosets (which keeps its own geosReader and panic messages).
type reader struct {
	data  []byte
	pos   int
	limit int
	where string // used in panic messages, e.g. "SEQS sequence 3"
}

func (r *reader) need(n int) {
	if n < 0 || uint64(r.pos)+uint64(n) > uint64(r.limit) {
		panic(fmt.Sprintf("mdx: %s: truncated at offset %d", r.where, r.pos))
	}
}

func (r *reader) u32() uint32 {
	r.need(4)
	v := binary.LittleEndian.Uint32(r.data[r.pos : r.pos+4])
	r.pos += 4
	return v
}

func (r *reader) i32() int32 {
	return int32(r.u32())
}

func (r *reader) f32() float32 {
	return math.Float32frombits(r.u32())
}

// tag reads a 4-byte ASCII tag.
func (r *reader) tag() string {
	r.need(4)
	s := string(r.data[r.pos : r.pos+4])
	r.pos += 4
	return s
}

// peekTag returns the next 4-byte tag without advancing.
func (r *reader) peekTag() string {
	r.need(4)
	return string(r.data[r.pos : r.pos+4])
}

func (r *reader) skip(n int) {
	r.need(n)
	r.pos += n
}
