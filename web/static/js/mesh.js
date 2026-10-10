// Decodes the binary buffer served by GET /api/mesh into typed-array views,
// and computes the bounds of the geosets visible at upload time (ported
// from viewer/mesh.go's uploadMesh bounds tracking).

import { MAX_INFLUENCES } from "./api.js";

/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./api.js").Geoset} Geoset */

/**
 * @typedef {Object} DecodedGeoset
 * @property {number} index - index into model.geosets
 * @property {number} alpha
 * @property {number} vertices
 * @property {Float32Array} positions - 3 per vertex
 * @property {Float32Array} normals - 3 per vertex, zero-filled when the geoset has none
 * @property {Float32Array[]} uvs - one per UV set, 2 per vertex
 * @property {Float32Array} weights - MAX_INFLUENCES per vertex
 * @property {Uint32Array} nodes - MAX_INFLUENCES per vertex
 * @property {Uint16Array} indices - 3 per triangle
 */

/**
 * checkView throws when a would-be typed array view of length bytes at
 * offset wouldn't lie entirely inside a buffer of size bufferLength.
 * @param {number} offset
 * @param {number} length
 * @param {number} bufferLength
 * @param {number} geosetIndex
 * @param {string} field
 */
function checkView(offset, length, bufferLength, geosetIndex, field) {
	if (offset < 0 || length < 0 || offset + length > bufferLength) {
		throw new Error(`mesh: geoset ${geosetIndex}: ${field} offset ${offset} length ${length} outside buffer of ${bufferLength} bytes`);
	}
}

/**
 * decodeMesh builds one DecodedGeoset per model.geosets entry whose mesh
 * offsets are non-null, as typed-array views into buffer.
 * @param {Model} model
 * @param {ArrayBuffer} buffer
 * @returns {DecodedGeoset[]}
 */
export function decodeMesh(model, buffer) {
	/** @type {DecodedGeoset[]} */
	const out = [];

	for (let i = 0; i < model.geosets.length; i++) {
		const g = model.geosets[i];
		if (g.mesh === null) {
			continue;
		}
		const off = g.mesh;
		const v = g.vertices;
		const t = g.triangles;

		checkView(off.positions, 12 * v, buffer.byteLength, i, "positions");
		const positions = new Float32Array(buffer, off.positions, 3 * v);

		checkView(off.normals, 12 * v, buffer.byteLength, i, "normals");
		const normals = new Float32Array(buffer, off.normals, 3 * v);

		const uvs = off.uvs.map((o, u) => {
			checkView(o, 8 * v, buffer.byteLength, i, `uvs[${u}]`);
			return new Float32Array(buffer, o, 2 * v);
		});

		checkView(off.weights, 4 * MAX_INFLUENCES * v, buffer.byteLength, i, "weights");
		const weights = new Float32Array(buffer, off.weights, MAX_INFLUENCES * v);

		checkView(off.nodes, 4 * MAX_INFLUENCES * v, buffer.byteLength, i, "nodes");
		const nodes = new Uint32Array(buffer, off.nodes, MAX_INFLUENCES * v);

		checkView(off.indices, 2 * 3 * t, buffer.byteLength, i, "indices");
		const indices = new Uint16Array(buffer, off.indices, 3 * t);

		out.push({ index: i, alpha: g.alpha, vertices: v, positions, normals, uvs, weights, nodes, indices });
	}

	return out;
}

/**
 * visibleBounds returns the min/max corners over the positions of geosets
 * with alpha > 0, or null when nothing is visible.
 * @param {DecodedGeoset[]} geosets
 * @returns {{min: [number, number, number], max: [number, number, number]} | null}
 */
export function visibleBounds(geosets) {
	/** @type {[number, number, number] | null} */
	let min = null;
	/** @type {[number, number, number] | null} */
	let max = null;

	for (const g of geosets) {
		if (g.alpha <= 0) {
			continue;
		}
		for (let v = 0; v < g.vertices; v++) {
			const p = /** @type {[number, number, number]} */ ([g.positions[3 * v], g.positions[3 * v + 1], g.positions[3 * v + 2]]);
			if (min === null || max === null) {
				min = [...p];
				max = [...p];
				continue;
			}
			for (let k = 0; k < 3; k++) {
				if (p[k] < min[k]) min[k] = p[k];
				if (p[k] > max[k]) max[k] = p[k];
			}
		}
	}

	if (min === null || max === null) {
		return null;
	}
	return { min, max };
}
