import test from "node:test";
import assert from "node:assert/strict";
import { identity, perspective, lookAt, mul, sub, cross, dot, normalize } from "../static/js/mat4.js";

/**
 * @param {ArrayLike<number>} a
 * @param {ArrayLike<number>} b
 * @param {number} [eps]
 */
function approx(a, b, eps = 1e-5) {
	assert.equal(a.length, b.length);
	for (let i = 0; i < a.length; i++) {
		assert.ok(Math.abs(a[i] - b[i]) < eps, `index ${i}: ${a[i]} !~ ${b[i]}`);
	}
}

/** @param {import("../static/js/mat4.js").Mat4} m @param {[number,number,number]} v @param {number} w */
function apply(m, v, w) {
	const x = [v[0], v[1], v[2], w];
	const r = [0, 0, 0, 0];
	for (let row = 0; row < 4; row++) {
		let sum = 0;
		for (let k = 0; k < 4; k++) {
			sum += m[k * 4 + row] * x[k];
		}
		r[row] = sum;
	}
	return r;
}

test("mul: identity on both sides", () => {
	const m = perspective(1, 1.5, 0.1, 10);
	approx(mul(identity(), m), m);
	approx(mul(m, identity()), m);
});

test("mul: two translations compose", () => {
	const t1 = identity();
	t1[12] = 1; t1[13] = 2; t1[14] = 3;
	const t2 = identity();
	t2[12] = 4; t2[13] = 5; t2[14] = 6;
	const t3 = mul(t1, t2);
	approx(t3.slice(12, 15), [5, 7, 9]);
});

test("mul: rotation and translation don't commute (column-major layout)", () => {
	// 90deg rotation around Z: row0 = [0,-1,0,0], row1 = [1,0,0,0].
	const r = identity();
	r[0] = 0; r[1] = 1; r[4] = -1; r[5] = 0;
	const t = identity();
	t[12] = 1; t[13] = 0; t[14] = 0;

	approx(mul(r, t).slice(12, 15), [0, 1, 0]);
	approx(mul(t, r).slice(12, 15), [1, 0, 0]);
});

test("lookAt: eye to origin, target to -distance, up to +y (Z-up camera)", () => {
	/** @type {[number, number, number]} */
	const eye = [0, 5, 0];
	/** @type {[number, number, number]} */
	const target = [0, 0, 0];
	const m = lookAt(eye, target, [0, 0, 1]);

	approx(apply(m, eye, 1).slice(0, 3), [0, 0, 0]);
	approx(apply(m, target, 1).slice(0, 3), [0, 0, -5]);
	approx(apply(m, [0, 0, 1], 0).slice(0, 3), [0, 1, 0]);
});

test("perspective: m[11] is -1, near/far map to NDC z -1/+1, x scales by 1/aspect", () => {
	const near = 1, far = 10, aspect = 2;
	const m = perspective(Math.PI / 2, aspect, near, far);
	assert.equal(m[11], -1);

	const atNear = apply(m, [1, 0, -near], 1);
	assert.ok(Math.abs(atNear[2] / atNear[3] - -1) < 1e-5);
	assert.ok(Math.abs(atNear[0] / atNear[3] - 1 / aspect) < 1e-5);

	const atFar = apply(m, [0, 0, -far], 1);
	assert.ok(Math.abs(atFar[2] / atFar[3] - 1) < 1e-5);
});

test("cross-check against viewer/camera.go for fixed inputs", () => {
	const p = perspective((45 * Math.PI) / 180, 16 / 9, 0.1, 100);
	approx(p, [
		1.35799503, 0, 0, 0,
		0, 2.41421342, 0, 0,
		0, 0, -1.002002, -1,
		0, 0, -0.2002002, 0,
	]);

	const la = lookAt([3, 4, 5], [0, 0, 0], [0, 0, 1]);
	approx(la, [
		-0.800000072, -0.424264073, 0.424264073, 0,
		0.600000024, -0.565685451, 0.565685451, 0,
		0, 0.707106829, 0.707106769, 0,
		0, -2.38418579e-7, -7.07106781, 1,
	]);

	approx(mul(la, p), [
		-1.0863961, -0.57614851, 0.57614851, 0,
		1.44852805, -1.36568546, 1.36568546, 0,
		0, -0.708522201, 6.36254549, -1,
		0, -0.141562924, -0.141562924, 0,
	]);
});

test("sub/cross/dot/normalize", () => {
	approx(sub([1, 2, 3], [1, 1, 1]), [0, 1, 2]);
	approx(cross([1, 0, 0], [0, 1, 0]), [0, 0, 1]);
	assert.equal(dot([1, 2, 3], [4, 5, 6]), 32);
	approx(normalize([3, 0, 0]), [1, 0, 0]);
	approx(normalize([0, 0, 0]), [0, 0, 0]);
});
