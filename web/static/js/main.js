// Wires the side panel, canvas, model panel and animation panel together:
// fetches the model list once, builds the tree, and on selection fetches
// /api/model + /api/mesh, decodes the mesh, uploads its layers' textures,
// and poses + draws it under an orbit camera. Playback (anim.js) drives a
// single PoseScheduler (pose.js) that keeps at most one /api/pose request
// in flight; a newly selected model is held as "pending" and only swapped
// in once its first pose has arrived, so there's no bind-pose flash.

import { getModels, getModel, getMesh, getTexture, getPose } from "./api.js";
import { buildTree, editionOptions, matchModels, refresh, rows } from "./tree.js";
import { OrbitCamera, wheelSteps } from "./camera.js";
import { decodeMesh, visibleBounds } from "./mesh.js";
import { parseDDS } from "./dds.js";
import { Renderer } from "./renderer.js";
import { uploadTexture } from "./texture.js";
import { TreeView } from "./treeview.js";
import { renderModelPanel } from "./modelpanel.js";
import { renderAnimPanel } from "./animpanel.js";
import { newPlayback, sequenceRange, advance, togglePlay, pickSequence, poseFrame } from "./anim.js";
import { decodePose, PoseScheduler } from "./pose.js";

/** @typedef {import("./api.js").ModelListEntry} ModelListEntry */
/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./api.js").TextureRef} TextureRef */
/** @typedef {import("./texture.js").CompressionExts} CompressionExts */
/** @typedef {import("./texture.js").UploadedTexture} UploadedTexture */
/** @typedef {import("./modelpanel.js").TextureInfo} TextureInfo */
/** @typedef {import("./modelpanel.js").ModelPanelControls} ModelPanelControls */
/** @typedef {import("./mesh.js").DecodedGeoset} DecodedGeoset */
/** @typedef {import("./pose.js").PoseRequest} PoseRequest */
/** @typedef {import("./anim.js").SequenceRef} SequenceRef */

async function main() {
	const canvas = /** @type {HTMLCanvasElement} */ (document.getElementById("canvas"));
	const noWebgl = document.getElementById("no-webgl");
	const glContext = canvas.getContext("webgl2", { alpha: false });
	if (!glContext) {
		canvas.hidden = true;
		if (noWebgl) {
			noWebgl.hidden = false;
		}
		return;
	}
	// A plain const (not the nullable result above) narrows across the
	// async closures below (loadTeam, selectModel), which a possibly-null
	// outer binding doesn't.
	const gl = glContext;

	const cpuOnly = new URLSearchParams(location.search).get("cpu") === "1";
	/** @type {CompressionExts} */
	const exts = {
		s3tc: !cpuOnly && gl.getExtension("WEBGL_compressed_texture_s3tc") !== null,
		rgtc: !cpuOnly && gl.getExtension("EXT_texture_compression_rgtc") !== null,
	};

	const filterInput = /** @type {HTMLInputElement} */ (document.getElementById("filter"));
	const editionSelect = /** @type {HTMLSelectElement} */ (document.getElementById("edition"));
	const countEl = /** @type {HTMLElement} */ (document.getElementById("count"));
	const scrollEl = /** @type {HTMLElement} */ (document.getElementById("tree"));
	const spacerEl = /** @type {HTMLElement} */ (document.getElementById("tree-spacer"));
	const rowsEl = /** @type {HTMLElement} */ (document.getElementById("tree-rows"));
	const panelEl = /** @type {HTMLElement} */ (document.getElementById("model-panel"));
	const animPanelEl = /** @type {HTMLElement} */ (document.getElementById("anim-panel"));
	const hudEl = /** @type {HTMLElement} */ (document.getElementById("hud"));

	const renderer = new Renderer(gl);
	const camera = new OrbitCamera();

	const list = await getModels();
	const models = list.models;
	const tree = buildTree(models);

	for (const e of editionOptions(models)) {
		const opt = document.createElement("option");
		opt.value = e;
		opt.textContent = e;
		editionSelect.appendChild(opt);
	}

	let currentPlayer = 0;
	let teamToken = 0;
	/** @param {number} player */
	async function loadTeam(player) {
		const token = ++teamToken;
		try {
			const team = list.team[player];
			const [colorBuffer, glowBuffer] = await Promise.all([getTexture(team.color), getTexture(team.glow)]);
			if (token !== teamToken) {
				return;
			}
			const color = uploadTexture(gl, exts, colorBuffer);
			const glow = uploadTexture(gl, exts, glowBuffer);
			if (token !== teamToken) {
				gl.deleteTexture(color.texture);
				gl.deleteTexture(glow.texture);
				return;
			}
			renderer.setTeam(color.texture, glow.texture);
		} catch (err) {
			if (token !== teamToken) {
				return;
			}
			console.error("main: loadTeam:", err);
		}
	}
	loadTeam(currentPlayer);

	/** @param {number} player */
	function selectPlayer(player) {
		currentPlayer = player;
		loadTeam(player);
	}

	let filtering = false;
	/** @type {boolean[]} */
	let shown = matchModels(models, "", "all");
	let selected = -1;
	let loadToken = 0;
	let framedName = "";

	const treeView = new TreeView(scrollEl, spacerEl, rowsEl, {
		onToggleDir(dirIndex) {
			const dir = tree.dirs[dirIndex];
			if (filtering) {
				dir.filterOpen = !dir.filterOpen;
			} else {
				dir.open = !dir.open;
			}
			refreshRows();
		},
		onSelectModel(modelIndex) {
			selectModel(modelIndex);
		},
	});

	function refreshRows() {
		const r = rows(tree, shown, filtering);
		treeView.setData(r, tree, selected, filtering);
	}

	function refreshModels() {
		const filter = filterInput.value;
		const edition = editionSelect.value;
		filtering = filter !== "";
		shown = matchModels(models, filter, edition);
		refresh(tree, shown, filtering);
		refreshRows();
		countEl.textContent = `${shown.filter(Boolean).length} of ${models.length} models`;
	}

	filterInput.addEventListener("input", refreshModels);
	editionSelect.addEventListener("change", refreshModels);
	refreshModels();

	panelEl.textContent = "Select a model";

	// --- Animation state (anim.js), shared across model/edition switches. ---
	const play = newPlayback();
	/** @type {Model | null} */
	let currentModel = null;
	let currentSeqIndex = -1;
	/** @type {SequenceRef | null} */
	let currentSeqRef = null;
	/** @type {ModelPanelControls | null} */
	let panelControls = null;
	/** @type {(frame: number) => void} */
	let animPanelUpdate = () => {};

	/**
	 * pending holds a fetched/decoded model not yet shown, waiting for its
	 * first pose at (seqIndex, frame) so there's no bind-pose flash; the
	 * rAF loop re-wants this same request every tick until it arrives.
	 * @type {{model: Model, meshBuffer: ArrayBuffer, decoded: DecodedGeoset[], textures: Map<string, UploadedTexture>, textureInfos: TextureInfo[], seqIndex: number, frame: number} | null}
	 */
	let pending = null;

	/** @type {number[]} */
	const frameTimestamps = [];
	/** @type {{t: number, latency: number}[]} */
	const latencyReports = [];

	/** @param {number} latencyMs */
	function recordLatency(latencyMs) {
		const t = performance.now();
		latencyReports.push({ t, latency: latencyMs });
		while (latencyReports.length > 0 && t - latencyReports[0].t > 1000) {
			latencyReports.shift();
		}
	}

	/** @param {number} now */
	function updateHud(now) {
		while (frameTimestamps.length > 0 && now - frameTimestamps[0] > 1000) {
			frameTimestamps.shift();
		}
		const fps = frameTimestamps.length;
		if (latencyReports.length === 0) {
			hudEl.textContent = `${fps} fps`;
			return;
		}
		const last = latencyReports[latencyReports.length - 1].latency;
		const avg = latencyReports.reduce((s, r) => s + r.latency, 0) / latencyReports.length;
		hudEl.textContent = `${fps} fps · pose ${last.toFixed(1)} ms (avg ${avg.toFixed(1)} ms)`;
	}

	/**
	 * wantPose asks the scheduler for model's pose at (seqIndex, frame,
	 * play.global): global is 0 when seqIndex is -1 (bind pose, ignored by
	 * the server either way).
	 * @param {Model} model
	 * @param {number} seqIndex
	 * @param {number} frame
	 */
	function wantPose(model, seqIndex, frame) {
		const global = seqIndex === -1 ? 0 : Math.floor(play.global) >>> 0;
		poseScheduler.want({ token: loadToken, model, seq: seqIndex, frame: Math.floor(frame), global });
	}

	/**
	 * rebuildAnimPanel re-renders the animation panel for the current
	 * model/sequence/playback state, replacing animPanelUpdate.
	 */
	function rebuildAnimPanel() {
		animPanelUpdate = renderAnimPanel(animPanelEl, currentModel, currentSeqIndex, play, {
			onSelectSequence(index) {
				const model = currentModel;
				currentSeqIndex = index;
				if (model) {
					currentSeqRef = { name: model.sequences[index].name, key: model.sequences[index].key };
					play.frame = sequenceRange(model.sequences[index]).start;
				}
				rebuildAnimPanel();
			},
			onTogglePlay() {
				const model = currentModel;
				if (!model) {
					return;
				}
				const { start, end } = sequenceRange(model.sequences[currentSeqIndex]);
				togglePlay(play, start, end);
				rebuildAnimPanel();
			},
			onStart() {
				const model = currentModel;
				if (!model) {
					return;
				}
				play.frame = sequenceRange(model.sequences[currentSeqIndex]).start;
			},
			onLoopChange(loop) {
				play.loop = loop;
			},
			onSpeedChange(speed) {
				play.speed = speed;
			},
			onFrameInput(frame) {
				play.frame = frame;
			},
			onScrubStart() {
				play.scrubbing = true;
			},
			onScrubEnd() {
				play.scrubbing = false;
			},
		});
	}

	/**
	 * @param {import("./pose.js").DecodedPose} pose
	 * @param {PoseRequest} req
	 */
	function swapInPending(pose, req) {
		if (!pending) {
			return;
		}
		const { model, meshBuffer, decoded, textures, textureInfos, seqIndex } = pending;
		pending = null;

		currentModel = model;
		currentSeqIndex = seqIndex;
		currentSeqRef = seqIndex !== -1 ? { name: model.sequences[seqIndex].name, key: model.sequences[seqIndex].key } : null;

		renderer.setModel(meshBuffer, decoded, model, textures);
		renderer.setPose(pose.matrices, pose.alphas);

		const bounds = visibleBounds(decoded);
		if (bounds !== null && model.name.toLowerCase() !== framedName) {
			camera.frame(bounds.min, bounds.max);
			framedName = model.name.toLowerCase();
		}

		panelControls = renderModelPanel(panelEl, model, {
			onSelectEdition(path) {
				const idx = models.findIndex((m) => m.path === path);
				if (idx !== -1) {
					selectModel(idx);
				}
			},
			player: currentPlayer,
			onSelectPlayer: selectPlayer,
			textures: textureInfos,
		});

		const seqName = seqIndex !== -1 ? model.sequences[seqIndex].name : "";
		panelControls.updatePose(pose.alphas, seqName, req.frame);

		rebuildAnimPanel();
	}

	const poseScheduler = new PoseScheduler(
		(req) => getPose(req.model, req.seq, req.frame, req.global),
		(buffer, req, latencyMs) => {
			recordLatency(latencyMs);

			let pose;
			try {
				pose = decodePose(buffer, req.model);
			} catch (err) {
				console.error("main: decodePose:", err);
				return;
			}

			if (pending && pending.model === req.model) {
				swapInPending(pose, req);
				return;
			}
			if (currentModel === req.model) {
				renderer.setPose(pose.matrices, pose.alphas);
				const seqName = currentSeqIndex !== -1 ? currentModel.sequences[currentSeqIndex].name : "";
				if (panelControls) {
					panelControls.updatePose(pose.alphas, seqName, req.frame);
				}
				animPanelUpdate(req.frame);
			}
			// Else: a response for a model that's neither pending nor current
			// anymore (superseded twice over) - ignore it.
		},
		(err, req, latencyMs) => {
			recordLatency(latencyMs);
			console.error("main: pose:", err);
			if (pending && pending.model === req.model) {
				for (const t of pending.textures.values()) {
					gl.deleteTexture(t.texture);
				}
				pending = null;
				panelEl.textContent = `Error: ${err instanceof Error ? err.message : String(err)}`;
				animPanelEl.textContent = "";
			}
			// Else: a steady-state pose error for the current model - logged
			// above, keeping the last pose on screen.
		},
		() => performance.now(),
	);

	/**
	 * collectTextureRefs gathers every distinct texture/mask TextureRef used
	 * by model's lowest-LOD geosets' layers, keyed by content key, plus
	 * which ckeys are ever used as a plain texture vs. as a mask (a ckey
	 * used only as a mask and whose DDS isn't DXT5 is never uploaded - it
	 * would be unmasked on the desktop too).
	 * @param {Model} model
	 * @returns {{refs: Map<string, TextureRef>, asTexture: Set<string>, asMask: Set<string>}}
	 */
	function collectTextureRefs(model) {
		/** @type {Map<string, TextureRef>} */
		const refs = new Map();
		/** @type {Set<string>} */
		const asTexture = new Set();
		/** @type {Set<string>} */
		const asMask = new Set();
		for (const g of model.geosets) {
			if (g.mesh === null) {
				continue;
			}
			for (const l of g.layers) {
				if (l.texture) {
					refs.set(l.texture.ckey, l.texture);
					asTexture.add(l.texture.ckey);
				}
				if (l.mask) {
					refs.set(l.mask.ckey, l.mask);
					asMask.add(l.mask.ckey);
				}
			}
		}
		return { refs, asTexture, asMask };
	}

	/** @param {number} modelIndex */
	async function selectModel(modelIndex) {
		selected = modelIndex;
		refreshRows();

		const entry = models[modelIndex];
		const token = ++loadToken;

		// A model picked before this one finished loading never gets shown;
		// free its textures now instead of waiting for its pose to resolve.
		if (pending) {
			for (const t of pending.textures.values()) {
				gl.deleteTexture(t.texture);
			}
			pending = null;
		}

		panelEl.textContent = "Loading…";
		animPanelEl.textContent = "";

		try {
			const model = await getModel(entry.path);
			if (token !== loadToken) {
				return;
			}
			const meshBuffer = await getMesh(model);
			if (token !== loadToken) {
				return;
			}

			const { refs, asTexture, asMask } = collectTextureRefs(model);
			const entries = await Promise.all(
				[...refs.entries()].map(async ([ckey, ref]) => {
					const buffer = await getTexture(ref);
					return /** @type {[string, TextureRef, ArrayBuffer]} */ ([ckey, ref, buffer]);
				}),
			);
			if (token !== loadToken) {
				return;
			}

			/** @type {Map<string, UploadedTexture>} */
			const textures = new Map();
			/** @type {TextureInfo[]} */
			const textureInfos = [];
			for (const [ckey, ref, buffer] of entries) {
				const dds = parseDDS(buffer);
				const maskOnly = asMask.has(ckey) && !asTexture.has(ckey);
				if (maskOnly && dds.format !== "DXT5") {
					textureInfos.push({ path: ref.path, format: dds.format, width: dds.width, height: dds.height, mips: dds.mips.length, uploadPath: "mask ignored: not DXT5" });
					continue;
				}
				const uploaded = uploadTexture(gl, exts, buffer);
				textures.set(ckey, uploaded);
				textureInfos.push({ path: ref.path, format: uploaded.format, width: uploaded.width, height: uploaded.height, mips: uploaded.mips, uploadPath: uploaded.path });
			}
			if (token !== loadToken) {
				for (const t of textures.values()) {
					gl.deleteTexture(t.texture);
				}
				return;
			}

			const decoded = decodeMesh(model, meshBuffer);

			const seqIndex = pickSequence(model, currentSeqRef);
			const frame = seqIndex !== -1 ? sequenceRange(model.sequences[seqIndex]).start : 0;
			play.frame = frame;

			// The rAF loop below re-wants this every tick until it resolves.
			pending = { model, meshBuffer, decoded, textures, textureInfos, seqIndex, frame };
		} catch (err) {
			if (token !== loadToken) {
				return;
			}
			renderer.releaseMesh();
			currentModel = null;
			currentSeqIndex = -1;
			panelControls = null;
			pending = null;
			panelEl.textContent = `Error: ${err instanceof Error ? err.message : String(err)}`;
			animPanelEl.textContent = "";
			animPanelUpdate = () => {};
		}
	}

	let dragging = false;
	canvas.addEventListener("pointerdown", (e) => {
		if (e.button !== 0) {
			return;
		}
		dragging = true;
		canvas.setPointerCapture(e.pointerId);
	});
	canvas.addEventListener("pointermove", (e) => {
		if (!dragging) {
			return;
		}
		camera.rotate(e.movementX, e.movementY);
	});
	canvas.addEventListener("pointerup", (e) => {
		dragging = false;
		canvas.releasePointerCapture(e.pointerId);
	});
	canvas.addEventListener(
		"wheel",
		(e) => {
			e.preventDefault();
			camera.zoom(wheelSteps(e.deltaY, e.deltaMode));
		},
		{ passive: false },
	);

	const resizeObserver = new ResizeObserver(() => {
		const dpr = window.devicePixelRatio || 1;
		canvas.width = Math.max(1, Math.round(canvas.clientWidth * dpr));
		canvas.height = Math.max(1, Math.round(canvas.clientHeight * dpr));
	});
	resizeObserver.observe(canvas);

	/** @type {number | null} */
	let lastTime = null;

	/** @param {number} now */
	function frame(now) {
		frameTimestamps.push(now);

		if (pending) {
			// Keep re-asserting the same request every tick: if a steady-state
			// request for the outgoing model was in flight when this was set,
			// the scheduler sends whichever request is "wanted" once it
			// settles, so this must stay "wanted" until it is sent and swapped
			// in - a one-shot want() here could get overwritten and lost.
			wantPose(pending.model, pending.seqIndex, pending.frame);
		} else if (currentModel && currentSeqIndex !== -1) {
			if (lastTime !== null) {
				const { start, end } = sequenceRange(currentModel.sequences[currentSeqIndex]);
				advance(play, (now - lastTime) / 1000, start, end);
			}
			const pf = poseFrame(play);
			wantPose(currentModel, currentSeqIndex, pf.frame);
			animPanelUpdate(pf.frame);
		}
		lastTime = now;

		updateHud(now);

		const aspect = canvas.height > 0 ? canvas.width / canvas.height : 1;
		const { view, proj } = camera.matrices(aspect);
		renderer.draw(view, proj, canvas.width, canvas.height);
		requestAnimationFrame(frame);
	}
	requestAnimationFrame(frame);
}

main();
