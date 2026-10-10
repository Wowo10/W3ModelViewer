// Decides how to upload a parsed DDS (compressed through the S3TC/RGTC
// extensions, or decoded to RGBA8 on the CPU) and performs the GL upload,
// mirroring viewer/texture.go's uploadTexture but with a CPU fallback the
// desktop doesn't need.

import { parseDDS, blockUploadable } from "./dds.js";
import { decodeMip } from "./bcdecode.js";

/** @typedef {import("./dds.js").DDS} DDS */
/** @typedef {import("./dds.js").Format} Format */

/**
 * @typedef {Object} CompressionExts
 * @property {boolean} s3tc - WEBGL_compressed_texture_s3tc available
 * @property {boolean} rgtc - EXT_texture_compression_rgtc available
 */

/**
 * @typedef {Object} UploadPath
 * @property {boolean} compressed
 * @property {string} reason
 */

/**
 * uploadPath decides whether dds can be uploaded compressed given the
 * available extensions and its mip sizes, or must be decoded to RGBA8 on
 * the CPU, and why.
 * @param {DDS} dds
 * @param {CompressionExts} exts
 * @returns {UploadPath}
 */
export function uploadPath(dds, exts) {
	if (dds.format === "ATI2") {
		if (!exts.rgtc) {
			return { compressed: false, reason: "CPU: no EXT_texture_compression_rgtc" };
		}
	} else if (!exts.s3tc) {
		return { compressed: false, reason: "CPU: no WEBGL_compressed_texture_s3tc" };
	}

	const bad = blockUploadable(dds);
	if (bad !== null) {
		return { compressed: false, reason: `CPU: mip ${bad.level} ${bad.width}x${bad.height} not a multiple of 4` };
	}

	return { compressed: true, reason: dds.format === "ATI2" ? "RGTC" : "S3TC" };
}

/** @type {Record<Format, number>} */
const S3TC_RGTC_ENUM = {
	// WEBGL_compressed_texture_s3tc / EXT_texture_compression_rgtc enum
	// values (fixed by the spec, not queried from the extension object).
	DXT1: 0x83f1, // COMPRESSED_RGBA_S3TC_DXT1_EXT
	DXT5: 0x83f3, // COMPRESSED_RGBA_S3TC_DXT5_EXT
	ATI2: 0x8dbd, // COMPRESSED_RED_GREEN_RGTC2_EXT
};

/**
 * @typedef {Object} UploadedTexture
 * @property {WebGLTexture} texture
 * @property {Format} format
 * @property {number} width
 * @property {number} height
 * @property {number} mips
 * @property {string} path - the reason string from uploadPath (or the
 *   compressed-upload-rejected fallback)
 */

/**
 * uploadCompressed uploads every mip of dds through compressedTexImage2D.
 * @param {WebGL2RenderingContext} gl
 * @param {WebGLTexture} texture
 * @param {DDS} dds
 * @param {Uint8Array} bytes
 */
function uploadCompressed(gl, texture, dds, bytes) {
	gl.bindTexture(gl.TEXTURE_2D, texture);
	const internalFormat = S3TC_RGTC_ENUM[dds.format];
	for (let level = 0; level < dds.mips.length; level++) {
		const m = dds.mips[level];
		gl.compressedTexImage2D(gl.TEXTURE_2D, level, internalFormat, m.width, m.height, 0, bytes.subarray(m.offset, m.offset + m.size));
	}
}

/**
 * uploadCPU decodes every mip of dds to RGBA8 and uploads with texImage2D.
 * @param {WebGL2RenderingContext} gl
 * @param {WebGLTexture} texture
 * @param {DDS} dds
 * @param {Uint8Array} bytes
 */
function uploadCPU(gl, texture, dds, bytes) {
	gl.bindTexture(gl.TEXTURE_2D, texture);
	for (let level = 0; level < dds.mips.length; level++) {
		const m = dds.mips[level];
		const rgba = decodeMip(dds.format, bytes.subarray(m.offset, m.offset + m.size), m.width, m.height);
		gl.texImage2D(gl.TEXTURE_2D, level, gl.RGBA8, m.width, m.height, 0, gl.RGBA, gl.UNSIGNED_BYTE, rgba);
	}
}

/**
 * uploadTexture parses buffer as a DDS and uploads it: compressed when
 * uploadPath says so, else CPU-decoded to RGBA8. If a compressed upload is
 * rejected by the GL (gl.getError() after the mip loop), falls back to the
 * CPU path with reason "CPU: compressed upload rejected" (browsers can be
 * stricter, or laxer, than the block-size rule in dds.js). Every mip, no
 * filtering past TEXTURE_MAX_LEVEL, trilinear, non-sRGB, no V flip (as
 * stored). GL must be current; ?cpu=1 in cpu forces the CPU path (used to
 * exercise it without a browser lacking the extensions).
 * @param {WebGL2RenderingContext} gl
 * @param {CompressionExts} exts
 * @param {ArrayBuffer} buffer
 * @param {boolean} [forceCPU]
 * @returns {UploadedTexture}
 */
export function uploadTexture(gl, exts, buffer, forceCPU = false) {
	const dds = parseDDS(buffer);
	const bytes = new Uint8Array(buffer);
	let { compressed, reason } = uploadPath(dds, forceCPU ? { s3tc: false, rgtc: false } : exts);

	const texture = gl.createTexture();
	if (!texture) {
		throw new Error("texture: createTexture failed");
	}

	if (compressed) {
		gl.getError(); // clear any pending error so the check below is ours
		uploadCompressed(gl, texture, dds, bytes);
		if (gl.getError() !== gl.NO_ERROR) {
			compressed = false;
			reason = "CPU: compressed upload rejected";
			uploadCPU(gl, texture, dds, bytes);
		}
	} else {
		uploadCPU(gl, texture, dds, bytes);
	}

	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_BASE_LEVEL, 0);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAX_LEVEL, dds.mips.length - 1);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, dds.mips.length > 1 ? gl.LINEAR_MIPMAP_LINEAR : gl.LINEAR);
	gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
	gl.bindTexture(gl.TEXTURE_2D, null);

	return { texture, format: dds.format, width: dds.width, height: dds.height, mips: dds.mips.length, path: reason };
}
