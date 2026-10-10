import test from "node:test";
import assert from "node:assert/strict";
import { newPlayback, sequenceRange, advance, togglePlay, sequenceIndexByName, pickSequence, sequenceLabel, poseFrame } from "../static/js/anim.js";

/** @typedef {import("../static/js/api.js").Sequence} Sequence */
/** @typedef {import("../static/js/api.js").Model} Model */

test("newPlayback: defaults to playing, looping, 1x, frame/global 0", () => {
	const p = newPlayback();
	assert.deepEqual(p, { playing: true, loop: true, speed: 1, frame: 0, global: 0, scrubbing: false });
});

test("sequenceRange: end < start treated as start", () => {
	assert.deepEqual(sequenceRange({ name: "a", key: "a#1", start: 100, end: 200 }), { start: 100, end: 200 });
	assert.deepEqual(sequenceRange({ name: "a", key: "a#1", start: 100, end: 50 }), { start: 100, end: 100 });
});

test("advance: no-op when not playing or scrubbing", () => {
	const notPlaying = { playing: false, loop: true, speed: 1, frame: 10, global: 10, scrubbing: false };
	advance(notPlaying, 1, 0, 100);
	assert.deepEqual(notPlaying, { playing: false, loop: true, speed: 1, frame: 10, global: 10, scrubbing: false });

	const scrubbing = { playing: true, loop: true, speed: 1, frame: 10, global: 10, scrubbing: true };
	advance(scrubbing, 1, 0, 100);
	assert.deepEqual(scrubbing, { playing: true, loop: true, speed: 1, frame: 10, global: 10, scrubbing: true });
});

test("advance: steps frame and global by dt*1000*speed", () => {
	const p = { playing: true, loop: true, speed: 2, frame: 0, global: 5, scrubbing: false };
	advance(p, 0.01, 0, 1000); // dt*1000*speed = 20
	assert.equal(p.frame, 20);
	assert.equal(p.global, 25);
});

test("advance: loops within [start, end]", () => {
	const p = { playing: true, loop: true, speed: 1, frame: 90, global: 0, scrubbing: false };
	advance(p, 0.02, 0, 100); // frame -> 110, past end 100
	assert.equal(p.frame, 10);
	assert.equal(p.playing, true);
});

test("advance: loops when start == end", () => {
	const p = { playing: true, loop: true, speed: 1, frame: 0, global: 0, scrubbing: false };
	advance(p, 0.01, 5, 5); // frame -> 10, past end 5, start == end
	assert.equal(p.frame, 5);
});

test("advance: clamps to end and stops when not looping", () => {
	const p = { playing: true, loop: false, speed: 1, frame: 90, global: 0, scrubbing: false };
	advance(p, 0.02, 0, 100); // frame -> 110, past end
	assert.equal(p.frame, 100);
	assert.equal(p.playing, false);
});

test("togglePlay: restarts from start when stopped at end without loop", () => {
	const p = { playing: false, loop: false, speed: 1, frame: 100, global: 0, scrubbing: false };
	togglePlay(p, 0, 100);
	assert.equal(p.frame, 0);
	assert.equal(p.playing, true);
});

test("togglePlay: no restart when looping, or not at end, or already playing", () => {
	const looping = { playing: false, loop: true, speed: 1, frame: 100, global: 0, scrubbing: false };
	togglePlay(looping, 0, 100);
	assert.equal(looping.frame, 100);
	assert.equal(looping.playing, true);

	const notAtEnd = { playing: false, loop: false, speed: 1, frame: 50, global: 0, scrubbing: false };
	togglePlay(notAtEnd, 0, 100);
	assert.equal(notAtEnd.frame, 50);
	assert.equal(notAtEnd.playing, true);

	const playing = { playing: true, loop: false, speed: 1, frame: 100, global: 0, scrubbing: false };
	togglePlay(playing, 0, 100);
	assert.equal(playing.frame, 100);
	assert.equal(playing.playing, false);
});

/** @type {Sequence[]} */
const classicSeqs = [
	{ name: "Stand - 1", key: "stand#1", start: 0, end: 1000 },
	{ name: "Walk", key: "walk#1", start: 1000, end: 2000 },
];

/** @type {Sequence[]} */
const hdSeqs = [
	{ name: "Stand 1", key: "stand#1", start: 0, end: 1500 },
	{ name: "Walk 1", key: "walk#1", start: 1500, end: 2500 },
];

test("sequenceIndexByName: case-insensitive exact name match wins over key", () => {
	// "stand - 1" matches HD's name case-insensitively? No: HD names differ
	// ("Stand 1" vs "Stand - 1"), so name match fails and key decides.
	assert.equal(sequenceIndexByName(hdSeqs, { name: "Stand - 1", key: "stand#1" }), 0);
	assert.equal(sequenceIndexByName(classicSeqs, { name: "WALK", key: "walk#1" }), 1);
});

test("sequenceIndexByName: falls back to key when name differs", () => {
	assert.equal(sequenceIndexByName(hdSeqs, { name: "Walk", key: "walk#1" }), 1);
});

test("sequenceIndexByName: empty key never matches", () => {
	assert.equal(sequenceIndexByName(hdSeqs, { name: "Unknown Sequence", key: "" }), -1);
});

test("sequenceIndexByName: missing match or null prev returns -1", () => {
	assert.equal(sequenceIndexByName(hdSeqs, { name: "Attack", key: "attack#1" }), -1);
	assert.equal(sequenceIndexByName(hdSeqs, null), -1);
});

test("sequenceIndexByName: no sequences returns -1", () => {
	assert.equal(sequenceIndexByName([], { name: "Walk", key: "walk#1" }), -1);
});

test("pickSequence: falls back to model.defaultSequence", () => {
	/** @type {Model} */
	const model = /** @type {any} */ ({ sequences: hdSeqs, defaultSequence: 0 });
	assert.equal(pickSequence(model, { name: "Walk", key: "walk#1" }), 1);
	assert.equal(pickSequence(model, { name: "Attack", key: "attack#1" }), 0);
	assert.equal(pickSequence(model, null), 0);
});

test("sequenceLabel: formats name, duration and range", () => {
	assert.equal(sequenceLabel({ name: "Walk 1", key: "walk#1", start: 1000, end: 2000 }), "Walk 1 (1000 ms) [1000-2000]");
});

test("poseFrame: floors frame and global", () => {
	assert.deepEqual(poseFrame({ playing: true, loop: true, speed: 1, frame: 12.7, global: 345.9, scrubbing: false }), { frame: 12, global: 345 });
});
