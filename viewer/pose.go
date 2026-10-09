package viewer

import (
	"unsafe"

	"github.com/go-gl/gl/v3.2-core/gl"
	"modelView/model/mdx"
)

// poseBuffer holds a model's current pose (one node world matrix per
// object id) in a texture buffer, indexed by object id directly: 4 RGBA32F
// texels (one per column) per matrix, in mdx.Mat4's column-major order.
// Per-model object id counts exceed GL 3.2's guaranteed uniform space (see
// ANIM-05's Context), hence the texture buffer instead of a uniform array.
type poseBuffer struct {
	buffer, texture uint32
	pose            int // the model.pose last uploaded, -1 before the first upload
}

// newPoseBuffer creates the GL buffer and its texture buffer view. GL must
// be current.
func newPoseBuffer() *poseBuffer {
	pb := &poseBuffer{pose: -1}
	gl.GenBuffers(1, &pb.buffer)
	gl.GenTextures(1, &pb.texture)
	return pb
}

// upload re-fills the buffer with matrices and (re)attaches it to the
// texture buffer view. matrices is empty before any model is loaded, or for
// a model with no bones/helpers/pivots.
func (pb *poseBuffer) upload(matrices []mdx.Mat4) {
	var data unsafe.Pointer
	if len(matrices) > 0 {
		data = gl.Ptr(matrices)
	}
	gl.BindBuffer(gl.TEXTURE_BUFFER, pb.buffer)
	gl.BufferData(gl.TEXTURE_BUFFER, len(matrices)*16*4, data, gl.DYNAMIC_DRAW)
	gl.BindTexture(gl.TEXTURE_BUFFER, pb.texture)
	gl.TexBuffer(gl.TEXTURE_BUFFER, gl.RGBA32F, pb.buffer)
}

// release deletes the GL buffer and texture.
func (pb *poseBuffer) release() {
	gl.DeleteBuffers(1, &pb.buffer)
	gl.DeleteTextures(1, &pb.texture)
}
