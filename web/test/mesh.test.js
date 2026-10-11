import test from "node:test";
import assert from "node:assert/strict";
import { decodeMesh, lodLabel, pickLOD, visibleBounds } from "../static/js/mesh.js";
import { MAX_INFLUENCES } from "../static/js/api.js";

/** @typedef {import("../static/js/api.js").Geoset} Geoset */
/** @typedef {import("../static/js/api.js").Model} Model */

/**
 * buildMesh lays out geosets into one ArrayBuffer using the server's
 * layout (web/model.go's buildMesh): positions, normals, each UV set,
 * weights, node ids, indices padded to 4 bytes - mirroring the Go encoder
 * exactly so decodeMesh's offsets are meaningful.
 * @param {{vertices: number, triangles: number, uvSets: number, alpha: number}[]} specs
 * @returns {{model: Model, buffer: ArrayBuffer}}
 */
function buildMesh(specs) {
	/** @type {ArrayBufferLike[]} */
	const chunks = [];
	let offset = 0;

	/** @type {Geoset[]} */
	const geosets = specs.map((spec) => {
		const v = spec.vertices;
		const t = spec.triangles;

		const positions = offset;
		chunks.push(new Float32Array(3 * v).fill(1).buffer);
		offset += 12 * v;

		const normals = offset;
		chunks.push(new Float32Array(3 * v).fill(0.5).buffer);
		offset += 12 * v;

		const uvs = [];
		for (let u = 0; u < spec.uvSets; u++) {
			uvs.push(offset);
			chunks.push(new Float32Array(2 * v).fill(0.25).buffer);
			offset += 8 * v;
		}

		const weights = offset;
		chunks.push(new Float32Array(MAX_INFLUENCES * v).fill(1).buffer);
		offset += 4 * MAX_INFLUENCES * v;

		const nodes = offset;
		chunks.push(new Uint32Array(MAX_INFLUENCES * v).fill(0).buffer);
		offset += 4 * MAX_INFLUENCES * v;

		const indices = offset;
		const n = 3 * t;
		const padded = n % 2 === 0 ? n : n + 1;
		const idx = new Uint16Array(padded);
		for (let i = 0; i < n; i++) idx[i] = i;
		chunks.push(idx.buffer);
		offset += 2 * padded;

		return {
			material: 0,
			lod: 0,
			vertices: v,
			triangles: t,
			priority: 0,
			alpha: spec.alpha,
			layers: [],
			mesh: { positions, normals, uvs, weights, nodes, indices },
		};
	});

	// A geoset with mesh: null (not at the model's lod), as the server sends.
	geosets.push({
		material: 0,
		lod: 1,
		vertices: 5,
		triangles: 3,
		priority: 0,
		alpha: 1,
		layers: [],
		mesh: null,
	});

	const buffer = new ArrayBuffer(offset);
	const out = new Uint8Array(buffer);
	let pos = 0;
	for (const c of chunks) {
		out.set(new Uint8Array(c), pos);
		pos += c.byteLength;
	}

	const model = /** @type {Model} */ (/** @type {unknown} */ ({ geosets }));
	return { model, buffer };
}

/**
 * offsetsOf is the lowest LOD's offsets, as the server's geosets[].mesh.
 * @param {Model} model
 * @returns {(?NonNullable<Geoset["mesh"]>)[]}
 */
function offsetsOf(model) {
	return model.geosets.map((g) => g.mesh);
}

test("decodeMesh: views equal the input values, null-mesh geosets skipped", () => {
	const { model, buffer } = buildMesh([
		{ vertices: 2, triangles: 1, uvSets: 2, alpha: 1 }, // odd index count, exercises padding
		{ vertices: 3, triangles: 2, uvSets: 0, alpha: 0 },
	]);

	const decoded = decodeMesh(model, buffer, offsetsOf(model));
	assert.equal(decoded.length, 2); // the third (mesh: null) geoset is skipped

	const a = decoded[0];
	assert.equal(a.index, 0);
	assert.equal(a.alpha, 1);
	assert.equal(a.vertices, 2);
	assert.deepEqual(Array.from(a.positions), [1, 1, 1, 1, 1, 1]);
	assert.deepEqual(Array.from(a.normals), [0.5, 0.5, 0.5, 0.5, 0.5, 0.5]);
	assert.equal(a.uvs.length, 2);
	assert.deepEqual(Array.from(a.uvs[0]), [0.25, 0.25, 0.25, 0.25]);
	assert.deepEqual(Array.from(a.uvs[1]), [0.25, 0.25, 0.25, 0.25]);
	assert.equal(a.weights.length, MAX_INFLUENCES * 2);
	assert.equal(a.nodes.length, MAX_INFLUENCES * 2);
	assert.deepEqual(Array.from(a.indices), [0, 1, 2]); // 3 indices, padding word not included

	const b = decoded[1];
	assert.equal(b.index, 1);
	assert.equal(b.alpha, 0);
	assert.equal(b.vertices, 3);
	assert.deepEqual(Array.from(b.indices), [0, 1, 2, 3, 4, 5]);
});

test("visibleBounds: excludes the alpha-0 geoset", () => {
	const { model, buffer } = buildMesh([
		{ vertices: 1, triangles: 1, uvSets: 0, alpha: 1 },
		{ vertices: 1, triangles: 1, uvSets: 0, alpha: 0 },
	]);
	// Give the two geosets distinct positions so bounds show which contributed.
	const decoded = decodeMesh(model, buffer, offsetsOf(model));
	decoded[0].positions.set([1, 2, 3]);
	decoded[1].positions.set([100, 200, 300]);

	const bounds = visibleBounds(decoded);
	assert.deepEqual(bounds, { min: [1, 2, 3], max: [1, 2, 3] });
});

test("visibleBounds: null when nothing is visible", () => {
	const { model, buffer } = buildMesh([{ vertices: 1, triangles: 1, uvSets: 0, alpha: 0 }]);
	const decoded = decodeMesh(model, buffer, offsetsOf(model));
	assert.equal(visibleBounds(decoded), null);
});

test("decodeMesh: out-of-range offsets throw", () => {
	const { model, buffer } = buildMesh([{ vertices: 1, triangles: 1, uvSets: 0, alpha: 1 }]);
	/** @type {NonNullable<Geoset["mesh"]>} */ (model.geosets[0].mesh).positions = buffer.byteLength + 100;
	assert.throws(() => decodeMesh(model, buffer, offsetsOf(model)), /geoset 0/);
});

test("decodeMesh: explicit offsets pick the geosets", () => {
	const { model, buffer } = buildMesh([
		{ vertices: 1, triangles: 1, uvSets: 0, alpha: 1 },
		{ vertices: 2, triangles: 1, uvSets: 0, alpha: 1 },
	]);
	const offsets = [null, model.geosets[1].mesh, null];
	const decoded = decodeMesh(model, buffer, offsets);
	assert.equal(decoded.length, 1);
	assert.equal(decoded[0].index, 1);
	assert.equal(decoded[0].vertices, 2);
});

test("pickLOD: match, fallback to the lowest, empty", () => {
	const lods = /** @type {import("../static/js/api.js").LOD[]} */ (/** @type {unknown} */ ([{ lod: 0 }, { lod: 1 }, { lod: 3 }]));
	assert.equal(pickLOD(lods, 3), 2);
	assert.equal(pickLOD(lods, 0), 0);
	assert.equal(pickLOD(lods, 2), 0);
	assert.equal(pickLOD([], 0), -1);
});

test("lodLabel", () => {
	const l = /** @type {import("../static/js/api.js").LOD} */ (/** @type {unknown} */ ({ lod: 3, vertices: 6125, triangles: 7018 }));
	assert.equal(lodLabel(l), "LOD 3: 6125 vertices, 7018 triangles");
});
