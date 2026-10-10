// CPU decoders for the three block-compressed formats model/texture.ReadDDS
// recognizes (DXT1/BC1, DXT5/BC3, ATI2/BC5), used when a WebGL compressed
// texture extension is missing or a mip's size breaks the upload rule (see
// dds.js's blockUploadable). Each decodes to RGBA8, matching how the GL
// extensions sample the same bits (RGBA_S3TC_DXT1 draws a 3-colour block's
// unused index as transparent black; RG_RGTC2 reads b=0, a=1).

/**
 * rgb565to888 expands a 5/6/5-bit packed colour to 8 bits per channel by bit
 * replication, as the S3TC decoders do.
 * @param {number} c565
 * @returns {[number, number, number]}
 */
function rgb565to888(c565) {
	const r5 = (c565 >> 11) & 0x1f;
	const g6 = (c565 >> 5) & 0x3f;
	const b5 = c565 & 0x1f;
	const r = (r5 << 3) | (r5 >> 2);
	const g = (g6 << 2) | (g6 >> 4);
	const b = (b5 << 3) | (b5 >> 2);
	return [r, g, b];
}

/**
 * decodeColorBlock decodes a BC1-layout 8-byte colour block (two RGB565
 * colours + 16 2-bit indices) into 16 RGBA bytes (one texel per row-major
 * position 0..15). When forceFourColor is true, the 4-colour interpolation
 * is used regardless of c0 vs c1 (BC3/BC5's colour block never has a
 * transparent index); otherwise c0 <= c1 selects BC1's 3-colour +
 * transparent-black mode.
 * @param {DataView} view
 * @param {number} offset
 * @param {boolean} forceFourColor
 * @returns {Uint8Array} 16*4 bytes, alpha 255 except BC1's transparent index
 */
function decodeColorBlock(view, offset, forceFourColor) {
	const c0 = view.getUint16(offset, true);
	const c1 = view.getUint16(offset + 2, true);
	const bits = view.getUint32(offset + 4, true);

	const [r0, g0, b0] = rgb565to888(c0);
	const [r1, g1, b1] = rgb565to888(c1);

	/** @type {[number, number, number, number][]} */
	let colors;
	if (forceFourColor || c0 > c1) {
		colors = [
			[r0, g0, b0, 255],
			[r1, g1, b1, 255],
			[Math.round((2 * r0 + r1) / 3), Math.round((2 * g0 + g1) / 3), Math.round((2 * b0 + b1) / 3), 255],
			[Math.round((r0 + 2 * r1) / 3), Math.round((g0 + 2 * g1) / 3), Math.round((b0 + 2 * b1) / 3), 255],
		];
	} else {
		colors = [
			[r0, g0, b0, 255],
			[r1, g1, b1, 255],
			[Math.round((r0 + r1) / 2), Math.round((g0 + g1) / 2), Math.round((b0 + b1) / 2), 255],
			[0, 0, 0, 0],
		];
	}

	const out = new Uint8Array(16 * 4);
	for (let i = 0; i < 16; i++) {
		const idx = (bits >> (2 * i)) & 3;
		const [r, g, b, a] = colors[idx];
		out[i * 4] = r;
		out[i * 4 + 1] = g;
		out[i * 4 + 2] = b;
		out[i * 4 + 3] = a;
	}
	return out;
}

/**
 * decodeAlphaBlock decodes a BC3/BC4-layout 8-byte alpha (or single-channel)
 * block: two endpoints plus 16 3-bit indices, 8-value or 6-value+0/255
 * interpolation depending on a0 vs a1.
 * @param {DataView} view
 * @param {number} offset
 * @returns {Uint8Array} 16 values, one per texel, row-major
 */
function decodeAlphaBlock(view, offset) {
	const a0 = view.getUint8(offset);
	const a1 = view.getUint8(offset + 1);

	/** @type {number[]} */
	let values;
	if (a0 > a1) {
		values = [
			a0,
			a1,
			Math.round((6 * a0 + 1 * a1) / 7),
			Math.round((5 * a0 + 2 * a1) / 7),
			Math.round((4 * a0 + 3 * a1) / 7),
			Math.round((3 * a0 + 4 * a1) / 7),
			Math.round((2 * a0 + 5 * a1) / 7),
			Math.round((1 * a0 + 6 * a1) / 7),
		];
	} else {
		values = [a0, a1, Math.round((4 * a0 + 1 * a1) / 5), Math.round((3 * a0 + 2 * a1) / 5), Math.round((2 * a0 + 3 * a1) / 5), Math.round((1 * a0 + 4 * a1) / 5), 0, 255];
	}

	const low24 = view.getUint8(offset + 2) | (view.getUint8(offset + 3) << 8) | (view.getUint8(offset + 4) << 16);
	const high24 = view.getUint8(offset + 5) | (view.getUint8(offset + 6) << 8) | (view.getUint8(offset + 7) << 16);

	const out = new Uint8Array(16);
	for (let i = 0; i < 8; i++) {
		out[i] = values[(low24 >> (3 * i)) & 7];
	}
	for (let i = 0; i < 8; i++) {
		out[8 + i] = values[(high24 >> (3 * i)) & 7];
	}
	return out;
}

/**
 * blitBlock writes a decoded 4x4xchannels block into out (w*h*4 RGBA8),
 * clipping texels past the image edge (a partial block at the bottom/right
 * of a non-multiple-of-4 mip).
 * @param {Uint8Array} out
 * @param {number} w
 * @param {number} h
 * @param {number} bx - block column
 * @param {number} by - block row
 * @param {(local: number) => [number, number, number, number]} texel - local 0..15, row-major
 */
function blitBlock(out, w, h, bx, by, texel) {
	for (let ty = 0; ty < 4; ty++) {
		const y = by * 4 + ty;
		if (y >= h) {
			continue;
		}
		for (let tx = 0; tx < 4; tx++) {
			const x = bx * 4 + tx;
			if (x >= w) {
				continue;
			}
			const [r, g, b, a] = texel(ty * 4 + tx);
			const o = (y * w + x) * 4;
			out[o] = r;
			out[o + 1] = g;
			out[o + 2] = b;
			out[o + 3] = a;
		}
	}
}

/**
 * decodeBC1 decodes a BC1/DXT1 mip (8 bytes per 4x4 block, RGBA_S3TC_DXT1
 * semantics: 3-colour blocks' unused index is transparent black).
 * @param {Uint8Array} data
 * @param {number} w
 * @param {number} h
 * @returns {Uint8Array} w*h*4 RGBA8
 */
export function decodeBC1(data, w, h) {
	const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
	const blocksWide = Math.max(1, Math.ceil(w / 4));
	const blocksHigh = Math.max(1, Math.ceil(h / 4));
	const out = new Uint8Array(w * h * 4);

	let offset = 0;
	for (let by = 0; by < blocksHigh; by++) {
		for (let bx = 0; bx < blocksWide; bx++) {
			const block = decodeColorBlock(view, offset, false);
			blitBlock(out, w, h, bx, by, (local) => [block[local * 4], block[local * 4 + 1], block[local * 4 + 2], block[local * 4 + 3]]);
			offset += 8;
		}
	}
	return out;
}

/**
 * decodeBC3 decodes a BC3/DXT5 mip (16 bytes per 4x4 block: 8-byte alpha
 * block, 8-byte colour block always in 4-colour mode).
 * @param {Uint8Array} data
 * @param {number} w
 * @param {number} h
 * @returns {Uint8Array} w*h*4 RGBA8
 */
export function decodeBC3(data, w, h) {
	const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
	const blocksWide = Math.max(1, Math.ceil(w / 4));
	const blocksHigh = Math.max(1, Math.ceil(h / 4));
	const out = new Uint8Array(w * h * 4);

	let offset = 0;
	for (let by = 0; by < blocksHigh; by++) {
		for (let bx = 0; bx < blocksWide; bx++) {
			const alpha = decodeAlphaBlock(view, offset);
			const color = decodeColorBlock(view, offset + 8, true);
			blitBlock(out, w, h, bx, by, (local) => [color[local * 4], color[local * 4 + 1], color[local * 4 + 2], alpha[local]]);
			offset += 16;
		}
	}
	return out;
}

/**
 * decodeBC5 decodes an ATI2/BC5 mip (16 bytes per 4x4 block: red then green
 * BC4 channel blocks) to (r, g, 0, 255), matching RG_RGTC2's b=0, a=1.
 * @param {Uint8Array} data
 * @param {number} w
 * @param {number} h
 * @returns {Uint8Array} w*h*4 RGBA8
 */
export function decodeBC5(data, w, h) {
	const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
	const blocksWide = Math.max(1, Math.ceil(w / 4));
	const blocksHigh = Math.max(1, Math.ceil(h / 4));
	const out = new Uint8Array(w * h * 4);

	let offset = 0;
	for (let by = 0; by < blocksHigh; by++) {
		for (let bx = 0; bx < blocksWide; bx++) {
			const red = decodeAlphaBlock(view, offset);
			const green = decodeAlphaBlock(view, offset + 8);
			blitBlock(out, w, h, bx, by, (local) => [red[local], green[local], 0, 255]);
			offset += 16;
		}
	}
	return out;
}

/**
 * decodeMip dispatches to the decoder matching format.
 * @param {import("./dds.js").Format} format
 * @param {Uint8Array} bytes
 * @param {number} w
 * @param {number} h
 * @returns {Uint8Array} w*h*4 RGBA8
 */
export function decodeMip(format, bytes, w, h) {
	switch (format) {
		case "DXT1":
			return decodeBC1(bytes, w, h);
		case "DXT5":
			return decodeBC3(bytes, w, h);
		case "ATI2":
			return decodeBC5(bytes, w, h);
		default:
			throw new Error(`bcdecode: unknown format "${format}"`);
	}
}
