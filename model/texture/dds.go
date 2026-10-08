package texture

import (
	"encoding/binary"
	"fmt"
)

// Format is a DDS pixel format ReadDDS recognizes.
type Format string

const (
	DXT1 Format = "DXT1"
	DXT5 Format = "DXT5"
	ATI2 Format = "ATI2"
)

// blockBytes is the compressed block size in bytes for each Format.
var blockBytes = map[Format]int{
	DXT1: 8,
	DXT5: 16,
	ATI2: 16,
}

// Mip is one mip level of a DDS texture. Offset and Size index the data
// ReadDDS was given, so callers slice data[m.Offset : m.Offset+m.Size].
type Mip struct {
	Offset, Size  int
	Width, Height int
}

// DDS is a parsed DDS header plus its mip layout.
type DDS struct {
	Format        Format
	Width, Height int
	Mips          []Mip
}

const (
	headerSize      = 128
	ddsdMipmapCount = 0x20000 // flags bit: dwMipMapCount is valid
	ddpfFourCC      = 0x4     // pixel format flags bit: dwFourCC is valid
)

// ReadDDS parses a DDS header and lays out its mips. wardata's model
// textures (see roadmap/04-tex/EPIC.md) are all plain "DDS " headers (no
// DX10 header), DXT1, DXT5 or ATI2, with full mip chains down to 1x1; every
// one has file size exactly 128 plus the sum of its mips' block-aligned
// sizes.
//
// ReadDDS panics ("texture: dds: ...") when: data is too short or lacks the
// "DDS " magic, the header size isn't 124, the pixel format doesn't have
// the FourCC bit set, the FourCC isn't DXT1, DXT5 or ATI2 (including
// DX10), caps2 isn't 0 (a cube map or volume texture), width or height is
// 0, or a mip runs past the end of data. Trailing bytes after the last mip
// are ignored. The sRGB and normal-map bits NVTT sets in the pixel format
// flags aren't exposed here; see TEX-03.
func ReadDDS(data []byte) DDS {
	if len(data) < headerSize || string(data[0:4]) != "DDS " {
		panic("texture: dds: too short or missing \"DDS \" magic")
	}

	if size := binary.LittleEndian.Uint32(data[4:8]); size != 124 {
		panic(fmt.Sprintf("texture: dds: header size %d, want 124", size))
	}

	flags := binary.LittleEndian.Uint32(data[8:12])
	height := int(binary.LittleEndian.Uint32(data[12:16]))
	width := int(binary.LittleEndian.Uint32(data[16:20]))
	mipCount := int(binary.LittleEndian.Uint32(data[28:32]))
	pfFlags := binary.LittleEndian.Uint32(data[80:84])
	fourCC := string(data[84:88])
	caps2 := binary.LittleEndian.Uint32(data[112:116])

	if width == 0 || height == 0 {
		panic(fmt.Sprintf("texture: dds: zero dimension %dx%d", width, height))
	}
	if caps2 != 0 {
		panic(fmt.Sprintf("texture: dds: caps2 0x%x, cube maps and volumes aren't supported", caps2))
	}
	if pfFlags&ddpfFourCC == 0 {
		panic(fmt.Sprintf("texture: dds: pixel format flags 0x%x, want the FourCC bit set", pfFlags))
	}

	var format Format
	switch fourCC {
	case "DXT1":
		format = DXT1
	case "DXT5":
		format = DXT5
	case "ATI2":
		format = ATI2
	default:
		panic(fmt.Sprintf("texture: dds: unsupported FourCC %q", fourCC))
	}

	if flags&ddsdMipmapCount == 0 || mipCount == 0 {
		mipCount = 1
	}

	bytesPerBlock := blockBytes[format]
	mips := make([]Mip, 0, mipCount)
	offset := headerSize
	w, h := width, height
	for i := 0; i < mipCount; i++ {
		blocksWide := max(1, (w+3)/4)
		blocksHigh := max(1, (h+3)/4)
		mipSize := blocksWide * blocksHigh * bytesPerBlock

		if offset+mipSize > len(data) {
			panic(fmt.Sprintf("texture: dds: mip %d (%d bytes at offset %d) runs past end of data (%d bytes)", i, mipSize, offset, len(data)))
		}

		mips = append(mips, Mip{Offset: offset, Size: mipSize, Width: w, Height: h})
		offset += mipSize

		w, h = max(1, w/2), max(1, h/2)
	}

	return DDS{Format: format, Width: width, Height: height, Mips: mips}
}
