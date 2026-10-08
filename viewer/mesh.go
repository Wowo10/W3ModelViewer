package viewer

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.2-core/gl"
	"modelView/extract/casc"
	"modelView/model/mdx"
	"modelView/model/texture"
)

const vertexShaderSrc = glslVersion + `
in vec3 position;
in vec3 normal;
in vec2 uv;

uniform mat4 viewProj;
uniform mat4 view;

out vec3 vNormal;
out vec2 vUV;

void main() {
	vNormal = mat3(view) * normal;
	vUV = uv;
	gl_Position = viewProj * vec4(position, 1.0);
}
` + "\x00"

const fragmentShaderSrc = glslVersion + `
in vec3 vNormal;
in vec2 vUV;
out vec4 fragColor;

uniform sampler2D diffuseMap;
uniform bool textured;

void main() {
	vec3 color = textured ? texture(diffuseMap, vUV).rgb : vec3(0.75, 0.75, 0.78);

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
	uvAttrib       = 2
)

// meshProgram is the single shader used to draw every geoset: flat grey or
// its diffuse texture, normal-lit.
type meshProgram struct {
	program         uint32
	viewProjUniform int32
	viewUniform     int32
	texturedUniform int32
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
	gl.BindAttribLocation(program, uvAttrib, gl.Str("uv\x00"))
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

	gl.UseProgram(program)
	gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("diffuseMap\x00")), 0)
	gl.UseProgram(0)

	return &meshProgram{
		program:         program,
		viewProjUniform: gl.GetUniformLocation(program, gl.Str("viewProj\x00")),
		viewUniform:     gl.GetUniformLocation(program, gl.Str("view\x00")),
		texturedUniform: gl.GetUniformLocation(program, gl.Str("textured\x00")),
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

// gpuGeoset is one geoset's VAO/VBO/IBO, plus its diffuse texture (0 = grey)
// and wrap mode.
type gpuGeoset struct {
	vao, vbo, ibo uint32
	count         int32
	texture       uint32
	wrapS, wrapT  int32
}

// modelMesh is the GPU-side form of a model's geosets at one LOD, plus the
// bounds of what was uploaded. textures holds one GL texture per resolved
// diffuse content key, shared across geosets and deleted with the mesh.
type modelMesh struct {
	geosets  []gpuGeoset
	min, max [3]float32
	lod      uint32
	textures map[[16]byte]uint32
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
// 0 to VAO/VBO/IBO pairs, interleaving position, normal (zero-filled when
// the geoset has no normals) and UV (diffuse's uvSet when the geoset has
// that set, else set 0, else zeros) into a single vertex buffer per geoset.
// lowestLOD is computed over every geoset, visible or not, so hiding
// geosets never changes which LOD is rendered. For each uploaded geoset
// whose diffuse resolved, its DDS is read through storage and uploaded once
// per content key, shared across geosets. GL must be current.
func uploadMesh(storage *casc.Storage, geosets []mdx.Geoset, alphas []float32, diffuse []geosetDiffuse) modelMesh {
	lod := lowestLOD(geosets)

	mesh := modelMesh{lod: lod, textures: map[[16]byte]uint32{}}
	first := true

	for i, g := range geosets {
		if g.LOD != lod || len(g.Indices) == 0 || alphas[i] <= 0 {
			continue
		}

		d := diffuse[i]
		uvSet := 0
		switch {
		case d.ok && d.uvSet < len(g.UVs):
			uvSet = d.uvSet
		case len(g.UVs) == 0:
			uvSet = -1
		}

		vertexCount := g.VertexCount()
		vertices := make([]float32, 0, vertexCount*8)
		hasNormals := len(g.Normals) == len(g.Positions)
		for i := 0; i < vertexCount; i++ {
			px, py, pz := g.Positions[i*3], g.Positions[i*3+1], g.Positions[i*3+2]
			vertices = append(vertices, px, py, pz)
			if hasNormals {
				vertices = append(vertices, g.Normals[i*3], g.Normals[i*3+1], g.Normals[i*3+2])
			} else {
				vertices = append(vertices, 0, 0, 0)
			}
			if uvSet >= 0 {
				vertices = append(vertices, g.UVs[uvSet][i*2], g.UVs[uvSet][i*2+1])
			} else {
				vertices = append(vertices, 0, 0)
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

		gl.VertexAttribPointerWithOffset(positionAttrib, 3, gl.FLOAT, false, 32, 0)
		gl.EnableVertexAttribArray(positionAttrib)
		gl.VertexAttribPointerWithOffset(normalAttrib, 3, gl.FLOAT, false, 32, 12)
		gl.EnableVertexAttribArray(normalAttrib)
		gl.VertexAttribPointerWithOffset(uvAttrib, 2, gl.FLOAT, false, 32, 24)
		gl.EnableVertexAttribArray(uvAttrib)

		gl.BindVertexArray(0)

		gg.count = int32(len(g.Indices))

		if d.ok {
			tex, ok := mesh.textures[d.entry.CKey]
			if !ok {
				data := storage.ReadByCKey(d.entry.CKey)
				tex = uploadTexture(texture.ReadDDS(data), data)
				mesh.textures[d.entry.CKey] = tex
			}
			gg.texture = tex
			if d.flags&1 != 0 {
				gg.wrapS = gl.REPEAT
			} else {
				gg.wrapS = gl.CLAMP_TO_EDGE
			}
			if d.flags&2 != 0 {
				gg.wrapT = gl.REPEAT
			} else {
				gg.wrapT = gl.CLAMP_TO_EDGE
			}
		}

		mesh.geosets = append(mesh.geosets, gg)
	}

	return mesh
}

// release deletes every geoset's VAO/VBO/IBO, every uploaded texture, and
// empties the mesh.
func (m *modelMesh) release() {
	for _, g := range m.geosets {
		gl.DeleteBuffers(1, &g.vbo)
		gl.DeleteBuffers(1, &g.ibo)
		gl.DeleteVertexArrays(1, &g.vao)
	}
	for _, tex := range m.textures {
		gl.DeleteTextures(1, &tex)
	}
	m.geosets = nil
	m.textures = nil
}

// draw renders every geoset with prog, under view and proj: textured when
// it has a diffuse texture, else flat grey.
func (m *modelMesh) draw(prog *meshProgram, view, proj mat4) {
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)

	gl.UseProgram(prog.program)
	viewProj := proj.mul(view)
	gl.UniformMatrix4fv(prog.viewProjUniform, 1, false, &viewProj[0])
	gl.UniformMatrix4fv(prog.viewUniform, 1, false, &view[0])

	gl.ActiveTexture(gl.TEXTURE0)
	for _, g := range m.geosets {
		if g.texture != 0 {
			gl.BindTexture(gl.TEXTURE_2D, g.texture)
			gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, g.wrapS)
			gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, g.wrapT)
			gl.Uniform1i(prog.texturedUniform, 1)
		} else {
			gl.Uniform1i(prog.texturedUniform, 0)
		}

		gl.BindVertexArray(g.vao)
		gl.DrawElements(gl.TRIANGLES, g.count, gl.UNSIGNED_SHORT, nil)
	}
	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
}
