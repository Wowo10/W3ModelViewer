// Parses a DDS header and lays out its mips, mirroring
// model/texture/dds.go's ReadDDS exactly (same fields, same panics as
// thrown Errors prefixed "dds: ").

/** @typedef {"DXT1" | "DXT5" | "ATI2"} Format */

/**
 * @typedef {Object} Mip
 * @property {number} offset
 * @property {number} size
 * @property {number} width
 * @property {number} height
 */

/**
 * @typedef {Object} DDS
 * @property {Format} format
 * @property {number} width
 * @property {number} height
 * @property {Mip[]} mips
 */

const HEADER_SIZE = 128;
const DDSD_MIPMAPCOUNT = 0x20000; // flags bit: dwMipMapCount is valid
const DDPF_FOURCC = 0x4; // pixel format flags bit: dwFourCC is valid

/** @type {Record<string, number>} */
const BLOCK_BYTES = { DXT1: 8, DXT5: 16, ATI2: 16 };

/**
 * parseDDS parses a DDS header out of buffer and lays out its mips, with the
 * same rules and errors (thrown as "dds: ..." Errors) as ReadDDS: too short
 * or missing the "DDS " magic, header size not 124, no FourCC bit, an
 * unsupported FourCC (including DX10), caps2 not 0, a zero dimension, or a
 * mip running past the end of buffer. Trailing bytes after the last mip are
 * ignored.
 * @param {ArrayBuffer} buffer
 * @returns {DDS}
 */
export function parseDDS(buffer) {
	if (buffer.byteLength < HEADER_SIZE) {
		throw new Error("dds: too short or missing \"DDS \" magic");
	}
	const view = new DataView(buffer);
	const magic = String.fromCharCode(view.getUint8(0), view.getUint8(1), view.getUint8(2), view.getUint8(3));
	if (magic !== "DDS ") {
		throw new Error("dds: too short or missing \"DDS \" magic");
	}

	const size = view.getUint32(4, true);
	if (size !== 124) {
		throw new Error(`dds: header size ${size}, want 124`);
	}

	const flags = view.getUint32(8, true);
	const height = view.getUint32(12, true);
	const width = view.getUint32(16, true);
	let mipCount = view.getUint32(28, true);
	const pfFlags = view.getUint32(80, true);
	const fourCC = String.fromCharCode(view.getUint8(84), view.getUint8(85), view.getUint8(86), view.getUint8(87));
	const caps2 = view.getUint32(112, true);

	if (width === 0 || height === 0) {
		throw new Error(`dds: zero dimension ${width}x${height}`);
	}
	if (caps2 !== 0) {
		throw new Error(`dds: caps2 0x${caps2.toString(16)}, cube maps and volumes aren't supported`);
	}
	if ((pfFlags & DDPF_FOURCC) === 0) {
		throw new Error(`dds: pixel format flags 0x${pfFlags.toString(16)}, want the FourCC bit set`);
	}

	/** @type {Format} */
	let format;
	switch (fourCC) {
		case "DXT1":
			format = "DXT1";
			break;
		case "DXT5":
			format = "DXT5";
			break;
		case "ATI2":
			format = "ATI2";
			break;
		default:
			throw new Error(`dds: unsupported FourCC "${fourCC}"`);
	}

	if ((flags & DDSD_MIPMAPCOUNT) === 0 || mipCount === 0) {
		mipCount = 1;
	}

	const bytesPerBlock = BLOCK_BYTES[format];
	/** @type {Mip[]} */
	const mips = [];
	let offset = HEADER_SIZE;
	let w = width;
	let h = height;
	for (let i = 0; i < mipCount; i++) {
		const blocksWide = Math.max(1, Math.ceil(w / 4));
		const blocksHigh = Math.max(1, Math.ceil(h / 4));
		const mipSize = blocksWide * blocksHigh * bytesPerBlock;

		if (offset + mipSize > buffer.byteLength) {
			throw new Error(`dds: mip ${i} (${mipSize} bytes at offset ${offset}) runs past end of data (${buffer.byteLength} bytes)`);
		}

		mips.push({ offset, size: mipSize, width: w, height: h });
		offset += mipSize;

		w = Math.max(1, Math.floor(w / 2));
		h = Math.max(1, Math.floor(h / 2));
	}

	return { format, width, height, mips };
}

/**
 * blockUploadable checks every mip against the WebGL compressed-texture
 * upload rule: level 0's width and height must be a multiple of 4; deeper
 * levels must be a multiple of 4, or exactly 1 or 2 (the implicit
 * partial-block levels at the bottom of a full mip chain). Returns the
 * first offending mip, or null when every mip is uploadable.
 * @param {DDS} dds
 * @returns {{level: number, width: number, height: number} | null}
 */
export function blockUploadable(dds) {
	for (let level = 0; level < dds.mips.length; level++) {
		const m = dds.mips[level];
		/** @param {number} dim */
		const ok = (dim) => dim % 4 === 0 || (level > 0 && (dim === 1 || dim === 2));
		if (!ok(m.width) || !ok(m.height)) {
			return { level, width: m.width, height: m.height };
		}
	}
	return null;
}
