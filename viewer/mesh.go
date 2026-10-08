package viewer

import (
	"fmt"
	"sort"
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
uniform sampler2D ormMap;
uniform sampler2D teamColorMap;
uniform bool textured;
uniform bool alphaTest;
uniform bool unshaded;
uniform bool teamMasked;
uniform float alpha;

void main() {
	vec4 tex = textured ? texture(diffuseMap, vUV) : vec4(0.75, 0.75, 0.78, 1.0);

	if (alphaTest && tex.a < 0.75) {
		discard;
	}

	vec3 rgb = tex.rgb;
	if (teamMasked) {
		rgb = mix(rgb, rgb * texture(teamColorMap, vUV).rgb, texture(ormMap, vUV).a);
	}

	float lit = 1.0;
	if (!unshaded) {
		float len = length(vNormal);
		if (len > 0.0001) {
			vec3 n = vNormal / len;
			vec3 l = normalize(vec3(-0.4, -0.2, 0.9));
			lit = 0.25 + 0.75 * abs(dot(n, l));
		}
	}

	fragColor = vec4(rgb * lit, tex.a * alpha);
}
` + "\x00"

const (
	positionAttrib = 0
	normalAttrib   = 1
	uvAttrib       = 2
)

// meshProgram is the single shader used to draw every geoset layer: flat
// grey or its texture, normal-lit unless unshaded, alpha-tested and/or
// blended per its filter mode.
type meshProgram struct {
	program           uint32
	viewProjUniform   int32
	viewUniform       int32
	texturedUniform   int32
	alphaTestUniform  int32
	unshadedUniform   int32
	alphaUniform      int32
	teamMaskedUniform int32
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
	gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("ormMap\x00")), 1)
	gl.Uniform1i(gl.GetUniformLocation(program, gl.Str("teamColorMap\x00")), 2)
	gl.UseProgram(0)

	return &meshProgram{
		program:           program,
		viewProjUniform:   gl.GetUniformLocation(program, gl.Str("viewProj\x00")),
		viewUniform:       gl.GetUniformLocation(program, gl.Str("view\x00")),
		texturedUniform:   gl.GetUniformLocation(program, gl.Str("textured\x00")),
		alphaTestUniform:  gl.GetUniformLocation(program, gl.Str("alphaTest\x00")),
		unshadedUniform:   gl.GetUniformLocation(program, gl.Str("unshaded\x00")),
		alphaUniform:      gl.GetUniformLocation(program, gl.Str("alpha\x00")),
		teamMaskedUniform: gl.GetUniformLocation(program, gl.Str("teamMasked\x00")),
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

// gpuLayer is one material layer of a gpuGeoset, ready to draw: its texture
// (0 = grey, bound at draw time for sourceTeamColor/sourceTeamGlow instead),
// filter mode and shading flags for GL state, its alpha (already scaled by
// the geoset's alpha), wrap mode, and the byte offset of its UV set's block
// in the geoset's VBO.
type gpuLayer struct {
	texture      uint32
	source       layerSource
	filter       mdx.FilterMode
	shading      uint32
	alpha        float32
	wrapS, wrapT int32
	uvOffset     int32
	mask         uint32 // ORM texture tinting the diffuse by team colour; 0 = unmasked
}

// gpuGeoset is one geoset's VAO/VBO/IBO, its layers in material order, its
// priority plane, and whether it draws opaque (no depth write skipped) or
// blended (drawn after every opaque geoset).
type gpuGeoset struct {
	vao, vbo, ibo uint32
	count         int32
	layers        []gpuLayer
	opaque        bool
	priority      int32
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
// 0 to VAO/VBO/IBO pairs: position+normal (zero-filled when the geoset has
// no normals) interleaved, followed by each of the geoset's UV sets as its
// own packed block, in a single vertex buffer per geoset. lowestLOD is
// computed over every geoset, visible or not, so hiding geosets never
// changes which LOD is rendered. For each layer whose source is
// sourceTexture, its DDS is read through storage and uploaded once per
// content key, shared across geosets and layers; a masked layer's ORM is
// likewise uploaded once per content key, but only when its DDS is DXT5 —
// otherwise the layer draws unmasked. Geosets are returned opaque first
// (first layer's filter none/transparent, or no layers), then blended, each
// group ordered by priority plane ascending then geoset index. GL must be
// current.
func uploadMesh(storage *casc.Storage, geosets []mdx.Geoset, alphas []float32, layers [][]geosetLayer, priority []int32) modelMesh {
	lod := lowestLOD(geosets)

	mesh := modelMesh{lod: lod, textures: map[[16]byte]uint32{}}
	first := true

	for i, g := range geosets {
		if g.LOD != lod || len(g.Indices) == 0 || alphas[i] <= 0 {
			continue
		}

		vertexCount := g.VertexCount()
		posNormBytes := vertexCount * 6 * 4
		uvBlockBytes := vertexCount * 2 * 4

		vertices := make([]float32, 0, vertexCount*6+len(g.UVs)*vertexCount*2)
		hasNormals := len(g.Normals) == len(g.Positions)
		for v := 0; v < vertexCount; v++ {
			px, py, pz := g.Positions[v*3], g.Positions[v*3+1], g.Positions[v*3+2]
			vertices = append(vertices, px, py, pz)
			if hasNormals {
				vertices = append(vertices, g.Normals[v*3], g.Normals[v*3+1], g.Normals[v*3+2])
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
		for _, uvSet := range g.UVs {
			vertices = append(vertices, uvSet...)
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
		gl.EnableVertexAttribArray(uvAttrib) // pointer set per layer in draw, same VBO

		gl.BindVertexArray(0)

		gg.count = int32(len(g.Indices))
		gg.priority = priority[i]

		gls := layers[i]
		if len(gls) == 0 {
			// No material (or an out-of-range id): draw flat grey, as a
			// single opaque layer, same as a geoset with a grey diffuse.
			gls = []geosetLayer{{filter: mdx.FilterNone, alpha: alphas[i]}}
		}
		gg.opaque = gls[0].filter == mdx.FilterNone || gls[0].filter == mdx.FilterTransparent
		gg.layers = make([]gpuLayer, len(gls))
		for j, l := range gls {
			gpuL := gpuLayer{
				source:   l.source,
				filter:   l.filter,
				shading:  l.shading,
				alpha:    l.alpha,
				uvOffset: int32(posNormBytes + l.uvSet*uvBlockBytes),
			}

			if l.flags&1 != 0 {
				gpuL.wrapS = gl.REPEAT
			} else {
				gpuL.wrapS = gl.CLAMP_TO_EDGE
			}
			if l.flags&2 != 0 {
				gpuL.wrapT = gl.REPEAT
			} else {
				gpuL.wrapT = gl.CLAMP_TO_EDGE
			}

			if l.source == sourceTexture {
				tex, ok := mesh.textures[l.entry.CKey]
				if !ok {
					data := storage.ReadByCKey(l.entry.CKey)
					tex = uploadTexture(texture.ReadDDS(data), data)
					mesh.textures[l.entry.CKey] = tex
				}
				gpuL.texture = tex
			}

			if l.masked {
				tex, ok := mesh.textures[l.mask.CKey]
				if !ok {
					data := storage.ReadByCKey(l.mask.CKey)
					dds := texture.ReadDDS(data)
					if dds.Format == texture.DXT5 {
						tex = uploadTexture(dds, data)
						mesh.textures[l.mask.CKey] = tex
					}
				}
				gpuL.mask = tex
			}

			gg.layers[j] = gpuL
		}

		mesh.geosets = append(mesh.geosets, gg)
	}

	sort.SliceStable(mesh.geosets, func(a, b int) bool {
		ga, gb := mesh.geosets[a], mesh.geosets[b]
		if ga.opaque != gb.opaque {
			return ga.opaque
		}
		return ga.priority < gb.priority
	})

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

// filterGLState maps a layer's filter mode to its blend factors (meaningful
// only when blend is true) and whether it writes depth, the usual MDX
// mapping (as in mdx-m3-viewer / Retera's Model Studio); an unrecognized
// filter mode draws as FilterNone (none seen in wardata).
func filterGLState(f mdx.FilterMode) (blend bool, src, dst uint32, depthWrite bool) {
	switch f {
	case mdx.FilterTransparent:
		return false, 0, 0, true
	case mdx.FilterBlend:
		return true, gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, false
	case mdx.FilterAdditive:
		return true, gl.ONE, gl.ONE, false
	case mdx.FilterAddAlpha:
		return true, gl.SRC_ALPHA, gl.ONE, false
	case mdx.FilterModulate:
		return true, gl.ZERO, gl.SRC_COLOR, false
	case mdx.FilterModulate2x:
		return true, gl.DST_COLOR, gl.SRC_COLOR, false
	default: // FilterNone and anything unrecognized
		return false, 0, 0, true
	}
}

// draw renders every geoset's layers with prog, under view and proj: opaque
// geosets first, then blended ones, each layer's GL state (alpha test,
// blend, depth write) set from its filter mode and shading flags (bit 0
// unshaded, bit 6 no depth test, bit 7 no depth write), textured when its
// source resolved (team colour/glow layers bind team's textures), else flat
// grey; a masked layer additionally tints its diffuse by team.color where
// its ORM's alpha says so. Leaves depth testing, depth writes and blending
// at their defaults.
func (m *modelMesh) draw(prog *meshProgram, view, proj mat4, team teamTextures) {
	gl.Disable(gl.CULL_FACE)
	gl.DepthFunc(gl.LEQUAL) // so a layer stacked on an earlier one at the same depth still passes

	gl.UseProgram(prog.program)
	viewProj := proj.mul(view)
	gl.UniformMatrix4fv(prog.viewProjUniform, 1, false, &viewProj[0])
	gl.UniformMatrix4fv(prog.viewUniform, 1, false, &view[0])

	gl.ActiveTexture(gl.TEXTURE0)
	for _, g := range m.geosets {
		gl.BindVertexArray(g.vao)

		for _, l := range g.layers {
			blend, src, dst, depthWrite := filterGLState(l.filter)
			if l.shading&(1<<7) != 0 {
				depthWrite = false
			}
			if l.shading&(1<<6) != 0 {
				gl.Disable(gl.DEPTH_TEST)
			} else {
				gl.Enable(gl.DEPTH_TEST)
			}
			gl.DepthMask(depthWrite)
			if blend {
				gl.Enable(gl.BLEND)
				gl.BlendFunc(src, dst)
			} else {
				gl.Disable(gl.BLEND)
			}

			gl.Uniform1i(prog.unshadedUniform, boolToInt32(l.shading&1 != 0))
			gl.Uniform1i(prog.alphaTestUniform, boolToInt32(l.filter == mdx.FilterTransparent))
			gl.Uniform1f(prog.alphaUniform, l.alpha)

			tex := l.texture
			switch l.source {
			case sourceTeamColor:
				tex = team.color
			case sourceTeamGlow:
				tex = team.glow
			}
			if tex != 0 {
				gl.BindTexture(gl.TEXTURE_2D, tex)
				gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, l.wrapS)
				gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, l.wrapT)
				gl.Uniform1i(prog.texturedUniform, 1)
			} else {
				gl.Uniform1i(prog.texturedUniform, 0)
			}

			if l.mask != 0 {
				gl.ActiveTexture(gl.TEXTURE1)
				gl.BindTexture(gl.TEXTURE_2D, l.mask)
				gl.ActiveTexture(gl.TEXTURE2)
				gl.BindTexture(gl.TEXTURE_2D, team.color)
				gl.ActiveTexture(gl.TEXTURE0)
				gl.Uniform1i(prog.teamMaskedUniform, 1)
			} else {
				gl.Uniform1i(prog.teamMaskedUniform, 0)
			}

			gl.VertexAttribPointerWithOffset(uvAttrib, 2, gl.FLOAT, false, 8, uintptr(l.uvOffset))
			gl.DrawElements(gl.TRIANGLES, g.count, gl.UNSIGNED_SHORT, nil)
		}
	}

	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	gl.DepthMask(true)
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LESS)
}

// boolToInt32 converts b to a GLSL-uniform-friendly 0/1.
func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
