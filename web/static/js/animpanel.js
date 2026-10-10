// Renders the animation panel overlay: a sequence select, transport
// controls (play/pause, start, loop, speed) and a frame slider - mirroring
// viewer/anim.go's "Animation" window.

import { sequenceRange, sequenceLabel } from "./anim.js";

/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./anim.js").Playback} Playback */

/**
 * @typedef {Object} AnimPanelCallbacks
 * @property {(index: number) => void} onSelectSequence
 * @property {() => void} onTogglePlay
 * @property {() => void} onStart
 * @property {(loop: boolean) => void} onLoopChange
 * @property {(speed: number) => void} onSpeedChange
 * @property {(frame: number) => void} onFrameInput
 * @property {() => void} onScrubStart
 * @property {() => void} onScrubEnd
 */

/** noopUpdate is returned when the panel has no frame slider to move. */
function noopUpdate() {}

/**
 * renderAnimPanel rebuilds container's contents for model's sequences and
 * play's current state, wiring callbacks to the controls. Returns
 * update(frame), which moves the frame slider and its label while playing
 * (a no-op while scrubbing, so dragging isn't fought over, and when there
 * is no slider to move).
 * @param {HTMLElement} container
 * @param {Model | null} model
 * @param {number} seqIndex
 * @param {Playback} play
 * @param {AnimPanelCallbacks} callbacks
 * @returns {(frame: number) => void}
 */
export function renderAnimPanel(container, model, seqIndex, play, callbacks) {
	container.textContent = "";

	if (!model) {
		container.textContent = "Select a model";
		return noopUpdate;
	}
	if (model.geosets.length === 0) {
		container.textContent = "No geosets";
		return noopUpdate;
	}
	if (model.sequences.length === 0) {
		container.textContent = "No sequences: bind pose";
		return noopUpdate;
	}

	const seq = model.sequences[seqIndex];
	const { start, end } = sequenceRange(seq);

	const select = document.createElement("select");
	model.sequences.forEach((s, i) => {
		const opt = document.createElement("option");
		opt.value = String(i);
		opt.textContent = sequenceLabel(s);
		select.appendChild(opt);
	});
	select.value = String(seqIndex);
	select.addEventListener("change", () => callbacks.onSelectSequence(Number(select.value)));
	container.appendChild(select);

	const playBtn = document.createElement("button");
	playBtn.textContent = play.playing ? "pause" : "play";
	playBtn.addEventListener("click", () => callbacks.onTogglePlay());
	container.appendChild(playBtn);

	const startBtn = document.createElement("button");
	startBtn.textContent = "start";
	startBtn.addEventListener("click", () => callbacks.onStart());
	container.appendChild(startBtn);

	const loopLabel = document.createElement("label");
	const loopCheckbox = document.createElement("input");
	loopCheckbox.type = "checkbox";
	loopCheckbox.checked = play.loop;
	loopCheckbox.addEventListener("change", () => callbacks.onLoopChange(loopCheckbox.checked));
	loopLabel.appendChild(loopCheckbox);
	loopLabel.append(" loop");
	container.appendChild(loopLabel);

	const speedLabel = document.createElement("label");
	speedLabel.append("speed ");
	const speedInput = document.createElement("input");
	speedInput.type = "range";
	speedInput.min = "0.1";
	speedInput.max = "2";
	speedInput.step = "0.1";
	speedInput.value = String(play.speed);
	const speedValue = document.createElement("span");
	speedValue.textContent = `${play.speed.toFixed(1)}x`;
	speedInput.addEventListener("input", () => {
		const v = Number(speedInput.value);
		speedValue.textContent = `${v.toFixed(1)}x`;
		callbacks.onSpeedChange(v);
	});
	speedLabel.appendChild(speedInput);
	speedLabel.appendChild(speedValue);
	container.appendChild(speedLabel);

	const frameLabel = document.createElement("label");
	frameLabel.append("frame ");
	const frameInput = document.createElement("input");
	frameInput.type = "range";
	frameInput.min = String(start);
	frameInput.max = String(end);
	frameInput.step = "1";
	const initialFrame = Math.floor(play.frame);
	frameInput.value = String(initialFrame);
	const frameValue = document.createElement("span");
	frameValue.textContent = String(initialFrame);
	frameInput.addEventListener("pointerdown", () => callbacks.onScrubStart());
	frameInput.addEventListener("pointerup", () => callbacks.onScrubEnd());
	frameInput.addEventListener("input", () => {
		const v = Number(frameInput.value);
		frameValue.textContent = String(v);
		callbacks.onFrameInput(v);
	});
	frameLabel.appendChild(frameInput);
	frameLabel.appendChild(frameValue);
	container.appendChild(frameLabel);

	return (frame) => {
		if (play.scrubbing) {
			return;
		}
		frameInput.value = String(frame);
		frameValue.textContent = String(frame);
	};
}
