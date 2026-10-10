import test from "node:test";
import assert from "node:assert/strict";
import { OrbitCamera, FOV_Y, wheelSteps } from "../static/js/camera.js";

/**
 * @param {ArrayLike<number>} a
 * @param {ArrayLike<number>} b
 * @param {number} [eps]
 */
function approx(a, b, eps = 1e-4) {
	assert.equal(a.length, b.length);
	for (let i = 0; i < a.length; i++) {
		assert.ok(Math.abs(a[i] - b[i]) < eps, `index ${i}: ${a[i]} !~ ${b[i]}`);
	}
}

test("frame: box target/radius/distance, pitch 20deg", () => {
	const c = new OrbitCamera();
	c.frame([-1, -2, -3], [5, 6, 7]);

	approx(c.target, [2, 2, 2]);
	approx([c.radius], [Math.sqrt((5 - 2) ** 2 + (6 - 2) ** 2 + (7 - 2) ** 2)]);
	assert.equal(c.yaw, 0);
	approx([c.pitch], [(20 * Math.PI) / 180]);
	approx([c.distance], [(c.radius / Math.sin(FOV_Y / 2)) * 1.1]);
});

test("frame: zero-size bounds give radius 1", () => {
	const c = new OrbitCamera();
	c.frame([1, 1, 1], [1, 1, 1]);
	assert.equal(c.radius, 1);
});

test("rotate: rate and +-89deg clamp", () => {
	const c = new OrbitCamera();
	c.rotate(1, 0);
	approx([c.yaw], [(0.4 * Math.PI) / 180]);

	const maxPitch = (89 * Math.PI) / 180;
	c.pitch = 88 * (Math.PI / 180);
	c.rotate(0, 1000);
	assert.equal(c.pitch, maxPitch);

	c.pitch = -88 * (Math.PI / 180);
	c.rotate(0, -1000);
	assert.equal(c.pitch, -maxPitch);
});

test("zoom: factor and both clamps", () => {
	const c = new OrbitCamera();
	c.radius = 1;
	c.distance = 10;
	c.zoom(1);
	approx([c.distance], [10 * 0.9]);

	const c2 = new OrbitCamera();
	c2.radius = 2;
	c2.distance = 1;
	c2.zoom(-1000); // steps < 0 = zoom out = distance grows, clamped to max
	assert.equal(c2.distance, 50 * c2.radius);

	const c3 = new OrbitCamera();
	c3.radius = 2;
	c3.distance = 100;
	c3.zoom(1000); // steps > 0 = zoom in = distance shrinks, clamped to min
	assert.equal(c3.distance, 0.05 * c3.radius);
});

test("matrices: target maps to (0,0,-distance) in view space, near/far", () => {
	const c = new OrbitCamera();
	c.frame([0, 0, 0], [2, 2, 2]);
	const { view, proj } = c.matrices(1.5);

	// Apply view to target (w=1): should land at (0, 0, -distance).
	const x = [c.target[0], c.target[1], c.target[2], 1];
	const r = [0, 0, 0, 0];
	for (let row = 0; row < 4; row++) {
		let sum = 0;
		for (let k = 0; k < 4; k++) {
			sum += view[k * 4 + row] * x[k];
		}
		r[row] = sum;
	}
	approx(r.slice(0, 3), [0, 0, -c.distance]);

	assert.equal(proj[11], -1);
	const near = c.distance / 100;
	const far = c.distance + 2 * c.radius;
	// proj[10] = (far+near)/(near-far), proj[14] = 2*far*near/(near-far)
	approx([proj[10]], [(far + near) / (near - far)]);
	approx([proj[14]], [(2 * far * near) / (near - far)]);
});

test("cross-check against viewer/camera.go for fixed inputs", () => {
	const c = new OrbitCamera();
	c.frame([-1, -2, -3], [5, 6, 7]);
	approx(c.target, [2, 2, 2]);
	approx([c.yaw], [0]);
	approx([c.pitch], [0.34906584]);
	approx([c.distance], [20.3253498]);
	approx([c.radius], [7.07106781]);

	c.rotate(30, -15);
	approx([c.yaw], [0.209439516]);
	approx([c.pitch], [0.244346082]);

	c.zoom(5);
	approx([c.distance], [12.0019159]);

	const { view, proj } = c.matrices(16 / 9);
	approx(
		view,
		[
			-0.20791169, -0.23663531, 0.9490924, 0, 0.9781476, -0.050298385, 0.20173581, 0, 0, 0.9702957, 0.24192189, 0,
			-1.540472, -1.366724, -14.787417, 1,
		],
	);
	approx(
		proj,
		[1.357995, 0, 0, 0, 0, 2.4142134, 0, 0, 0, 0, -1.0092238, -1, 0, 0, -0.24114536, 0],
	);
});

test("wheelSteps: signs and modes", () => {
	assert.equal(wheelSteps(-100, 0), 1); // away from the user = zoom in
	assert.equal(wheelSteps(100, 0), -1);
	assert.equal(wheelSteps(-3, 1), 1);
	assert.equal(wheelSteps(3, 1), -1);
});
