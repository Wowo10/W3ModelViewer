// Wires the side panel, canvas and model panel together: fetches the model
// list once, builds the tree, and on selection fetches /api/model +
// /api/mesh, decodes the mesh, uploads its layers' textures and draws it
// under an orbit camera.

import { getModels, getModel, getMesh, getTexture } from "./api.js";
import { buildTree, editionOptions, matchModels, refresh, rows } from "./tree.js";
import { OrbitCamera, wheelSteps } from "./camera.js";
import { decodeMesh, visibleBounds } from "./mesh.js";
import { Renderer } from "./renderer.js";
import { uploadTexture } from "./texture.js";
import { TreeView } from "./treeview.js";
import { renderModelPanel } from "./modelpanel.js";

/** @typedef {import("./api.js").ModelListEntry} ModelListEntry */
/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./api.js").TextureRef} TextureRef */
/** @typedef {import("./texture.js").CompressionExts} CompressionExts */
/** @typedef {import("./texture.js").UploadedTexture} UploadedTexture */

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

	let teamToken = 0;
	/** @param {number} player */
	async function loadTeam(player) {
		const token = ++teamToken;
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
	}
	loadTeam(0);

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

	/**
	 * uniqueTextureRefs collects every distinct texture/mask TextureRef used
	 * by model's lowest-LOD geosets' layers, keyed by content key.
	 * @param {Model} model
	 * @returns {Map<string, TextureRef>}
	 */
	function uniqueTextureRefs(model) {
		/** @type {Map<string, TextureRef>} */
		const refs = new Map();
		for (const g of model.geosets) {
			if (g.mesh === null) {
				continue;
			}
			for (const l of g.layers) {
				if (l.texture) {
					refs.set(l.texture.ckey, l.texture);
				}
				if (l.mask) {
					refs.set(l.mask.ckey, l.mask);
				}
			}
		}
		return refs;
	}

	/** @param {number} modelIndex */
	async function selectModel(modelIndex) {
		selected = modelIndex;
		refreshRows();

		const entry = models[modelIndex];
		const token = ++loadToken;
		panelEl.textContent = "Loading…";

		try {
			const model = await getModel(entry.path);
			if (token !== loadToken) {
				return;
			}
			const meshBuffer = await getMesh(model);
			if (token !== loadToken) {
				return;
			}

			const refs = uniqueTextureRefs(model);
			const entries = await Promise.all(
				[...refs.entries()].map(async ([ckey, ref]) => {
					const buffer = await getTexture(ref);
					return /** @type {[string, ArrayBuffer]} */ ([ckey, buffer]);
				}),
			);
			if (token !== loadToken) {
				return;
			}

			/** @type {Map<string, UploadedTexture>} */
			const textures = new Map();
			for (const [ckey, buffer] of entries) {
				textures.set(ckey, uploadTexture(gl, exts, buffer));
			}
			if (token !== loadToken) {
				for (const t of textures.values()) {
					gl.deleteTexture(t.texture);
				}
				return;
			}

			const decoded = decodeMesh(model, meshBuffer);
			renderer.setModel(meshBuffer, decoded, model, textures);

			const bounds = visibleBounds(decoded);
			if (bounds !== null && model.name.toLowerCase() !== framedName) {
				camera.frame(bounds.min, bounds.max);
				framedName = model.name.toLowerCase();
			}

			renderModelPanel(panelEl, model, (path) => {
				const idx = models.findIndex((m) => m.path === path);
				if (idx !== -1) {
					selectModel(idx);
				}
			});
		} catch (err) {
			if (token !== loadToken) {
				return;
			}
			renderer.releaseMesh();
			panelEl.textContent = `Error: ${err instanceof Error ? err.message : String(err)}`;
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

	function frame() {
		const aspect = canvas.height > 0 ? canvas.width / canvas.height : 1;
		const { view, proj } = camera.matrices(aspect);
		renderer.draw(view, proj, canvas.width, canvas.height);
		requestAnimationFrame(frame);
	}
	requestAnimationFrame(frame);
}

main();
