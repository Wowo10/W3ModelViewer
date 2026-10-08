package viewer

import (
	"fmt"

	"github.com/go-gl/gl/v3.2-core/gl"
	"modelView/model/texture"
)

// compressedFormat maps a DDS Format to its GL internal format. Non-sRGB on
// purpose: the default framebuffer isn't sRGB, so sampling the stored values
// raw shows them as authored.
var compressedFormat = map[texture.Format]uint32{
	texture.DXT1: gl.COMPRESSED_RGBA_S3TC_DXT1_EXT,
	texture.DXT5: gl.COMPRESSED_RGBA_S3TC_DXT5_EXT,
	texture.ATI2: gl.COMPRESSED_RG_RGTC2,
}

// uploadTexture uploads dds's mips (sliced from data) as a compressed 2D GL
// texture with trilinear filtering and returns its name. GL must be current.
func uploadTexture(dds texture.DDS, data []byte) uint32 {
	internalFormat, ok := compressedFormat[dds.Format]
	if !ok {
		panic(fmt.Sprintf("viewer: unknown DDS format %q", dds.Format))
	}

	var tex uint32
	gl.GenTextures(1, &tex)
	gl.BindTexture(gl.TEXTURE_2D, tex)

	for level, m := range dds.Mips {
		gl.CompressedTexImage2D(gl.TEXTURE_2D, int32(level), internalFormat, int32(m.Width), int32(m.Height), 0, int32(m.Size), gl.Ptr(data[m.Offset:m.Offset+m.Size]))
	}

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_BASE_LEVEL, 0)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAX_LEVEL, int32(len(dds.Mips)-1))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)

	gl.BindTexture(gl.TEXTURE_2D, 0)
	return tex
}
