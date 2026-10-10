// Wires the side panel, canvas and model panel together: fetches the model
// list once, builds the tree, and on selection fetches /api/model +
// /api/mesh, decodes the mesh and draws it under an orbit camera.

import { getModels, getModel, getMesh } from "./api.js";
import { buildTree, editionOptions, matchModels, refresh, rows } from "./tree.js";
import { OrbitCamera, wheelSteps } from "./camera.js";
import { decodeMesh, visibleBounds } from "./mesh.js";
import { Renderer } from "./renderer.js";
import { TreeView } from "./treeview.js";
import { renderModelPanel } from "./modelpanel.js";

/** @typedef {import("./api.js").ModelListEntry} ModelListEntry */

async function main() {
	const canvas = /** @type {HTMLCanvasElement} */ (document.getElementById("canvas"));
	const noWebgl = document.getElementById("no-webgl");
	const gl = canvas.getContext("webgl2");
	if (!gl) {
		canvas.hidden = true;
		if (noWebgl) {
			noWebgl.hidden = false;
		}
		return;
	}

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

			const decoded = decodeMesh(model, meshBuffer);
			renderer.setMesh(meshBuffer, decoded);

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
