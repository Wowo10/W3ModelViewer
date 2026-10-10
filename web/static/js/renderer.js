// WebGL2 renderer: the desktop's mesh shader (viewer/mesh.go) reduced to
// position + normal and the grey, lit branch - no textures, blending or
// skinning yet (WEB-05/WEB-06).

import { mul } from "./mat4.js";

/** @typedef {import("./mat4.js").Mat4} Mat4 */
/** @typedef {import("./mesh.js").DecodedGeoset} DecodedGeoset */

const POSITION_ATTRIB = 0;
const NORMAL_ATTRIB = 1;

const VERTEX_SHADER_SRC = `#version 300 es
in vec3 position;
in vec3 normal;

uniform mat4 viewProj;
uniform mat4 view;

out vec3 vNormal;

void main() {
	vNormal = mat3(view) * normal;
	gl_Position = viewProj * vec4(position, 1.0);
}
`;

const FRAGMENT_SHADER_SRC = `#version 300 es
precision highp float;

in vec3 vNormal;
out vec4 fragColor;

void main() {
	vec3 color = vec3(0.75, 0.75, 0.78);

	float lit = 1.0;
	float len = length(vNormal);
	if (len > 0.0001) {
		vec3 n = vNormal / len;
		vec3 l = normalize(vec3(-0.4, -0.2, 0.9));
		lit = 0.25 + 0.75 * abs(dot(n, l));
	}

	fragColor = vec4(color * lit, 1.0);
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
 * @typedef {Object} GPUGeoset
 * @property {WebGLVertexArrayObject} vao
 * @property {WebGLBuffer} ibo
 * @property {number} count - indices drawn
 * @property {number} alpha
 */

/**
 * Renderer draws a model's lowest-LOD geosets (visible ones, grey and
 * normal-lit) over a WebGL2 context.
 */
export class Renderer {
	/** @param {WebGL2RenderingContext} gl */
	constructor(gl) {
		this.gl = gl;
		this.program = compileProgram(gl);
		this.viewProjLoc = gl.getUniformLocation(this.program, "viewProj");
		this.viewLoc = gl.getUniformLocation(this.program, "view");

		/** @type {WebGLBuffer | null} */
		this.vbo = null;
		/** @type {GPUGeoset[]} */
		this.geosets = [];
	}

	/**
	 * setMesh uploads buffer once as a shared ARRAY_BUFFER and builds one
	 * VAO + its own ELEMENT_ARRAY_BUFFER per geoset (a single buffer can't
	 * be bound as both array and element buffer), releasing the previous
	 * model's GL objects first.
	 * @param {ArrayBuffer} buffer
	 * @param {DecodedGeoset[]} geosets
	 */
	setMesh(buffer, geosets) {
		const gl = this.gl;
		this.releaseMesh();

		const vbo = gl.createBuffer();
		if (!vbo) {
			throw new Error("renderer: createBuffer failed");
		}
		gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
		gl.bufferData(gl.ARRAY_BUFFER, buffer, gl.STATIC_DRAW);
		this.vbo = vbo;

		this.geosets = geosets.map((g) => {
			const vao = gl.createVertexArray();
			if (!vao) {
				throw new Error("renderer: createVertexArray failed");
			}
			gl.bindVertexArray(vao);
			gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
			gl.vertexAttribPointer(POSITION_ATTRIB, 3, gl.FLOAT, false, 0, g.positions.byteOffset);
			gl.enableVertexAttribArray(POSITION_ATTRIB);
			gl.vertexAttribPointer(NORMAL_ATTRIB, 3, gl.FLOAT, false, 0, g.normals.byteOffset);
			gl.enableVertexAttribArray(NORMAL_ATTRIB);

			const ibo = gl.createBuffer();
			if (!ibo) {
				throw new Error("renderer: createBuffer failed");
			}
			gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, ibo);
			gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, g.indices, gl.STATIC_DRAW);

			gl.bindVertexArray(null);
			return { vao, ibo, count: g.indices.length, alpha: g.alpha };
		});
	}

	/** releaseMesh deletes every geoset's VAO/IBO and the shared VBO. */
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
	}

	/**
	 * draw clears the canvas and draws every geoset with alpha > 0.
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

		if (this.geosets.length === 0) {
			return;
		}

		gl.disable(gl.CULL_FACE);
		gl.enable(gl.DEPTH_TEST);
		gl.depthFunc(gl.LEQUAL);
		gl.depthMask(true);
		gl.disable(gl.BLEND);

		gl.useProgram(this.program);
		const viewProj = mul(proj, view);
		gl.uniformMatrix4fv(this.viewProjLoc, false, viewProj);
		gl.uniformMatrix4fv(this.viewLoc, false, view);

		for (const g of this.geosets) {
			if (g.alpha <= 0) {
				continue;
			}
			gl.bindVertexArray(g.vao);
			gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, g.ibo);
			gl.drawElements(gl.TRIANGLES, g.count, gl.UNSIGNED_SHORT, 0);
		}

		gl.bindVertexArray(null);
	}
}
