// Decodes GET /api/pose's binary response, lays out the node matrix
// texture's dimensions, and schedules pose requests so at most one is ever
// in flight (a slow tab never queues several).

/** @typedef {import("./api.js").Model} Model */

/**
 * @typedef {Object} DecodedPose
 * @property {Float32Array} matrices - objects*16, column-major, by object id
 * @property {Float32Array} alphas - one per geoset
 */

/**
 * decodePose parses the buffer served by GET /api/pose: uint32 object
 * count, uint32 geoset count, object-count x 16 float32 matrices, then
 * geoset-count x float32 alphas. Throws "pose: ..." when the header counts
 * don't match model.objects/model.geosets.length, or the buffer is too
 * short for them.
 * @param {ArrayBuffer} buffer
 * @param {Model} model
 * @returns {DecodedPose}
 */
export function decodePose(buffer, model) {
	if (buffer.byteLength < 8) {
		throw new Error(`pose: buffer of ${buffer.byteLength} bytes too short for the header`);
	}

	const header = new DataView(buffer);
	const objects = header.getUint32(0, true);
	const geosets = header.getUint32(4, true);
	if (objects !== model.objects) {
		throw new Error(`pose: objects ${objects} != model.objects ${model.objects}`);
	}
	if (geosets !== model.geosets.length) {
		throw new Error(`pose: geosets ${geosets} != model.geosets.length ${model.geosets.length}`);
	}

	const matricesBytes = 16 * 4 * objects;
	const alphasBytes = 4 * geosets;
	const want = 8 + matricesBytes + alphasBytes;
	if (buffer.byteLength < want) {
		throw new Error(`pose: buffer of ${buffer.byteLength} bytes too short, want ${want}`);
	}

	const matrices = new Float32Array(buffer, 8, 16 * objects);
	const alphas = new Float32Array(buffer, 8 + matricesBytes, geosets);
	return { matrices, alphas };
}

/**
 * MATRIX_TEXTURE_WIDTH is fixed at 1,024 texels (256 matrices per row, a
 * multiple of 4 so a matrix never straddles rows): 2,940 objects (the worst
 * model measured) times 4 exceeds WebGL2's guaranteed 2,048 MAX_TEXTURE_SIZE
 * in one row, so the matrix texture is laid out 2D instead.
 */
export const MATRIX_TEXTURE_WIDTH = 1024;

/**
 * matrixTextureSize returns the RGBA32F node matrix texture's dimensions
 * for objects object ids: a fixed width of 1,024 texels (256 matrices per
 * row) and enough rows to fit every matrix (at least 1, so a model with no
 * objects still gets an allocatable texture).
 * @param {number} objects
 * @returns {{width: number, height: number}}
 */
export function matrixTextureSize(objects) {
	return { width: MATRIX_TEXTURE_WIDTH, height: Math.max(1, Math.ceil(objects / 256)) };
}

/**
 * @typedef {Object} PoseRequest
 * @property {number} token - identifies which model/edition this pose is for
 * @property {Model} model
 * @property {number} seq
 * @property {number} frame
 * @property {number} global
 */

/**
 * sameRequest reports whether a and b ask for the same pose.
 * @param {PoseRequest | null} a
 * @param {PoseRequest | null} b
 * @returns {boolean}
 */
function sameRequest(a, b) {
	return a !== null && b !== null && a.token === b.token && a.model === b.model && a.seq === b.seq && a.frame === b.frame && a.global === b.global;
}

/**
 * PoseScheduler keeps at most one fetchPose call in flight: want() records
 * the latest desired request, sending it immediately when nothing is
 * pending and it differs from the last request sent; each settlement
 * (success or error) reports latency via onPose/onError and, if the wanted
 * request has since changed, sends the newest one.
 */
export class PoseScheduler {
	/**
	 * @param {(req: PoseRequest) => Promise<ArrayBuffer>} fetchPose
	 * @param {(buffer: ArrayBuffer, req: PoseRequest, latencyMs: number) => void} onPose
	 * @param {(err: unknown, req: PoseRequest, latencyMs: number) => void} onError
	 * @param {() => number} now
	 */
	constructor(fetchPose, onPose, onError, now) {
		this.fetchPose = fetchPose;
		this.onPose = onPose;
		this.onError = onError;
		this.now = now;

		/** @type {PoseRequest | null} */
		this.wanted = null;
		/** @type {PoseRequest | null} */
		this.lastSent = null;
		this.pending = false;
	}

	/** @param {PoseRequest} req */
	want(req) {
		this.wanted = req;
		this.send();
	}

	send() {
		if (this.pending || this.wanted === null || sameRequest(this.wanted, this.lastSent)) {
			return;
		}

		const req = this.wanted;
		this.lastSent = req;
		this.pending = true;
		const start = this.now();

		this.fetchPose(req).then(
			(buffer) => {
				this.pending = false;
				this.onPose(buffer, req, this.now() - start);
				this.send();
			},
			(err) => {
				this.pending = false;
				this.onError(err, req, this.now() - start);
				this.send();
			},
		);
	}
}
