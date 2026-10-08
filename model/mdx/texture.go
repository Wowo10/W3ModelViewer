package mdx

import (
	"fmt"
	"strings"
)

// Texture is one entry of a TEXS chunk.
type Texture struct {
	ReplaceableID uint32
	Path          string // as stored (backslashes, original extension); empty for most replaceable textures
	Flags         uint32
}

// Textures parses the model's TEXS chunk, if any, into textures. It returns
// nil when there is no TEXS chunk.
//
// TEXS layout in version 1800 (the only version seen in wardata): entries
// packed back to back, each 268 bytes: uint32 replaceable id, char[260]
// path (NUL-padded), uint32 flags (bit 0 wrap width, bit 1 wrap height).
//
// Textures panics ("mdx: TEXS: ...") when the chunk size isn't a multiple
// of 268, or on a version other than 1800.
func (m Model) Textures() []Texture {
	var texsChunk *Chunk
	for i := range m.Chunks {
		if m.Chunks[i].Tag == "TEXS" {
			texsChunk = &m.Chunks[i]
			break
		}
	}
	if texsChunk == nil {
		return nil
	}

	if m.Version != 1800 {
		panic(fmt.Sprintf("mdx: TEXS parsing supports version 1800 only, got %d", m.Version))
	}

	const entrySize = 268

	data := texsChunk.Data
	if len(data)%entrySize != 0 {
		panic(fmt.Sprintf("mdx: TEXS: chunk size %d not a multiple of %d", len(data), entrySize))
	}

	var texs []Texture
	for pos := 0; pos < len(data); pos += entrySize {
		r := reader{data: data, pos: pos, limit: pos + entrySize, where: fmt.Sprintf("TEXS texture %d", len(texs))}

		t := Texture{ReplaceableID: r.u32()}

		r.need(260)
		path := string(r.data[r.pos : r.pos+260])
		if i := strings.IndexByte(path, 0); i >= 0 {
			path = path[:i]
		}
		r.pos += 260
		t.Path = path

		t.Flags = r.u32()
		texs = append(texs, t)
	}

	return texs
}
