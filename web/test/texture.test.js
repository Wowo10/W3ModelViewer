import test from "node:test";
import assert from "node:assert/strict";
import { uploadPath } from "../static/js/texture.js";

/** @typedef {import("../static/js/dds.js").DDS} DDS */

/**
 * dds builds a minimal DDS value (uploadPath only reads format and mips).
 * @param {import("../static/js/dds.js").Format} format
 * @param {{width: number, height: number}[]} mips
 * @returns {DDS}
 */
function dds(format, mips) {
	return { format, width: mips[0].width, height: mips[0].height, mips: mips.map((m) => ({ offset: 0, size: 0, ...m })) };
}

const BOTH = { s3tc: true, rgtc: true };

test("uploadPath: DXT1 with S3TC and aligned mips -> compressed, S3TC", () => {
	assert.deepEqual(uploadPath(dds("DXT1", [{ width: 8, height: 8 }]), BOTH), { compressed: true, reason: "S3TC" });
});

test("uploadPath: DXT5 with S3TC and aligned mips -> compressed, S3TC", () => {
	assert.deepEqual(uploadPath(dds("DXT5", [{ width: 256, height: 128 }]), BOTH), { compressed: true, reason: "S3TC" });
});

test("uploadPath: ATI2 with RGTC and aligned mips -> compressed, RGTC", () => {
	assert.deepEqual(uploadPath(dds("ATI2", [{ width: 16, height: 16 }]), BOTH), { compressed: true, reason: "RGTC" });
});

test("uploadPath: DXT1 without S3TC -> CPU", () => {
	assert.deepEqual(uploadPath(dds("DXT1", [{ width: 8, height: 8 }]), { s3tc: false, rgtc: true }), {
		compressed: false,
		reason: "CPU: no WEBGL_compressed_texture_s3tc",
	});
});

test("uploadPath: ATI2 without RGTC -> CPU", () => {
	assert.deepEqual(uploadPath(dds("ATI2", [{ width: 8, height: 8 }]), { s3tc: true, rgtc: false }), {
		compressed: false,
		reason: "CPU: no EXT_texture_compression_rgtc",
	});
});

test("uploadPath: non-multiple-of-4 mip -> CPU, names the offending mip", () => {
	assert.deepEqual(uploadPath(dds("DXT1", [{ width: 12, height: 12 }, { width: 6, height: 6 }]), BOTH), {
		compressed: false,
		reason: "CPU: mip 1 6x6 not a multiple of 4",
	});
});

test("uploadPath: level 0 not a multiple of 4 -> CPU", () => {
	assert.deepEqual(uploadPath(dds("DXT1", [{ width: 6, height: 4 }]), BOTH), {
		compressed: false,
		reason: "CPU: mip 0 6x4 not a multiple of 4",
	});
});

test("uploadPath: missing extension takes priority over a bad mip size", () => {
	assert.deepEqual(uploadPath(dds("DXT1", [{ width: 6, height: 4 }]), { s3tc: false, rgtc: true }), {
		compressed: false,
		reason: "CPU: no WEBGL_compressed_texture_s3tc",
	});
});
