// Z-up orbit camera, ported 1:1 from viewer/camera.go.

import { lookAt, perspective, sub, dot } from "./mat4.js";

/** @typedef {import("./mat4.js").Vec3} Vec3 */
/** @typedef {import("./mat4.js").Mat4} Mat4 */

/** FOV_Y is the camera's vertical field of view, in radians. */
export const FOV_Y = (45 * Math.PI) / 180;

/**
 * OrbitCamera orbits target at distance, controlled by yaw (around Z) and
 * pitch (around the horizontal axis).
 */
export class OrbitCamera {
	constructor() {
		/** @type {Vec3} */
		this.target = [0, 0, 0];
		this.yaw = 0;
		this.pitch = 0;
		this.distance = 0;
		this.radius = 0;
	}

	/**
	 * frame centres the camera on the bounds [min, max] and backs it off to
	 * fit the whole bounding sphere in FOV_Y.
	 * @param {Vec3} min
	 * @param {Vec3} max
	 */
	frame(min, max) {
		this.target = [(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2];
		const d = sub(max, this.target);
		this.radius = Math.sqrt(dot(d, d));
		if (this.radius === 0) {
			this.radius = 1;
		}
		this.yaw = 0;
		this.pitch = (20 * Math.PI) / 180;
		this.distance = (this.radius / Math.sin(FOV_Y / 2)) * 1.1;
	}

	/**
	 * rotate adjusts yaw/pitch by a drag of dx, dy pixels.
	 * @param {number} dx
	 * @param {number} dy
	 */
	rotate(dx, dy) {
		const degPerPixel = (0.4 * Math.PI) / 180;
		this.yaw += dx * degPerPixel;
		this.pitch += dy * degPerPixel;

		const maxPitch = (89 * Math.PI) / 180;
		if (this.pitch > maxPitch) {
			this.pitch = maxPitch;
		}
		if (this.pitch < -maxPitch) {
			this.pitch = -maxPitch;
		}
	}

	/**
	 * zoom scales the distance by 0.9^steps, clamped to [0.05, 50] x radius.
	 * @param {number} steps
	 */
	zoom(steps) {
		this.distance *= Math.pow(0.9, steps);

		const min = 0.05 * this.radius;
		const max = 50 * this.radius;
		if (this.distance < min) {
			this.distance = min;
		}
		if (this.distance > max) {
			this.distance = max;
		}
	}

	/**
	 * matrices returns the view and projection matrices for the current
	 * camera state and viewport aspect ratio.
	 * @param {number} aspect
	 * @returns {{view: Mat4, proj: Mat4}}
	 */
	matrices(aspect) {
		const cosPitch = Math.cos(this.pitch);
		/** @type {Vec3} */
		const eye = [
			this.target[0] + this.distance * cosPitch * Math.cos(this.yaw),
			this.target[1] + this.distance * cosPitch * Math.sin(this.yaw),
			this.target[2] + this.distance * Math.sin(this.pitch),
		];

		const view = lookAt(eye, this.target, [0, 0, 1]);

		const near = this.distance / 100;
		const far = this.distance + 2 * this.radius;
		const proj = perspective(FOV_Y, aspect, near, far);
		return { view, proj };
	}
}

/**
 * wheelSteps converts a WheelEvent's deltaY/deltaMode into camera.zoom's
 * steps, matching SDL's PreciseY (positive = away from the user = zoom in):
 * one notch is about 1 step in Chrome and Firefox (deltaMode 0, ~100px per
 * notch; deltaMode 1, 3 lines per notch).
 * @param {number} deltaY
 * @param {number} deltaMode
 * @returns {number}
 */
export function wheelSteps(deltaY, deltaMode) {
	return -deltaY / (deltaMode === 1 ? 3 : 100);
}
