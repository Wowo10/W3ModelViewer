import test from "node:test";
import assert from "node:assert/strict";
import { decodePose, matrixTextureSize, PoseScheduler } from "../static/js/pose.js";

/** @typedef {import("../static/js/api.js").Model} Model */
/** @typedef {import("../static/js/pose.js").PoseRequest} PoseRequest */

/**
 * buildPose lays out a pose buffer the way web/pose.go's handlePose does:
 * uint32 objects, uint32 geosets, objects x 16 float32, geosets x float32.
 * @param {number} objects
 * @param {number} geosets
 * @returns {ArrayBuffer}
 */
function buildPose(objects, geosets) {
	const buffer = new ArrayBuffer(8 + 16 * 4 * objects + 4 * geosets);
	const view = new DataView(buffer);
	view.setUint32(0, objects, true);
	view.setUint32(4, geosets, true);
	const matrices = new Float32Array(buffer, 8, 16 * objects);
	for (let i = 0; i < matrices.length; i++) matrices[i] = i;
	const alphas = new Float32Array(buffer, 8 + 16 * 4 * objects, geosets);
	for (let i = 0; i < alphas.length; i++) alphas[i] = Math.fround(i / 10);
	return buffer;
}

test("decodePose: matrices and alphas views match the input", () => {
	const buffer = buildPose(2, 3);
	/** @type {Model} */
	const model = /** @type {any} */ ({ objects: 2, geosets: [{}, {}, {}] });
	const pose = decodePose(buffer, model);
	assert.equal(pose.matrices.length, 32);
	assert.deepEqual(Array.from(pose.matrices.slice(0, 4)), [0, 1, 2, 3]);
	assert.equal(pose.alphas.length, 3);
	assert.deepEqual(Array.from(pose.alphas), Array.from(new Float32Array([0, 0.1, 0.2])));
});

test("decodePose: objects mismatch throws", () => {
	const buffer = buildPose(2, 3);
	/** @type {Model} */
	const model = /** @type {any} */ ({ objects: 5, geosets: [{}, {}, {}] });
	assert.throws(() => decodePose(buffer, model), /objects/);
});

test("decodePose: geosets mismatch throws", () => {
	const buffer = buildPose(2, 3);
	/** @type {Model} */
	const model = /** @type {any} */ ({ objects: 2, geosets: [{}] });
	assert.throws(() => decodePose(buffer, model), /geosets/);
});

test("decodePose: short buffer throws", () => {
	const full = buildPose(2, 3);
	const short = full.slice(0, full.byteLength - 1);
	/** @type {Model} */
	const model = /** @type {any} */ ({ objects: 2, geosets: [{}, {}, {}] });
	assert.throws(() => decodePose(short, model), /too short/);

	const tooShortForHeader = new ArrayBuffer(4);
	assert.throws(() => decodePose(tooShortForHeader, model), /too short/);
});

test("matrixTextureSize: fixed width 1024, height from object count", () => {
	assert.deepEqual(matrixTextureSize(0), { width: 1024, height: 1 });
	assert.deepEqual(matrixTextureSize(1), { width: 1024, height: 1 });
	assert.deepEqual(matrixTextureSize(256), { width: 1024, height: 1 });
	assert.deepEqual(matrixTextureSize(257), { width: 1024, height: 2 });
	assert.deepEqual(matrixTextureSize(2940), { width: 1024, height: 12 });
});

/**
 * deferred returns a promise plus its resolve/reject, for fake fetchPose
 * implementations driven by hand.
 * @returns {{promise: Promise<ArrayBuffer>, resolve: (v: ArrayBuffer) => void, reject: (e: unknown) => void}}
 */
function deferred() {
	/** @type {(v: ArrayBuffer) => void} */
	let resolve = () => {};
	/** @type {(e: unknown) => void} */
	let reject = () => {};
	const promise = new Promise((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
}

/** @type {Model} */
const fakeModel = /** @type {any} */ ({});

/** @param {number} token @param {number} frame @returns {PoseRequest} */
function req(token, frame) {
	return /** @type {PoseRequest} */ ({ token, model: fakeModel, seq: 0, frame, global: frame });
}

test("PoseScheduler: many wants while one is in flight send exactly one more, the latest", async () => {
	/** @type {{promise: Promise<ArrayBuffer>, resolve: (v: ArrayBuffer) => void}[]} */
	const calls = [];
	/** @type {PoseRequest[]} */
	const sent = [];
	let clock = 0;

	const scheduler = new PoseScheduler(
		(r) => {
			sent.push(r);
			const d = deferred();
			calls.push(d);
			return d.promise;
		},
		() => {},
		() => {},
		() => clock,
	);

	scheduler.want(req(1, 1));
	scheduler.want(req(1, 2));
	scheduler.want(req(1, 3));
	assert.equal(sent.length, 1); // only the first was sent, since one was already in flight
	assert.equal(sent[0].frame, 1);

	calls[0].resolve(new ArrayBuffer(0));
	await Promise.resolve();
	await Promise.resolve();

	assert.equal(sent.length, 2);
	assert.equal(sent[1].frame, 3); // the latest want, not frame 2
});

test("PoseScheduler: identical wants don't refetch", async () => {
	/** @type {PoseRequest[]} */
	const sent = [];
	const d = deferred();
	const scheduler = new PoseScheduler(
		(r) => {
			sent.push(r);
			return d.promise;
		},
		() => {},
		() => {},
		() => 0,
	);

	scheduler.want(req(1, 5));
	scheduler.want(req(1, 5));
	scheduler.want(req(1, 5));
	assert.equal(sent.length, 1);

	d.resolve(new ArrayBuffer(0));
	await Promise.resolve();
	await Promise.resolve();

	scheduler.want(req(1, 5)); // still the same request as the one already sent
	assert.equal(sent.length, 1);
});

test("PoseScheduler: an error clears in-flight and calls onError, then resends if wanted changed", async () => {
	/** @type {PoseRequest[]} */
	const sent = [];
	/** @type {[unknown, PoseRequest, number][]} */
	const errors = [];
	const d1 = deferred();
	let d2 = deferred();

	const scheduler = new PoseScheduler(
		(r) => {
			sent.push(r);
			return sent.length === 1 ? d1.promise : d2.promise;
		},
		() => {},
		(err, r, latency) => errors.push([err, r, latency]),
		() => 0,
	);

	scheduler.want(req(1, 1));
	scheduler.want(req(1, 2)); // queued behind the first
	d1.reject(new Error("boom"));
	await Promise.resolve();
	await Promise.resolve();

	assert.equal(errors.length, 1);
	assert.equal(errors[0][1].frame, 1);
	assert.equal(sent.length, 2);
	assert.equal(sent[1].frame, 2); // the queued want was sent after the error cleared in-flight
});

test("PoseScheduler: latency comes from the injected clock", async () => {
	/** @type {[ArrayBuffer, PoseRequest, number][]} */
	const poses = [];
	const d = deferred();
	let clock = 100;

	const scheduler = new PoseScheduler(
		() => d.promise,
		(buffer, r, latency) => poses.push([buffer, r, latency]),
		() => {},
		() => clock,
	);

	scheduler.want(req(1, 1));
	clock = 142;
	d.resolve(new ArrayBuffer(0));
	await Promise.resolve();
	await Promise.resolve();

	assert.equal(poses.length, 1);
	assert.equal(poses[0][2], 42);
});
