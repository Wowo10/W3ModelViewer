// Column-major 4x4 matrix helpers, matching GL's uniform layout. Ported
// 1:1 from viewer/camera.go.

/** @typedef {[number, number, number]} Vec3 */
/** @typedef {Float32Array} Mat4 */

/** @returns {Mat4} the identity matrix. */
export function identity() {
	const m = new Float32Array(16);
	m[0] = m[5] = m[10] = m[15] = 1;
	return m;
}

/**
 * perspective builds a right-handed perspective projection (fovY in
 * radians).
 * @param {number} fovY
 * @param {number} aspect
 * @param {number} near
 * @param {number} far
 * @returns {Mat4}
 */
export function perspective(fovY, aspect, near, far) {
	const f = 1 / Math.tan(fovY / 2);
	const m = new Float32Array(16);
	m[0] = f / aspect;
	m[5] = f;
	m[10] = (far + near) / (near - far);
	m[11] = -1;
	m[14] = (2 * far * near) / (near - far);
	return m;
}

/**
 * lookAt builds a right-handed view matrix for a camera at eye looking at
 * target, with up as the world up vector.
 * @param {Vec3} eye
 * @param {Vec3} target
 * @param {Vec3} up
 * @returns {Mat4}
 */
export function lookAt(eye, target, up) {
	const f = normalize(sub(target, eye));
	const s = normalize(cross(f, up));
	const u = cross(s, f);

	const m = new Float32Array(16);
	m[0] = s[0]; m[4] = s[1]; m[8] = s[2];
	m[1] = u[0]; m[5] = u[1]; m[9] = u[2];
	m[2] = -f[0]; m[6] = -f[1]; m[10] = -f[2];
	m[12] = -dot(s, eye);
	m[13] = -dot(u, eye);
	m[14] = dot(f, eye);
	m[15] = 1;
	return m;
}

/**
 * mul returns a*b (a applied after b, as in GL conventions).
 * @param {Mat4} a
 * @param {Mat4} b
 * @returns {Mat4}
 */
export function mul(a, b) {
	const r = new Float32Array(16);
	for (let col = 0; col < 4; col++) {
		for (let row = 0; row < 4; row++) {
			let sum = 0;
			for (let k = 0; k < 4; k++) {
				sum += a[k * 4 + row] * b[col * 4 + k];
			}
			r[col * 4 + row] = sum;
		}
	}
	return r;
}

/**
 * @param {Vec3} a
 * @param {Vec3} b
 * @returns {Vec3}
 */
export function sub(a, b) {
	return [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
}

/**
 * @param {Vec3} a
 * @param {Vec3} b
 * @returns {Vec3}
 */
export function cross(a, b) {
	return [
		a[1] * b[2] - a[2] * b[1],
		a[2] * b[0] - a[0] * b[2],
		a[0] * b[1] - a[1] * b[0],
	];
}

/**
 * @param {Vec3} a
 * @param {Vec3} b
 * @returns {number}
 */
export function dot(a, b) {
	return a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
}

/**
 * @param {Vec3} v
 * @returns {Vec3}
 */
export function normalize(v) {
	const l = Math.sqrt(dot(v, v));
	if (l === 0) {
		return v;
	}
	return [v[0] / l, v[1] / l, v[2] / l];
}
