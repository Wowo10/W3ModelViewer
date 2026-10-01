package blte

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
)

var DEBUG = false

const (
	magic           = "BLTE"
	headerSizeStart = 4
	chunkInfoStart  = 12 // after magic, header size, flags and chunk count
	chunkInfoSize   = 24 // 4-byte compressed size + 4-byte decoded size + 16-byte MD5

	modeRaw  = 'N'
	modeZlib = 'Z'
)

type chunkInfo struct {
	compressedSize uint32
	decodedSize    uint32
	md5            [16]byte
}

// Decode unpacks a BLTE blob. With verify set, every chunk is checked against
// the MD5 stored in the chunk table.
func Decode(data []byte, verify bool) []byte {
	if string(data[:len(magic)]) != magic {
		panic(fmt.Sprintf("Bad BLTE magic: %q", data[:len(magic)]))
	}

	headerSize := binary.BigEndian.Uint32(data[headerSizeStart : headerSizeStart+4])

	// Without a chunk table the rest of the blob is a single chunk.
	if headerSize == 0 {
		return decodeChunk(data[headerSizeStart+4:])
	}

	chunks := readChunkTable(data)

	if DEBUG {
		fmt.Println("header size:", headerSize, "chunks:", len(chunks))
	}

	decodedSize := 0
	for _, chunk := range chunks {
		decodedSize += int(chunk.decodedSize)
	}

	out := make([]byte, 0, decodedSize)
	offset := int(headerSize)
	for _, chunk := range chunks {
		raw := data[offset : offset+int(chunk.compressedSize)]
		offset += int(chunk.compressedSize)

		if verify && md5.Sum(raw) != chunk.md5 {
			panic(fmt.Sprintf("Chunk MD5 mismatch at offset %d", offset-len(raw)))
		}

		out = append(out, decodeChunk(raw)...)
	}

	return out
}

func readChunkTable(data []byte) []chunkInfo {
	count := int(data[9])<<16 | int(data[10])<<8 | int(data[11])

	chunks := make([]chunkInfo, count)
	for i := range chunks {
		raw := data[chunkInfoStart+i*chunkInfoSize : chunkInfoStart+(i+1)*chunkInfoSize]
		chunks[i].compressedSize = binary.BigEndian.Uint32(raw[0:4])
		chunks[i].decodedSize = binary.BigEndian.Uint32(raw[4:8])
		copy(chunks[i].md5[:], raw[8:24])
	}

	return chunks
}

// decodeChunk decodes one chunk; the first byte selects the encoding mode.
func decodeChunk(raw []byte) []byte {
	mode, payload := raw[0], raw[1:]

	if DEBUG {
		fmt.Printf("mode: %c, size: %d\n", mode, len(payload))
	}

	switch mode {
	case modeRaw:
		return payload
	case modeZlib:
		r, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			panic(err)
		}
		defer r.Close()

		out, err := io.ReadAll(r)
		if err != nil {
			panic(err)
		}
		return out
	default:
		panic(fmt.Sprintf("Unsupported BLTE chunk mode: %c", mode))
	}
}
