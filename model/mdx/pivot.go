package mdx

import "fmt"

// Pivots parses the model's PIVT chunk, if any, into pivot points, indexed
// by object id (matching BONE/HELP entries' ObjectID). It returns nil when
// there is no PIVT chunk.
//
// PIVT layout in version 1800 (the only version seen in wardata): one
// float32[3] per object id, packed back to back.
//
// Pivots panics ("mdx: PIVT: ...") when the chunk size isn't a multiple of
// 12, or on a version other than 1800.
func (m Model) Pivots() [][3]float32 {
	var pivtChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "PIVT" {
			pivtChunk = &m.Chunks[i]
			break
		}
	}
	if pivtChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: PIVT parsing supports version 1800 only, got %d", m.Version))
	}

	const entrySize = 12

	data := pivtChunk.Data
	if len(data)%entrySize != 0 {
		panic(fmt.Sprintf("mdx: PIVT: chunk size %d not a multiple of %d", len(data), entrySize))
	}

	pivots := make([][3]float32, len(data)/entrySize)
	r := reader{data: data, pos: 0, limit: len(data), where: "PIVT"}
	for i := range pivots {
		pivots[i] = [3]float32{r.f32(), r.f32(), r.f32()}
	}

	return pivots
}

// GlobalSequences parses the model's GLBS chunk, if any, into global
// sequence lengths, indexed by global sequence id (matching a track's
// GlobalSequence field). It returns nil when there is no GLBS chunk.
//
// GLBS layout in version 1800 (the only version seen in wardata): one
// uint32 length per global sequence, packed back to back.
//
// GlobalSequences panics ("mdx: GLBS: ...") when the chunk size isn't a
// multiple of 4, or on a version other than 1800.
func (m Model) GlobalSequences() []uint32 {
	var glbsChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "GLBS" {
			glbsChunk = &m.Chunks[i]
			break
		}
	}
	if glbsChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: GLBS parsing supports version 1800 only, got %d", m.Version))
	}

	const entrySize = 4

	data := glbsChunk.Data
	if len(data)%entrySize != 0 {
		panic(fmt.Sprintf("mdx: GLBS: chunk size %d not a multiple of %d", len(data), entrySize))
	}

	seqs := make([]uint32, len(data)/entrySize)
	r := reader{data: data, pos: 0, limit: len(data), where: "GLBS"}
	for i := range seqs {
		seqs[i] = r.u32()
	}

	return seqs
}
