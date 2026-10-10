import test from "node:test";
import assert from "node:assert/strict";
import { layerState, geosetLayers, isOpaque, drawOrder, FILTER_NONE, FILTER_TRANSPARENT, FILTER_BLEND, FILTER_ADDITIVE, FILTER_ADD_ALPHA, FILTER_MODULATE, FILTER_MODULATE2X, ZERO, ONE, SRC_COLOR, SRC_ALPHA, ONE_MINUS_SRC_ALPHA, DST_COLOR } from "../static/js/layers.js";

/** @typedef {import("../static/js/api.js").Layer} Layer */
/** @typedef {import("../static/js/api.js").Geoset} Geoset */

/**
 * layer builds a Layer with defaults, overridden by opts.
 * @param {Partial<Layer>} opts
 * @returns {Layer}
 */
function layer(opts) {
	return { filter: FILTER_NONE, shading: 0, alpha: 1, uvSet: 0, wrapWidth: false, wrapHeight: false, source: "texture", label: "", texture: null, mask: null, ...opts };
}

test("layerState: none -> no blend, depth write", () => {
	assert.deepEqual(layerState(layer({ filter: FILTER_NONE })), { blend: false, src: ZERO, dst: ZERO, depthWrite: true, depthTest: true, alphaTest: false, unshaded: false });
});

test("layerState: transparent -> no blend, depth write, alpha test", () => {
	assert.deepEqual(layerState(layer({ filter: FILTER_TRANSPARENT })), { blend: false, src: ZERO, dst: ZERO, depthWrite: true, depthTest: true, alphaTest: true, unshaded: false });
});

test("layerState: blend -> SRC_ALPHA/ONE_MINUS_SRC_ALPHA, no depth write", () => {
	const s = layerState(layer({ filter: FILTER_BLEND }));
	assert.equal(s.blend, true);
	assert.equal(s.src, SRC_ALPHA);
	assert.equal(s.dst, ONE_MINUS_SRC_ALPHA);
	assert.equal(s.depthWrite, false);
});

test("layerState: additive -> ONE/ONE", () => {
	const s = layerState(layer({ filter: FILTER_ADDITIVE }));
	assert.equal(s.blend, true);
	assert.equal(s.src, ONE);
	assert.equal(s.dst, ONE);
});

test("layerState: add alpha -> SRC_ALPHA/ONE", () => {
	const s = layerState(layer({ filter: FILTER_ADD_ALPHA }));
	assert.equal(s.src, SRC_ALPHA);
	assert.equal(s.dst, ONE);
});

test("layerState: modulate -> ZERO/SRC_COLOR", () => {
	const s = layerState(layer({ filter: FILTER_MODULATE }));
	assert.equal(s.src, ZERO);
	assert.equal(s.dst, SRC_COLOR);
});

test("layerState: modulate 2x -> DST_COLOR/SRC_COLOR", () => {
	const s = layerState(layer({ filter: FILTER_MODULATE2X }));
	assert.equal(s.src, DST_COLOR);
	assert.equal(s.dst, SRC_COLOR);
});

test("layerState: unknown filter mode draws as none", () => {
	const s = layerState(layer({ filter: 99 }));
	assert.equal(s.blend, false);
	assert.equal(s.depthWrite, true);
	assert.equal(s.alphaTest, false);
});

test("layerState: shading bit 0 -> unshaded", () => {
	assert.equal(layerState(layer({ shading: 1 })).unshaded, true);
	assert.equal(layerState(layer({ shading: 0 })).unshaded, false);
});

test("layerState: shading bit 6 -> no depth test", () => {
	assert.equal(layerState(layer({ shading: 1 << 6 })).depthTest, false);
	assert.equal(layerState(layer({ shading: 0 })).depthTest, true);
});

test("layerState: shading bit 7 -> no depth write, even for filter none", () => {
	assert.equal(layerState(layer({ filter: FILTER_NONE, shading: 1 << 7 })).depthWrite, false);
});

test("layerState: shading bit 7 on an already-no-depth-write blend filter stays false", () => {
	assert.equal(layerState(layer({ filter: FILTER_BLEND, shading: 1 << 7 })).depthWrite, false);
});

/** @param {Partial<Geoset>} opts */
function geoset(opts) {
	return { material: 0, lod: 0, vertices: 0, triangles: 0, priority: 0, alpha: 1, layers: [], mesh: null, ...opts };
}

test("geosetLayers: no layers -> one grey opaque layer", () => {
	const layers = geosetLayers(geoset({ layers: [] }));
	assert.equal(layers.length, 1);
	assert.equal(layers[0].filter, FILTER_NONE);
	assert.equal(layers[0].alpha, 1);
	assert.equal(layers[0].source, "grey");
});

test("geosetLayers: passes through existing layers", () => {
	const ls = [layer({ filter: FILTER_BLEND })];
	assert.equal(geosetLayers(geoset({ layers: ls })), ls);
});

test("isOpaque: first layer none or transparent is opaque", () => {
	assert.equal(isOpaque([layer({ filter: FILTER_NONE })]), true);
	assert.equal(isOpaque([layer({ filter: FILTER_TRANSPARENT })]), true);
	assert.equal(isOpaque([layer({ filter: FILTER_BLEND })]), false);
	assert.equal(isOpaque([layer({ filter: FILTER_ADDITIVE })]), false);
});

test("drawOrder: opaque first, then priority ascending, stable on ties", () => {
	const geosets = [
		geoset({ layers: [layer({ filter: FILTER_BLEND })], priority: 5 }), // 0: blended, prio 5
		geoset({ layers: [layer({ filter: FILTER_NONE })], priority: 2 }), // 1: opaque, prio 2
		geoset({ layers: [layer({ filter: FILTER_BLEND })], priority: 1 }), // 2: blended, prio 1
		geoset({ layers: [layer({ filter: FILTER_TRANSPARENT })], priority: 2 }), // 3: opaque, prio 2 (tie with 1)
		geoset({ layers: [], priority: -1 }), // 4: no layers -> grey opaque, prio -1
	];
	assert.deepEqual(drawOrder(geosets), [4, 1, 3, 2, 0]);
});
