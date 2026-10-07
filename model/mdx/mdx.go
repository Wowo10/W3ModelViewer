// Package mdx reads the chunk structure of an MDX model file.
//
// An MDX file starts with the 4-byte magic "MDLX", followed by chunks packed
// back to back to EOF: a 4-byte ASCII tag, a uint32 LE size, then that many
// bytes of chunk data. The first chunk is always "VERS", a 4-byte uint32 LE
// version number.
package mdx

import (
	"encoding/binary"
	"fmt"
)

const (
	magic        = "MDLX"
	tagSize      = 4
	sizeFieldLen = 4
	headerLen    = tagSize + sizeFieldLen
)

// Chunk is one chunk of an MDX file.
type Chunk struct {
	Tag    string // 4-byte ASCII tag, e.g. "GEOS"
	Offset int    // where Data starts in the file
	Data   []byte // chunk data, sliced from the input (no copy)
}

// Model is the chunk structure of an MDX file.
type Model struct {
	Version uint32
	Chunks  []Chunk
}

// Read parses the MDX chunk list from data. It panics on a bad magic, a
// truncated chunk header or body, a VERS chunk that isn't 4 bytes, or a
// missing VERS chunk.
func Read(data []byte) Model {
	if len(data) < len(magic) || string(data[:len(magic)]) != magic {
		panic(fmt.Sprintf("mdx: bad magic %q", peek(data, len(magic))))
	}

	m := Model{}
	pos := len(magic)

	for pos < len(data) {
		if len(data)-pos < headerLen {
			panic(fmt.Sprintf("mdx: truncated chunk header at offset %d", pos))
		}

		tag := string(data[pos : pos+tagSize])
		size := binary.LittleEndian.Uint32(data[pos+tagSize : pos+headerLen])
		dataStart := pos + headerLen

		if uint64(dataStart)+uint64(size) > uint64(len(data)) {
			panic(fmt.Sprintf("mdx: chunk %q at offset %d runs past EOF", tag, pos))
		}

		chunk := Chunk{
			Tag:    tag,
			Offset: dataStart,
			Data:   data[dataStart : dataStart+int(size)],
		}
		m.Chunks = append(m.Chunks, chunk)

		if tag == "VERS" {
			if len(chunk.Data) != 4 {
				panic(fmt.Sprintf("mdx: VERS chunk is %d bytes, want 4", len(chunk.Data)))
			}
			m.Version = binary.LittleEndian.Uint32(chunk.Data)
		}

		pos = dataStart + int(size)
	}

	if len(m.Chunks) == 0 || m.Chunks[0].Tag != "VERS" {
		panic("mdx: no VERS chunk")
	}

	return m
}

// peek returns up to n bytes of data for an error message.
func peek(data []byte, n int) []byte {
	if len(data) < n {
		return data
	}
	return data[:n]
}
