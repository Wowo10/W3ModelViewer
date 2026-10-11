// WebGL2 renderer: ports the desktop's mesh shader and draw (viewer/mesh.go)
// to the page - every layer textured, blended and team-coloured, in the
// desktop's draw order and GL state, skinned on the GPU from a per-pose
// RGBA32F node matrix texture and each vertex's up to 8 influences
// (Geoset.Influences(), see viewer/mesh.go and viewer/pose.go).

import { mul } from "./mat4.js";
import { layerState, geosetLayers, drawOrder } from "./layers.js";
import { MATRIX_TEXTURE_WIDTH, matrixTextureSize } from "./pose.js";

/** @typedef {import("./mat4.js").Mat4} Mat4 */
/** @typedef {import("./mesh.js").DecodedGeoset} DecodedGeoset */
/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./api.js").Layer} Layer */
/** @typedef {import("./texture.js").UploadedTexture} UploadedTexture */

const POSITION_ATTRIB = 0;
const NORMAL_ATTRIB = 1;
const UV_ATTRIB = 2;
const NODES0_ATTRIB = 3;
const NODES1_ATTRIB = 4;
const WEIGHTS0_ATTRIB = 5;
const WEIGHTS1_ATTRIB = 6;

// Texture units, past diffuseMap (0), ormMap (1) and teamColorMap (2).
const DIFFUSE_UNIT = 0;
const ORM_UNIT = 1;
const TEAM_COLOR_UNIT = 2;
const NODE_MATRICES_UNIT = 3;

const VERTEX_SHADER_SRC = `#version 300 es
in vec3 position;
in vec3 normal;
in vec2 uv;
in uvec4 nodes0;
in uvec4 nodes1;
in vec4 weights0;
in vec4 weights1;

uniform mat4 viewProj;
uniform mat4 view;
uniform highp sampler2D nodeMatrices;

out vec3 vNormal;
out vec2 vUV;

ivec2 texelPos(int t) {
	return ivec2(t % ${MATRIX_TEXTURE_WIDTH}, t / ${MATRIX_TEXTURE_WIDTH});
}

mat4 nodeMatrix(uint id) {
	int base = int(id) * 4;
	return mat4(
		texelFetch(nodeMatrices, texelPos(base), 0),
		texelFetch(nodeMatrices, texelPos(base + 1), 0),
		texelFetch(nodeMatrices, texelPos(base + 2), 0),
		texelFetch(nodeMatrices, texelPos(base + 3), 0)
	);
}

void main() {
	mat4 skin = weights0[0] * nodeMatrix(nodes0[0])
	          + weights0[1] * nodeMatrix(nodes0[1])
	          + weights0[2] * nodeMatrix(nodes0[2])
	          + weights0[3] * nodeMatrix(nodes0[3])
	          + weights1[0] * nodeMatrix(nodes1[0])
	          + weights1[1] * nodeMatrix(nodes1[1])
	          + weights1[2] * nodeMatrix(nodes1[2])
	          + weights1[3] * nodeMatrix(nodes1[3]);

	vNormal = mat3(view) * mat3(skin) * normal;
	vUV = uv;
	gl_Position = viewProj * skin * vec4(position, 1.0);
}
`;

const FRAGMENT_SHADER_SRC = `#version 300 es
precision highp float;

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
`;

/**
 * compileShader compiles src as type, throwing with the info log on
 * failure.
 * @param {WebGL2RenderingContext} gl
 * @param {number} type
 * @param {string} src
 * @returns {WebGLShader}
 */
function compileShader(gl, type, src) {
	const shader = gl.createShader(type);
	if (!shader) {
		throw new Error("renderer: createShader failed");
	}
	gl.shaderSource(shader, src);
	gl.compileShader(shader);
	if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
		const log = gl.getShaderInfoLog(shader);
		gl.deleteShader(shader);
		throw new Error(`renderer: compile shader: ${log}`);
	}
	return shader;
}

/**
 * compileProgram links the mesh program, binding attribute locations before
 * linking.
 * @param {WebGL2RenderingContext} gl
 * @returns {WebGLProgram}
 */
function compileProgram(gl) {
	const vs = compileShader(gl, gl.VERTEX_SHADER, VERTEX_SHADER_SRC);
	const fs = compileShader(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER_SRC);

	const program = gl.createProgram();
	if (!program) {
		throw new Error("renderer: createProgram failed");
	}
	gl.attachShader(program, vs);
	gl.attachShader(program, fs);
	gl.bindAttribLocation(program, POSITION_ATTRIB, "position");
	gl.bindAttribLocation(program, NORMAL_ATTRIB, "normal");
	gl.bindAttribLocation(program, UV_ATTRIB, "uv");
	gl.bindAttribLocation(program, NODES0_ATTRIB, "nodes0");
	gl.bindAttribLocation(program, NODES1_ATTRIB, "nodes1");
	gl.bindAttribLocation(program, WEIGHTS0_ATTRIB, "weights0");
	gl.bindAttribLocation(program, WEIGHTS1_ATTRIB, "weights1");
	gl.linkProgram(program);

	gl.deleteShader(vs);
	gl.deleteShader(fs);

	if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
		const log = gl.getProgramInfoLog(program);
		gl.deleteProgram(program);
		throw new Error(`renderer: link program: ${log}`);
	}
	return program;
}

/**
 * @typedef {Object} GPULayer
 * @property {import("./layers.js").LayerState} state
 * @property {string} source - "texture" | "teamColor" | "teamGlow" | "grey"
 * @property {WebGLTexture | null} texture - bound for source "texture"
 * @property {WebGLTexture | null} mask - the ORM, when masked and its DDS is DXT5
 * @property {number} alpha
 * @property {number} wrapS
 * @property {number} wrapT
 * @property {number | null} uvOffset - byte offset into the geoset's VBO, null when the geoset has no UV sets
 */

/**
 * @typedef {Object} GPUGeoset
 * @property {WebGLVertexArrayObject} vao
 * @property {WebGLBuffer} ibo
 * @property {number} count - indices drawn
 * @property {GPULayer[]} layers
 * @property {number} index - index into model.geosets, to look up the current pose alpha
 */

/**
 * @typedef {Object} TeamTextures
 * @property {WebGLTexture} color
 * @property {WebGLTexture} glow
 */

/**
 * wrapEnum maps a Layer's wrap flag to a GL wrap mode.
 * @param {WebGL2RenderingContext} gl
 * @param {boolean} wrap
 * @returns {number}
 */
function wrapEnum(gl, wrap) {
	return wrap ? gl.REPEAT : gl.CLAMP_TO_EDGE;
}

/**
 * Renderer draws a model's lowest-LOD geosets, textured and blended per
 * layer, team-coloured, in the desktop's draw order.
 */
export class Renderer {
	/** @param {WebGL2RenderingContext} gl */
	constructor(gl) {
		this.gl = gl;
		this.program = compileProgram(gl);
		this.viewProjLoc = gl.getUniformLocation(this.program, "viewProj");
		this.viewLoc = gl.getUniformLocation(this.program, "view");
		this.texturedLoc = gl.getUniformLocation(this.program, "textured");
		this.alphaTestLoc = gl.getUniformLocation(this.program, "alphaTest");
		this.unshadedLoc = gl.getUniformLocation(this.program, "unshaded");
		this.teamMaskedLoc = gl.getUniformLocation(this.program, "teamMasked");
		this.alphaLoc = gl.getUniformLocation(this.program, "alpha");

		gl.useProgram(this.program);
		gl.uniform1i(gl.getUniformLocation(this.program, "diffuseMap"), DIFFUSE_UNIT);
		gl.uniform1i(gl.getUniformLocation(this.program, "ormMap"), ORM_UNIT);
		gl.uniform1i(gl.getUniformLocation(this.program, "teamColorMap"), TEAM_COLOR_UNIT);
		gl.uniform1i(gl.getUniformLocation(this.program, "nodeMatrices"), NODE_MATRICES_UNIT);
		gl.useProgram(null);

		/** @type {WebGLBuffer | null} */
		this.vbo = null;
		/** @type {GPUGeoset[]} */
		this.geosets = [];
		/** @type {Map<string, UploadedTexture>} */
		this.textures = new Map();
		/** @type {TeamTextures | null} */
		this.team = null;

		/** @type {WebGLTexture | null} */
		this.matrixTexture = null;
		this.matrixWidth = 0;
		this.matrixHeight = 0;
		/** @type {Float32Array | null} - reused across setPose calls, sized width*height*4 */
		this.poseBuffer = null;
		/** @type {Float32Array | null} - one per geoset, from the last setPose; draw skips until set */
		this.alphas = null;
	}

	/**
	 * setModel uploads buffer once as a shared ARRAY_BUFFER and builds one VAO
	 * + its own ELEMENT_ARRAY_BUFFER per geoset, in the desktop's draw order
	 * (opaque first, then blended by priority plane, stable on geoset
	 * index), over every lowest-LOD geoset - alpha is per-pose now (see
	 * draw), so a geoset hidden at upload time (e.g. a construction stage)
	 * can still appear once its track says so. textures is keyed by the hex
	 * content key of every resolved diffuse/mask texture the model's
	 * lowest-LOD layers use; the renderer takes ownership and deletes them
	 * on the next setModel/releaseMesh. Also (re)allocates the node matrix
	 * texture for model.objects; a pose must be set with setPose before
	 * draw renders anything for the new model. Releases the previous
	 * model's GL objects first.
	 * @param {ArrayBuffer} buffer
	 * @param {DecodedGeoset[]} decoded
	 * @param {Model} model
	 * @param {Map<string, UploadedTexture>} textures
	 */
	setModel(buffer, decoded, model, textures) {
		const gl = this.gl;
		this.releaseMesh();
		this.textures = textures;

		const vbo = gl.createBuffer();
		if (!vbo) {
			throw new Error("renderer: createBuffer failed");
		}
		gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
		gl.bufferData(gl.ARRAY_BUFFER, buffer, gl.STATIC_DRAW);
		this.vbo = vbo;

		const order = drawOrder(decoded.map((dg) => model.geosets[dg.index]));

		this.geosets = order.map((i) => {
			const dg = decoded[i];
			const def = model.geosets[dg.index];
			const layers = geosetLayers(def);

			const vao = gl.createVertexArray();
			if (!vao) {
				throw new Error("renderer: createVertexArray failed");
			}
			gl.bindVertexArray(vao);
			gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
			gl.vertexAttribPointer(POSITION_ATTRIB, 3, gl.FLOAT, false, 0, dg.positions.byteOffset);
			gl.enableVertexAttribArray(POSITION_ATTRIB);
			gl.vertexAttribPointer(NORMAL_ATTRIB, 3, gl.FLOAT, false, 0, dg.normals.byteOffset);
			gl.enableVertexAttribArray(NORMAL_ATTRIB);
			if (dg.uvs.length > 0) {
				gl.enableVertexAttribArray(UV_ATTRIB); // pointer re-set per layer in draw
			} else {
				gl.disableVertexAttribArray(UV_ATTRIB);
				gl.vertexAttrib2f(UV_ATTRIB, 0, 0);
			}

			gl.vertexAttribIPointer(NODES0_ATTRIB, 4, gl.UNSIGNED_INT, 32, dg.nodes.byteOffset);
			gl.enableVertexAttribArray(NODES0_ATTRIB);
			gl.vertexAttribIPointer(NODES1_ATTRIB, 4, gl.UNSIGNED_INT, 32, dg.nodes.byteOffset + 16);
			gl.enableVertexAttribArray(NODES1_ATTRIB);
			gl.vertexAttribPointer(WEIGHTS0_ATTRIB, 4, gl.FLOAT, false, 32, dg.weights.byteOffset);
			gl.enableVertexAttribArray(WEIGHTS0_ATTRIB);
			gl.vertexAttribPointer(WEIGHTS1_ATTRIB, 4, gl.FLOAT, false, 32, dg.weights.byteOffset + 16);
			gl.enableVertexAttribArray(WEIGHTS1_ATTRIB);

			const ibo = gl.createBuffer();
			if (!ibo) {
				throw new Error("renderer: createBuffer failed");
			}
			gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo);
			gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, dg.indices, gl.STATIC_DRAW);

			gl.bindVertexArray(null);

			/** @type {GPULayer[]} */
			const gpuLayers = layers.map((l) => {
				const texRecord = l.source === "texture" && l.texture ? this.textures.get(l.texture.ckey) : undefined;
				const maskRecord = l.mask ? this.textures.get(l.mask.ckey) : undefined;
				const masked = maskRecord !== undefined && maskRecord.format === "DXT5";

				return {
					state: layerState(l),
					source: l.source,
					texture: texRecord ? texRecord.texture : null,
					mask: masked && maskRecord ? maskRecord.texture : null,
					alpha: l.alpha,
					wrapS: wrapEnum(gl, l.wrapWidth),
					wrapT: wrapEnum(gl, l.wrapHeight),
					uvOffset: dg.uvs.length > 0 ? dg.uvs[Math.min(l.uvSet, dg.uvs.length - 1)].byteOffset : null,
				};
			});

			return { vao, ibo, count: dg.indices.length, layers: gpuLayers, index: dg.index };
		});

		const size = matrixTextureSize(model.objects);
		const texture = gl.createTexture();
		if (!texture) {
			throw new Error("renderer: createTexture failed");
		}
		gl.bindTexture(gl.TEXTURE_2D, texture);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
		gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
		gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, size.width, size.height, 0, gl.RGBA, gl.FLOAT, null);
		gl.bindTexture(gl.TEXTURE_2D, null);

		this.matrixTexture = texture;
		this.matrixWidth = size.width;
		this.matrixHeight = size.height;
		this.poseBuffer = new Float32Array(size.width * size.height * 4);
		this.alphas = null;
	}

	/**
	 * setPose uploads matrices (model.objects x 16 float32, column-major, by
	 * object id) into the node matrix texture and keeps alphas (one per
	 * geoset) for draw to look up by geoset index.
	 * @param {Float32Array} matrices
	 * @param {Float32Array} alphas
	 */
	setPose(matrices, alphas) {
		const gl = this.gl;
		if (!this.matrixTexture || !this.poseBuffer) {
			return;
		}

		this.poseBuffer.set(matrices);

		gl.bindTexture(gl.TEXTURE_2D, this.matrixTexture);
		gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, this.matrixWidth, this.matrixHeight, gl.RGBA, gl.FLOAT, this.poseBuffer);
		gl.bindTexture(gl.TEXTURE_2D, null);

		this.alphas = alphas;
	}

	/**
	 * setTeam swaps the team colour/glow textures used by "teamColor"/
	 * "teamGlow" layers, deleting the previous pair.
	 * @param {WebGLTexture} color
	 * @param {WebGLTexture} glow
	 */
	setTeam(color, glow) {
		const gl = this.gl;
		if (this.team) {
			gl.deleteTexture(this.team.color);
			gl.deleteTexture(this.team.glow);
		}
		this.team = { color, glow };
	}

	/** releaseMesh deletes every geoset's VAO/IBO, the shared VBO, the node matrix texture and every uploaded texture (not the team pair). */
	releaseMesh() {
		const gl = this.gl;
		for (const g of this.geosets) {
			gl.deleteVertexArray(g.vao);
			gl.deleteBuffer(g.ibo);
		}
		this.geosets = [];
		if (this.vbo) {
			gl.deleteBuffer(this.vbo);
			this.vbo = null;
		}
		if (this.matrixTexture) {
			gl.deleteTexture(this.matrixTexture);
			this.matrixTexture = null;
		}
		this.poseBuffer = null;
		this.alphas = null;
		for (const t of this.textures.values()) {
			gl.deleteTexture(t.texture);
		}
		this.textures = new Map();
	}

	/**
	 * draw clears the canvas and draws every geoset's layers: opaque first,
	 * then blended (the order setModel already sorted into), each layer's GL
	 * state from its filter mode and shading flags, team colour/glow layers
	 * drawn grey until the team pair is loaded. A geoset whose current pose
	 * alpha is <= 0 is skipped entirely; otherwise each layer's alpha
	 * uniform is its own alpha times the geoset's. Draws nothing until
	 * setPose has been called at least once for the current model.
	 * @param {Mat4} view
	 * @param {Mat4} proj
	 * @param {number} width
	 * @param {number} height
	 */
	draw(view, proj, width, height) {
		const gl = this.gl;
		gl.viewport(0, 0, width, height);
		gl.clearColor(0.1, 0.11, 0.13, 1);
		gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);

		if (this.geosets.length === 0 || this.alphas === null) {
			return;
		}
		const alphas = this.alphas;

		gl.disable(gl.CULL_FACE);
		gl.depthFunc(gl.LEQUAL);

		gl.useProgram(this.program);
		const viewProj = mul(proj, view);
		gl.uniformMatrix4fv(this.viewProjLoc, false, viewProj);
		gl.uniformMatrix4fv(this.viewLoc, false, view);

		gl.activeTexture(gl.TEXTURE0 + NODE_MATRICES_UNIT);
		gl.bindTexture(gl.TEXTURE_2D, this.matrixTexture);

		gl.activeTexture(gl.TEXTURE0 + DIFFUSE_UNIT);
		for (const g of this.geosets) {
			const geosetAlpha = alphas[g.index];
			if (geosetAlpha <= 0) {
				continue;
			}

			gl.bindVertexArray(g.vao);
			gl.bindBuffer(gl.ARRAY_BUFFER, this.vbo); // the UV pointer below needs ARRAY_BUFFER bound; VAO state doesn't cover it

			for (const l of g.layers) {
				gl.depthMask(l.state.depthWrite);
				if (l.state.depthTest) {
					gl.enable(gl.DEPTH_TEST);
				} else {
					gl.disable(gl.DEPTH_TEST);
				}
				if (l.state.blend) {
					gl.enable(gl.BLEND);
					gl.blendFunc(l.state.src, l.state.dst);
				} else {
					gl.disable(gl.BLEND);
				}

				gl.uniform1i(this.unshadedLoc, l.state.unshaded ? 1 : 0);
				gl.uniform1i(this.alphaTestLoc, l.state.alphaTest ? 1 : 0);
				gl.uniform1f(this.alphaLoc, l.alpha * geosetAlpha);

				let tex = l.texture;
				if (l.source === "teamColor" && this.team) {
					tex = this.team.color;
				} else if (l.source === "teamGlow" && this.team) {
					tex = this.team.glow;
				} else if ((l.source === "teamColor" || l.source === "teamGlow") && !this.team) {
					tex = null;
				}

				if (tex) {
					gl.activeTexture(gl.TEXTURE0 + DIFFUSE_UNIT);
					gl.bindTexture(gl.TEXTURE_2D, tex);
					gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, l.wrapS);
					gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, l.wrapT);
					gl.uniform1i(this.texturedLoc, 1);
				} else {
					gl.uniform1i(this.texturedLoc, 0);
				}

				if (l.mask && this.team) {
					gl.activeTexture(gl.TEXTURE0 + ORM_UNIT);
					gl.bindTexture(gl.TEXTURE_2D, l.mask);
					gl.activeTexture(gl.TEXTURE0 + TEAM_COLOR_UNIT);
					gl.bindTexture(gl.TEXTURE_2D, this.team.color);
					gl.activeTexture(gl.TEXTURE0 + DIFFUSE_UNIT);
					gl.uniform1i(this.teamMaskedLoc, 1);
				} else {
					gl.uniform1i(this.teamMaskedLoc, 0);
				}

				if (l.uvOffset !== null) {
					gl.vertexAttribPointer(UV_ATTRIB, 2, gl.FLOAT, false, 0, l.uvOffset);
				}
				gl.drawElements(gl.TRIANGLES, g.count, gl.UNSIGNED_SHORT, 0);
			}
		}

		gl.bindVertexArray(null);
		gl.bindTexture(gl.TEXTURE_2D, null);
		gl.depthMask(true);
		gl.disable(gl.BLEND);
		gl.enable(gl.DEPTH_TEST);
		gl.depthFunc(gl.LESS);
	}
}
