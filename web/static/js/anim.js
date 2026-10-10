// Playback state and sequence-matching logic for the animation panel,
// ported from viewer/anim.go's playback and viewer/models.go's
// sequenceIndexByName. No DOM, no GL - exercised directly by anim.test.js.

/** @typedef {import("./api.js").Sequence} Sequence */
/** @typedef {import("./api.js").Model} Model */

/**
 * @typedef {Object} Playback
 * @property {boolean} playing
 * @property {boolean} loop
 * @property {number} speed
 * @property {number} frame
 * @property {number} global
 * @property {boolean} scrubbing - the frame slider was active last frame
 */

/**
 * newPlayback returns a Playback that plays a model's default sequence in a
 * loop at 1x as soon as it is picked.
 * @returns {Playback}
 */
export function newPlayback() {
	return { playing: true, loop: true, speed: 1, frame: 0, global: 0, scrubbing: false };
}

/**
 * sequenceRange returns seq's [start, end], treating end < start as start
 * (not seen in wardata; keeps the slider/loop math sane).
 * @param {Sequence} seq
 * @returns {{start: number, end: number}}
 */
export function sequenceRange(seq) {
	return { start: seq.start, end: seq.end < seq.start ? seq.start : seq.end };
}

/**
 * advance steps play.frame and play.global by dt (wall-clock seconds)
 * scaled by speed, when playing and not scrubbing. When frame runs past
 * end, it wraps within [start, end] when loop is set, else clamps to end
 * and stops playing. Mutates play.
 * @param {Playback} play
 * @param {number} dt
 * @param {number} start
 * @param {number} end
 */
export function advance(play, dt, start, end) {
	if (!play.playing || play.scrubbing) {
		return;
	}

	const step = dt * 1000 * play.speed;
	play.frame += step;
	play.global += step;

	if (play.frame > end) {
		if (play.loop) {
			play.frame = end === start ? start : start + ((play.frame - start) % (end - start));
		} else {
			play.frame = end;
			play.playing = false;
		}
	}
}

/**
 * togglePlay applies the play button's rule (restart from start when
 * stopped at end without loop), then flips play.playing. Mutates play.
 * @param {Playback} play
 * @param {number} start
 * @param {number} end
 */
export function togglePlay(play, start, end) {
	if (!play.playing && !play.loop && Math.floor(play.frame) >= end) {
		play.frame = start;
	}
	play.playing = !play.playing;
}

/**
 * @typedef {Object} SequenceRef
 * @property {string} name
 * @property {string} key
 */

/**
 * sequenceIndexByName returns the index of the first sequence whose name
 * matches prev.name case-insensitively, else the first whose key equals
 * prev.key (when non-empty), else -1 (including when prev is null).
 * @param {Sequence[]} seqs
 * @param {SequenceRef | null} prev
 * @returns {number}
 */
export function sequenceIndexByName(seqs, prev) {
	if (!prev) {
		return -1;
	}

	const name = prev.name.toLowerCase();
	for (let i = 0; i < seqs.length; i++) {
		if (seqs[i].name.toLowerCase() === name) {
			return i;
		}
	}

	if (prev.key === "") {
		return -1;
	}
	for (let i = 0; i < seqs.length; i++) {
		if (seqs[i].key === prev.key) {
			return i;
		}
	}
	return -1;
}

/**
 * pickSequence returns the sequence index to pose model at when switching
 * from prev (the previous model's {name, key}, or null): by name/key via
 * sequenceIndexByName, else model.defaultSequence.
 * @param {Model} model
 * @param {SequenceRef | null} prev
 * @returns {number}
 */
export function pickSequence(model, prev) {
	const idx = sequenceIndexByName(model.sequences, prev);
	if (idx !== -1) {
		return idx;
	}
	return model.defaultSequence;
}

/**
 * sequenceLabel names a sequence in the combo, e.g. "Walk 1 (1000 ms)
 * [1000-2000]" (the desktop's "%s (%d ms) [%d-%d]").
 * @param {Sequence} seq
 * @returns {string}
 */
export function sequenceLabel(seq) {
	return `${seq.name} (${seq.end - seq.start} ms) [${seq.start}-${seq.end}]`;
}

/**
 * poseFrame returns the (frame, global) pair to request a pose at: frame
 * floored, global floored and wrapped to an unsigned 32-bit value (matches
 * the desktop's uint32(global) cast; global only ever increases from 0).
 * @param {Playback} play
 * @returns {{frame: number, global: number}}
 */
export function poseFrame(play) {
	return { frame: Math.floor(play.frame), global: Math.floor(play.global) >>> 0 };
}
