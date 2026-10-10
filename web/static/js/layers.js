// Per-layer GL state and geoset draw order, ported from viewer/mesh.go's
// filterGLState and uploadMesh/draw's sort and defaults (shading bit
// overrides, the empty-layers grey fallback).

/** @typedef {import("./api.js").Layer} Layer */
/** @typedef {import("./api.js").Geoset} Geoset */

// mdx.FilterMode values (model/mdx/material.go).
export const FILTER_NONE = 0;
export const FILTER_TRANSPARENT = 1;
export const FILTER_BLEND = 2;
export const FILTER_ADDITIVE = 3;
export const FILTER_ADD_ALPHA = 4;
export const FILTER_MODULATE = 5;
export const FILTER_MODULATE2X = 6;

// Fixed GL enum values (WebGL2RenderingContext constants), usable without a
// context so this module's tests don't need one.
export const ZERO = 0;
export const ONE = 1;
export const SRC_COLOR = 0x0300;
export const SRC_ALPHA = 0x0302;
export const ONE_MINUS_SRC_ALPHA = 0x0303;
export const DST_COLOR = 0x0306;

/**
 * @typedef {Object} LayerState
 * @property {boolean} blend
 * @property {number} src
 * @property {number} dst
 * @property {boolean} depthWrite
 * @property {boolean} depthTest
 * @property {boolean} alphaTest
 * @property {boolean} unshaded
 */

/**
 * layerState maps a layer's filter mode to its blend factors and depth
 * write (filterGLState), then applies shading's bit 6 (no depth test) and
 * bit 7 (no depth write) overrides, and derives alphaTest (filter
 * transparent) and unshaded (shading bit 0). An unrecognized filter mode
 * draws as FILTER_NONE, same as the desktop.
 * @param {Layer} layer
 * @returns {LayerState}
 */
export function layerState(layer) {
	/** @type {{blend: boolean, src: number, dst: number, depthWrite: boolean}} */
	let base;
	switch (layer.filter) {
		case FILTER_TRANSPARENT:
			base = { blend: false, src: ZERO, dst: ZERO, depthWrite: true };
			break;
		case FILTER_BLEND:
			base = { blend: true, src: SRC_ALPHA, dst: ONE_MINUS_SRC_ALPHA, depthWrite: false };
			break;
		case FILTER_ADDITIVE:
			base = { blend: true, src: ONE, dst: ONE, depthWrite: false };
			break;
		case FILTER_ADD_ALPHA:
			base = { blend: true, src: SRC_ALPHA, dst: ONE, depthWrite: false };
			break;
		case FILTER_MODULATE:
			base = { blend: true, src: ZERO, dst: SRC_COLOR, depthWrite: false };
			break;
		case FILTER_MODULATE2X:
			base = { blend: true, src: DST_COLOR, dst: SRC_COLOR, depthWrite: false };
			break;
		default: // FILTER_NONE and anything unrecognized
			base = { blend: false, src: ZERO, dst: ZERO, depthWrite: true };
			break;
	}

	let depthWrite = base.depthWrite;
	if (layer.shading & (1 << 7)) {
		depthWrite = false;
	}
	const depthTest = (layer.shading & (1 << 6)) === 0;

	return {
		blend: base.blend,
		src: base.src,
		dst: base.dst,
		depthWrite,
		depthTest,
		alphaTest: layer.filter === FILTER_TRANSPARENT,
		unshaded: (layer.shading & 1) !== 0,
	};
}

/**
 * grayLayer is the single opaque layer a geoset with no layers draws, as
 * uploadMesh substitutes for an empty or out-of-range material.
 * @returns {Layer}
 */
function grayLayer() {
	return { filter: FILTER_NONE, shading: 0, alpha: 1, uvSet: 0, wrapWidth: false, wrapHeight: false, source: "grey", label: "", texture: null, mask: null };
}

/**
 * geosetLayers returns geoset's layers, or a single flat-grey opaque layer
 * when it has none.
 * @param {Geoset} geoset
 * @returns {Layer[]}
 */
export function geosetLayers(geoset) {
	if (geoset.layers.length === 0) {
		return [grayLayer()];
	}
	return geoset.layers;
}

/**
 * isOpaque reports whether a geoset draws opaque: its first layer's filter
 * is none or transparent (uploadMesh's gg.opaque).
 * @param {Layer[]} layers
 * @returns {boolean}
 */
export function isOpaque(layers) {
	return layers[0].filter === FILTER_NONE || layers[0].filter === FILTER_TRANSPARENT;
}

/**
 * drawOrder returns indices into geosets in draw order: opaque geosets
 * first, then blended, each group ordered by priority plane ascending,
 * stable on geoset index (mirrors uploadMesh's sort.SliceStable).
 * @param {Geoset[]} geosets
 * @returns {number[]}
 */
export function drawOrder(geosets) {
	const indices = geosets.map((_, i) => i);
	const opaque = geosets.map((g) => isOpaque(geosetLayers(g)));
	indices.sort((a, b) => {
		if (opaque[a] !== opaque[b]) {
			return opaque[a] ? -1 : 1;
		}
		if (geosets[a].priority !== geosets[b].priority) {
			return geosets[a].priority - geosets[b].priority;
		}
		return a - b;
	});
	return indices;
}
