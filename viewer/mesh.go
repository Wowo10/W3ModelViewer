package viewer

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.2-core/gl"
	"modelView/model/mdx"
)

const vertexShaderSrc = glslVersion + `
in vec3 position;
in vec3 normal;

uniform mat4 viewProj;
uniform mat4 view;

out vec3 vNormal;

void main() {
	vNormal = mat3(view) * normal;
	gl_Position = viewProj * vec4(position, 1.0);
}
` + "\x00"

const fragmentShaderSrc = glslVersion + `
in vec3 vNormal;
out vec4 fragColor;

void main() {
	vec3 color = vec3(0.75, 0.75, 0.78);

	float len = length(vNormal);
	float lit = 1.0;
	if (len > 0.0001) {
		vec3 n = vNormal / len;
		vec3 l = normalize(vec3(-0.4, -0.2, 0.9));
		lit = 0.25 + 0.75 * abs(dot(n, l));
	}

	fragColor = vec4(color * lit, 1.0);
}
` + "\x00"

const (
	positionAttrib = 0
	normalAttrib   = 1
)

// meshProgram is the single shader used to draw every geoset: untextured,
// normal-lit, flat grey.
type meshProgram struct {
	program         uint32
	viewProjUniform int32
	viewUniform     int32
}

// newMeshProgram compiles and links the mesh shader. GL must be current.
func newMeshProgram() *meshProgram {
	vs := compileShader(gl.VERTEX_SHADER, vertexShaderSrc)
	fs := compileShader(gl.FRAGMENT_SHADER, fragmentShaderSrc)

	program := gl.CreateProgram()
	gl.AttachShader(program, vs)
	gl.AttachShader(program, fs)
	gl.BindAttribLocation(program, positionAttrib, gl.Str("position\x00"))
	gl.BindAttribLocation(program, normalAttrib, gl.Str("normal\x00"))
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetProgramInfoLog(program, logLen, nil, gl.Str(log))
		panic(fmt.Sprintf("viewer: link mesh program: %s", log))
	}

	gl.DeleteShader(vs)
	gl.DeleteShader(fs)

	return &meshProgram{
		program:         program,
		viewProjUniform: gl.GetUniformLocation(program, gl.Str("viewProj\x00")),
		viewUniform:     gl.GetUniformLocation(program, gl.Str("view\x00")),
	}
}

// release deletes the GL program.
func (p *meshProgram) release() {
	gl.DeleteProgram(p.program)
	p.program = 0
}

func compileShader(xtype uint32, src string) uint32 {
	shader := gl.CreateShader(xtype)
	csrc, free := gl.Strs(src)
	gl.ShaderSource(shader, 1, csrc, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLen int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLen)
		log := strings.Repeat("\x00", int(logLen+1))
		gl.GetShaderInfoLog(shader, logLen, nil, gl.Str(log))
		panic(fmt.Sprintf("viewer: compile shader: %s", log))
	}
	return shader
}

// gpuGeoset is one geoset's VAO/VBO/IBO.
type gpuGeoset struct {
	vao, vbo, ibo uint32
	count         int32
}

// modelMesh is the GPU-side form of a model's geosets at one LOD, plus the
// bounds of what was uploaded.
type modelMesh struct {
	geosets  []gpuGeoset
	min, max [3]float32
	lod      uint32
}

// lowestLOD returns the lowest LOD value present among geosets, or 0 if
// there are none.
func lowestLOD(geosets []mdx.Geoset) uint32 {
	if len(geosets) == 0 {
		return 0
	}
	lod := geosets[0].LOD
	for _, g := range geosets[1:] {
		if g.LOD < lod {
			lod = g.LOD
		}
	}
	return lod
}

// uploadMesh uploads the geosets at the model's lowest LOD whose alpha is >
// 0 to VAO/VBO/IBO pairs, interleaving position and normal (zero-filled
// when the geoset has no normals) into a single vertex buffer per geoset.
// lowestLOD is computed over every geoset, visible or not, so hiding
// geosets never changes which LOD is rendered. GL must be current.
func uploadMesh(geosets []mdx.Geoset, alphas []float32) modelMesh {
	lod := lowestLOD(geosets)

	mesh := modelMesh{lod: lod}
	first := true

	for i, g := range geosets {
		if g.LOD != lod || len(g.Indices) == 0 || alphas[i] <= 0 {
			continue
		}

		vertexCount := g.VertexCount()
		vertices := make([]float32, 0, vertexCount*6)
		hasNormals := len(g.Normals) == len(g.Positions)
		for i := 0; i < vertexCount; i++ {
			px, py, pz := g.Positions[i*3], g.Positions[i*3+1], g.Positions[i*3+2]
			vertices = append(vertices, px, py, pz)
			if hasNormals {
				vertices = append(vertices, g.Normals[i*3], g.Normals[i*3+1], g.Normals[i*3+2])
			} else {
				vertices = append(vertices, 0, 0, 0)
			}

			p := [3]float32{px, py, pz}
			if first {
				mesh.min, mesh.max = p, p
				first = false
			} else {
				for k := 0; k < 3; k++ {
					if p[k] < mesh.min[k] {
						mesh.min[k] = p[k]
					}
					if p[k] > mesh.max[k] {
						mesh.max[k] = p[k]
					}
				}
			}
		}

		var gg gpuGeoset
		gl.GenVertexArrays(1, &gg.vao)
		gl.BindVertexArray(gg.vao)

		gl.GenBuffers(1, &gg.vbo)
		gl.BindBuffer(gl.ARRAY_BUFFER, gg.vbo)
		gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

		gl.GenBuffers(1, &gg.ibo)
		gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, gg.ibo)
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(g.Indices)*2, gl.Ptr(g.Indices), gl.STATIC_DRAW)

		gl.VertexAttribPointerWithOffset(positionAttrib, 3, gl.FLOAT, false, 24, 0)
		gl.EnableVertexAttribArray(positionAttrib)
		gl.VertexAttribPointerWithOffset(normalAttrib, 3, gl.FLOAT, false, 24, 12)
		gl.EnableVertexAttribArray(normalAttrib)

		gl.BindVertexArray(0)

		gg.count = int32(len(g.Indices))
		mesh.geosets = append(mesh.geosets, gg)
	}

	return mesh
}

// release deletes every geoset's VAO/VBO/IBO and empties the mesh.
func (m *modelMesh) release() {
	for _, g := range m.geosets {
		gl.DeleteBuffers(1, &g.vbo)
		gl.DeleteBuffers(1, &g.ibo)
		gl.DeleteVertexArrays(1, &g.vao)
	}
	m.geosets = nil
}

// draw renders every geoset with prog, under view and proj.
func (m *modelMesh) draw(prog *meshProgram, view, proj mat4) {
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)

	gl.UseProgram(prog.program)
	viewProj := proj.mul(view)
	gl.UniformMatrix4fv(prog.viewProjUniform, 1, false, &viewProj[0])
	gl.UniformMatrix4fv(prog.viewUniform, 1, false, &view[0])

	for _, g := range m.geosets {
		gl.BindVertexArray(g.vao)
		gl.DrawElements(gl.TRIANGLES, g.count, gl.UNSIGNED_SHORT, nil)
	}
	gl.BindVertexArray(0)
}
