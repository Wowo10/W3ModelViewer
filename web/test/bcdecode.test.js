import test from "node:test";
import assert from "node:assert/strict";
import { decodeBC1, decodeBC3, decodeBC5, decodeMip } from "../static/js/bcdecode.js";

/**
 * colorBlock builds an 8-byte BC1 colour block: two RGB565 colours (raw
 * 16-bit values, little-endian) and 16 2-bit indices (one per texel,
 * row-major, packed little-endian into a uint32).
 * @param {number} c0
 * @param {number} c1
 * @param {number[]} indices - 16 values in [0,3]
 * @returns {Uint8Array} 8 bytes
 */
function colorBlock(c0, c1, indices) {
	const out = new Uint8Array(8);
	const view = new DataView(out.buffer);
	view.setUint16(0, c0, true);
	view.setUint16(2, c1, true);
	let bits = 0;
	for (let i = 0; i < 16; i++) {
		bits |= indices[i] << (2 * i);
	}
	view.setUint32(4, bits >>> 0, true);
	return out;
}

/**
 * alphaBlock builds an 8-byte BC3/BC4 alpha (or single-channel) block: two
 * endpoints and 16 3-bit indices, row-major.
 * @param {number} a0
 * @param {number} a1
 * @param {number[]} indices - 16 values in [0,7]
 * @returns {Uint8Array} 8 bytes
 */
function alphaBlock(a0, a1, indices) {
	const out = new Uint8Array(8);
	out[0] = a0;
	out[1] = a1;
	let low24 = 0;
	for (let i = 0; i < 8; i++) {
		low24 |= indices[i] << (3 * i);
	}
	let high24 = 0;
	for (let i = 0; i < 8; i++) {
		high24 |= indices[8 + i] << (3 * i);
	}
	out[2] = low24 & 0xff;
	out[3] = (low24 >> 8) & 0xff;
	out[4] = (low24 >> 16) & 0xff;
	out[5] = high24 & 0xff;
	out[6] = (high24 >> 8) & 0xff;
	out[7] = (high24 >> 16) & 0xff;
	return out;
}

/**
 * concat joins Uint8Arrays into one.
 * @param {Uint8Array[]} parts
 * @returns {Uint8Array}
 */
function concat(parts) {
	const total = parts.reduce((n, p) => n + p.length, 0);
	const out = new Uint8Array(total);
	let o = 0;
	for (const p of parts) {
		out.set(p, o);
		o += p.length;
	}
	return out;
}

/**
 * pixel reads the RGBA texel at (x, y) of a w*h*4 RGBA8 buffer.
 * @param {Uint8Array} out
 * @param {number} w
 * @param {number} x
 * @param {number} y
 * @returns {[number, number, number, number]}
 */
function pixel(out, w, x, y) {
	const o = (y * w + x) * 4;
	return [out[o], out[o + 1], out[o + 2], out[o + 3]];
}

// Black (raw 0) and white (raw 0xFFFF): 255 divides by 3 exactly (85, 170).
const BLACK = 0;
const WHITE = 0xffff;

test("decodeBC1: 4-colour mode (c0 > c1), every index", () => {
	const indices = new Array(16).fill(0);
	indices[1] = 1;
	indices[2] = 2;
	indices[3] = 3;
	const block = colorBlock(WHITE, BLACK, indices); // c0=white > c1=black numerically
	const out = decodeBC1(block, 4, 4);

	assert.deepEqual(pixel(out, 4, 0, 0), [255, 255, 255, 255]); // index 0: c0
	assert.deepEqual(pixel(out, 4, 1, 0), [0, 0, 0, 255]); // index 1: c1
	assert.deepEqual(pixel(out, 4, 2, 0), [170, 170, 170, 255]); // index 2: (2c0+c1)/3
	assert.deepEqual(pixel(out, 4, 3, 0), [85, 85, 85, 255]); // index 3: (c0+2c1)/3
});

test("decodeBC1: 3-colour + transparent mode (c0 <= c1), every index incl. transparent", () => {
	// c0 = (0,0,0) raw 0; c1 = r5=8,g6=8,b5=8 -> (66,32,66), raw > 0.
	const c1raw = (8 << 11) | (8 << 5) | 8;
	const indices = new Array(16).fill(0);
	indices[1] = 1;
	indices[2] = 2;
	indices[3] = 3;
	const block = colorBlock(BLACK, c1raw, indices);
	const out = decodeBC1(block, 4, 4);

	assert.deepEqual(pixel(out, 4, 0, 0), [0, 0, 0, 255]); // index 0: c0
	assert.deepEqual(pixel(out, 4, 1, 0), [66, 32, 66, 255]); // index 1: c1
	assert.deepEqual(pixel(out, 4, 2, 0), [33, 16, 33, 255]); // index 2: (c0+c1)/2
	assert.deepEqual(pixel(out, 4, 3, 0), [0, 0, 0, 0]); // index 3: transparent black
});

test("decodeBC1: 8x4 multi-block layout, second block at x 4..7", () => {
	const allZero = new Array(16).fill(0);
	const block0 = colorBlock(WHITE, BLACK, allZero); // solid white
	const block1 = colorBlock(BLACK, WHITE, [...new Array(15).fill(0), 3]); // mostly black, last texel transparent
	const data = concat([block0, block1]);
	const out = decodeBC1(data, 8, 4);

	assert.deepEqual(pixel(out, 8, 0, 0), [255, 255, 255, 255]);
	assert.deepEqual(pixel(out, 8, 3, 3), [255, 255, 255, 255]);
	assert.deepEqual(pixel(out, 8, 4, 0), [0, 0, 0, 255]);
	assert.deepEqual(pixel(out, 8, 7, 3), [0, 0, 0, 0]); // local index 15 -> transparent
});

test("decodeBC1: clipping a 6x6 image (partial blocks)", () => {
	const allZero = new Array(16).fill(0);
	const blocks = [colorBlock(WHITE, BLACK, allZero), colorBlock(WHITE, BLACK, allZero), colorBlock(WHITE, BLACK, allZero), colorBlock(WHITE, BLACK, allZero)];
	const out = decodeBC1(concat(blocks), 6, 6);
	assert.equal(out.length, 6 * 6 * 4);
	assert.deepEqual(pixel(out, 6, 5, 5), [255, 255, 255, 255]);
});

test("decodeBC1: clipping a 1x1 image", () => {
	const out = decodeBC1(colorBlock(WHITE, BLACK, new Array(16).fill(1)), 1, 1);
	assert.equal(out.length, 4);
	assert.deepEqual(pixel(out, 1, 0, 0), [0, 0, 0, 255]); // index 1: c1
});

test("decodeBC1: clipping a 2x2 image", () => {
	const indices = new Array(16).fill(0);
	indices[1] = 1;
	indices[4] = 2;
	indices[5] = 3;
	const out = decodeBC1(colorBlock(WHITE, BLACK, indices), 2, 2);
	assert.equal(out.length, 2 * 2 * 4);
	assert.deepEqual(pixel(out, 2, 0, 0), [255, 255, 255, 255]);
	assert.deepEqual(pixel(out, 2, 1, 0), [0, 0, 0, 255]);
});

test("decodeBC3: 8-alpha mode (a0 > a1), every index", () => {
	const aIdx = new Array(16).fill(0);
	for (let i = 0; i < 8; i++) aIdx[i] = i;
	const alpha = alphaBlock(7, 0, aIdx); // divides exactly by 7
	const color = colorBlock(WHITE, BLACK, new Array(16).fill(0));
	const out = decodeBC3(concat([alpha, color]), 4, 4);

	const expectedA = [7, 0, 6, 5, 4, 3, 2, 1];
	for (let i = 0; i < 8; i++) {
		assert.equal(pixel(out, 4, i % 4, Math.floor(i / 4))[3], expectedA[i], `texel ${i}`);
	}
});

test("decodeBC3: 6-alpha + 0/255 mode (a0 <= a1), both special codes", () => {
	const aIdx = new Array(16).fill(0);
	aIdx[0] = 6; // -> 0
	aIdx[1] = 7; // -> 255
	aIdx[2] = 2;
	aIdx[3] = 3;
	aIdx[4] = 4;
	aIdx[5] = 5;
	const alpha = alphaBlock(0, 5, aIdx); // divides exactly by 5
	const color = colorBlock(WHITE, BLACK, new Array(16).fill(0));
	const out = decodeBC3(concat([alpha, color]), 4, 4);

	const expectedA = [0, 255, 1, 2, 3, 4];
	for (let i = 0; i < 6; i++) {
		assert.equal(pixel(out, 4, i, 0)[3], expectedA[i], `texel ${i}`);
	}
});

test("decodeBC3: colour block is always 4-colour, even when c0 <= c1", () => {
	const r1 = (8 << 11) | (12 << 5) | 8; // > 0, so c0=0 <= c1=r1
	const indices = new Array(16).fill(0);
	indices[1] = 1;
	indices[2] = 2;
	indices[3] = 3;
	const color = colorBlock(BLACK, r1, indices);
	const alpha = alphaBlock(255, 255, new Array(16).fill(0));
	const out = decodeBC3(concat([alpha, color]), 4, 4);

	assert.deepEqual(pixel(out, 4, 0, 0).slice(0, 3), [0, 0, 0]);
	assert.deepEqual(pixel(out, 4, 1, 0).slice(0, 3), [66, 48, 66]);
	assert.deepEqual(pixel(out, 4, 2, 0).slice(0, 3), [22, 16, 22]); // (2c0+c1)/3, not (c0+c1)/2
	assert.deepEqual(pixel(out, 4, 3, 0).slice(0, 3), [44, 32, 44]); // (c0+2c1)/3
});

test("decodeBC5: red and green channels independently, 0 blue 255 alpha", () => {
	const redIdx = new Array(16).fill(0);
	redIdx[0] = 2; // (6*7+0)/7 = 6
	const greenIdx = new Array(16).fill(1); // a1 = 5
	const red = alphaBlock(7, 0, redIdx);
	const green = alphaBlock(0, 5, greenIdx);
	const out = decodeBC5(concat([red, green]), 4, 4);

	assert.deepEqual(pixel(out, 4, 0, 0), [6, 5, 0, 255]);
	assert.deepEqual(pixel(out, 4, 1, 0), [7, 5, 0, 255]); // index 0 -> a0=7
});

test("decodeMip dispatches on format", () => {
	const block = colorBlock(WHITE, BLACK, new Array(16).fill(0));
	assert.deepEqual(decodeMip("DXT1", block, 4, 4), decodeBC1(block, 4, 4));
	assert.throws(() => decodeMip(/** @type {any} */ ("BOGUS"), block, 4, 4), /unknown format/);
});
