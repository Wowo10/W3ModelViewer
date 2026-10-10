import test from "node:test";
import assert from "node:assert/strict";
import { parseDDS, blockUploadable } from "../static/js/dds.js";

const HEADER_SIZE = 128;
const DDSD_MIPMAPCOUNT = 0x20000;
const DDPF_FOURCC = 0x4;

/**
 * makeDDS builds DDS bytes with one byte per mip (not block-sized, since
 * parseDDS computes sizes from dimensions) so tests can check the parsed
 * layout and leave out pixel data entirely.
 * @param {Object} opts
 * @param {string} [opts.fourCC]
 * @param {number} opts.width
 * @param {number} opts.height
 * @param {number} [opts.mipCount]
 * @param {number} [opts.flags]
 * @param {number} [opts.caps2]
 * @param {number} [opts.pfFlags]
 * @param {number} [opts.headerSize]
 * @param {number} [opts.extra] - extra trailing bytes after the mip chain
 * @returns {ArrayBuffer}
 */
function makeDDS(opts) {
	const fourCC = opts.fourCC ?? "DXT1";
	const mipCount = opts.mipCount ?? 1;
	const flags = opts.flags ?? DDSD_MIPMAPCOUNT;
	const caps2 = opts.caps2 ?? 0;
	const pfFlags = opts.pfFlags ?? DDPF_FOURCC;
	const headerSize = opts.headerSize ?? 124;

	const blockBytes = { DXT1: 8, DXT5: 16, ATI2: 16 }[fourCC] ?? 8;
	let w = opts.width;
	let h = opts.height;
	let total = 0;
	const effectiveMipCount = (flags & DDSD_MIPMAPCOUNT) === 0 || mipCount === 0 ? 1 : mipCount;
	for (let i = 0; i < effectiveMipCount; i++) {
		const bw = Math.max(1, Math.ceil(w / 4));
		const bh = Math.max(1, Math.ceil(h / 4));
		total += bw * bh * blockBytes;
		w = Math.max(1, Math.floor(w / 2));
		h = Math.max(1, Math.floor(h / 2));
	}
	total += opts.extra ?? 0;

	const buffer = new ArrayBuffer(HEADER_SIZE + total);
	const view = new DataView(buffer);
	view.setUint8(0, "D".charCodeAt(0));
	view.setUint8(1, "D".charCodeAt(0));
	view.setUint8(2, "S".charCodeAt(0));
	view.setUint8(3, " ".charCodeAt(0));
	view.setUint32(4, headerSize, true);
	view.setUint32(8, flags, true);
	view.setUint32(12, opts.height, true);
	view.setUint32(16, opts.width, true);
	view.setUint32(28, mipCount, true);
	view.setUint32(80, pfFlags, true);
	for (let i = 0; i < 4; i++) {
		view.setUint8(84 + i, fourCC.charCodeAt(i));
	}
	view.setUint32(112, caps2, true);
	return buffer;
}

test("parseDDS: 256x128 DXT5 mip offsets/sizes", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT5", width: 256, height: 128, mipCount: 1 }));
	assert.equal(dds.format, "DXT5");
	assert.equal(dds.width, 256);
	assert.equal(dds.height, 128);
	assert.equal(dds.mips.length, 1);
	assert.deepEqual(dds.mips[0], { offset: 128, size: 64 * 32 * 16, width: 256, height: 128 });
});

test("parseDDS: 8x8 DXT1 full mip chain", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 8, height: 8, mipCount: 4 }));
	assert.equal(dds.mips.length, 4);
	assert.deepEqual(
		dds.mips.map((m) => [m.width, m.height, m.size]),
		[
			[8, 8, 2 * 2 * 8],
			[4, 4, 1 * 1 * 8],
			[2, 2, 1 * 1 * 8],
			[1, 1, 1 * 1 * 8],
		],
	);
	let offset = 128;
	for (const m of dds.mips) {
		assert.equal(m.offset, offset);
		offset += m.size;
	}
});

test("parseDDS: 4x4 ATI2 down to 1x1", () => {
	const dds = parseDDS(makeDDS({ fourCC: "ATI2", width: 4, height: 4, mipCount: 3 }));
	assert.deepEqual(
		dds.mips.map((m) => [m.width, m.height]),
		[
			[4, 4],
			[2, 2],
			[1, 1],
		],
	);
	for (const m of dds.mips) {
		assert.equal(m.size, 16);
	}
});

test("parseDDS: non-square 1024x2048 DXT5", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT5", width: 1024, height: 2048, mipCount: 1 }));
	assert.equal(dds.width, 1024);
	assert.equal(dds.height, 2048);
	assert.equal(dds.mips[0].size, (1024 / 4) * (2048 / 4) * 16);
});

test("parseDDS: no mip flag gives 1 mip even with mipCount set", () => {
	const dds = parseDDS(makeDDS({ width: 16, height: 16, mipCount: 5, flags: 0 }));
	assert.equal(dds.mips.length, 1);
});

test("parseDDS: mip flag set but count 0 gives 1 mip", () => {
	const dds = parseDDS(makeDDS({ width: 16, height: 16, mipCount: 0, flags: DDSD_MIPMAPCOUNT }));
	assert.equal(dds.mips.length, 1);
});

test("parseDDS: 6x6 block size (partial block)", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 6, height: 6, mipCount: 1 }));
	assert.equal(dds.mips[0].size, 2 * 2 * 8);
});

test("parseDDS: 1x1 block size", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 1, height: 1, mipCount: 1 }));
	assert.equal(dds.mips[0].size, 1 * 1 * 8);
});

test("parseDDS: 2x2 block size", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT5", width: 2, height: 2, mipCount: 1 }));
	assert.equal(dds.mips[0].size, 1 * 1 * 16);
});

test("parseDDS: trailing bytes after the last mip are ignored", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 4, height: 4, mipCount: 1, extra: 100 }));
	assert.equal(dds.mips.length, 1);
});

test("parseDDS: too short", () => {
	assert.throws(() => parseDDS(new ArrayBuffer(10)), /dds: too short or missing "DDS " magic/);
});

test("parseDDS: bad magic", () => {
	const buffer = makeDDS({ width: 4, height: 4 });
	new DataView(buffer).setUint8(0, "X".charCodeAt(0));
	assert.throws(() => parseDDS(buffer), /dds: too short or missing "DDS " magic/);
});

test("parseDDS: bad header size", () => {
	assert.throws(() => parseDDS(makeDDS({ width: 4, height: 4, headerSize: 100 })), /dds: header size 100, want 124/);
});

test("parseDDS: zero dimension", () => {
	assert.throws(() => parseDDS(makeDDS({ width: 0, height: 4 })), /dds: zero dimension 0x4/);
});

test("parseDDS: caps2 set (cube map / volume)", () => {
	assert.throws(() => parseDDS(makeDDS({ width: 4, height: 4, caps2: 0x200 })), /dds: caps2 0x200/);
});

test("parseDDS: no FourCC bit", () => {
	assert.throws(() => parseDDS(makeDDS({ width: 4, height: 4, pfFlags: 0 })), /dds: pixel format flags 0x0, want the FourCC bit set/);
});

test("parseDDS: unsupported FourCC (DX10)", () => {
	assert.throws(() => parseDDS(makeDDS({ width: 4, height: 4, fourCC: "DX10" })), /dds: unsupported FourCC "DX10"/);
});

test("parseDDS: mip past end of data", () => {
	const buffer = makeDDS({ fourCC: "DXT1", width: 8, height: 8, mipCount: 1 });
	const truncated = buffer.slice(0, buffer.byteLength - 1);
	assert.throws(() => parseDDS(truncated), /dds: mip 0 .* runs past end of data/);
});

test("blockUploadable: 12x12 fails at mip 1 (6x6)", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 12, height: 12, mipCount: 2 }));
	assert.deepEqual(blockUploadable(dds), { level: 1, width: 6, height: 6 });
});

test("blockUploadable: 1536x1536 fails at mip 8 (6x6)", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT5", width: 1536, height: 1536, mipCount: 9 }));
	assert.deepEqual(blockUploadable(dds), { level: 8, width: 6, height: 6 });
});

test("blockUploadable: 6x4 fails at level 0", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 6, height: 4, mipCount: 1 }));
	assert.deepEqual(blockUploadable(dds), { level: 0, width: 6, height: 4 });
});

test("blockUploadable: 4x4 ok", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 4, height: 4, mipCount: 1 }));
	assert.equal(blockUploadable(dds), null);
});

test("blockUploadable: 8x8 ok", () => {
	const dds = parseDDS(makeDDS({ fourCC: "DXT1", width: 8, height: 8, mipCount: 4 }));
	assert.equal(blockUploadable(dds), null);
});
