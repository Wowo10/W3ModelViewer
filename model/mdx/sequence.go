package mdx

import (
	"fmt"
	"strings"
)

// Sequence is one entry of a SEQS chunk.
type Sequence struct {
	Name       string
	Start, End uint32
}

// Sequences parses the model's SEQS chunk, if any, into sequences. It
// returns nil when there is no SEQS chunk.
//
// SEQS layout in version 1800 (the only version seen in wardata): entries
// packed back to back, each 132 bytes: char[80] name (NUL-padded), uint32
// start, uint32 end, then 44 B skipped (move speed, flags, rarity, sync
// point, 28 B extent).
//
// Sequences panics ("mdx: SEQS: ...") when the chunk size isn't a multiple
// of 132, or on a version other than 1800.
func (m Model) Sequences() []Sequence {
	var seqsChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "SEQS" {
			seqsChunk = &m.Chunks[i]
			break
		}
	}
	if seqsChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: SEQS parsing supports version 1800 only, got %d", m.Version))
	}

	const entrySize = 132

	data := seqsChunk.Data
	if len(data)%entrySize != 0 {
		panic(fmt.Sprintf("mdx: SEQS: chunk size %d not a multiple of %d", len(data), entrySize))
	}

	var seqs []Sequence
	for pos := 0; pos < len(data); pos += entrySize {
		r := reader{data: data, pos: pos, limit: pos + entrySize, where: fmt.Sprintf("SEQS sequence %d", len(seqs))}

		r.need(80)
		name := string(r.data[r.pos : r.pos+80])
		if i := strings.IndexByte(name, 0); i >= 0 {
			name = name[:i]
		}
		r.pos += 80

		seqs = append(seqs, Sequence{
			Name:  name,
			Start: r.u32(),
			End:   r.u32(),
		})
	}

	return seqs
}

// DefaultSequence returns the first sequence whose name starts with "stand"
// (case-insensitive), else the first sequence. It returns false when seqs is
// empty.
func DefaultSequence(seqs []Sequence) (Sequence, bool) {
	if len(seqs) == 0 {
		return Sequence{}, false
	}
	for _, s := range seqs {
		if strings.HasPrefix(strings.ToLower(s.Name), "stand") {
			return s, true
		}
	}
	return seqs[0], true
}
