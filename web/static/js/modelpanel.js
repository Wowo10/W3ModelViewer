// Renders the model panel overlay: path, edition buttons, team colour
// select, version/chunk/geoset counts, the rendering summary, a geoset
// table with a layers column and a textures table - mirroring
// viewer/models.go's "Model" window.

import { lodLabel } from "./mesh.js";

/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./api.js").Edition} Edition */

/**
 * playerNames are the team colour select's entries: the 24 standard player
 * colours (00-23, in the game's order) then "neutral" (24), indexed
 * directly by TeamColor/TeamGlow number - the same list as the desktop's
 * viewer/models.go playerNames.
 * @type {string[]}
 */
export const playerNames = [
	"red", "blue", "teal", "purple", "yellow", "orange", "green", "pink",
	"gray", "light blue", "dark green", "brown", "maroon", "navy",
	"turquoise", "violet", "wheat", "peach", "mint", "lavender", "coal",
	"snow", "emerald", "peanut", "neutral",
];

/**
 * @typedef {Object} TextureInfo
 * @property {string} path
 * @property {string} format
 * @property {number} width
 * @property {number} height
 * @property {number} mips
 * @property {string} uploadPath - "S3TC" | "RGTC" | "CPU: ..." | "mask ignored: not DXT5"
 */

/**
 * @typedef {Object} ModelPanelOptions
 * @property {(path: string) => void} onSelectEdition
 * @property {number} player - index into playerNames
 * @property {(player: number) => void} onSelectPlayer
 * @property {number} lodIndex - the rendered LOD, an index into model.lods (-1 without LODs)
 * @property {(index: number) => void} onSelectLOD
 * @property {TextureInfo[]} textures - every unique texture/mask the rendered LOD's layers use
 */

/**
 * @typedef {Object} ModelPanelControls
 * @property {(alphas: Float32Array, seqName: string, frame: number) => void} updatePose
 */

/**
 * renderingLine formats the "Rendering LOD ..." line in the desktop's three
 * forms (viewer/models.go drawModel): no sequences, a sequence with
 * nothing visible, or a sequence with some geosets shown.
 * @param {Model} model
 * @param {number} lod - the rendered LOD
 * @param {number} atLOD - geosets at lod
 * @param {number} drawn - of those, how many have alpha > 0
 * @param {string} seqName - "" for no sequences
 * @param {number} frame
 * @returns {string}
 */
function renderingLine(model, lod, atLOD, drawn, seqName, frame) {
	if (seqName === "") {
		return `Rendering LOD ${lod}, no sequences: ${atLOD} of ${model.geosets.length} geosets`;
	}
	if (drawn === 0) {
		return `Rendering LOD ${lod} in "${seqName}" at frame ${frame}: nothing visible`;
	}
	return `Rendering LOD ${lod} in "${seqName}" at frame ${frame}: ${drawn} of ${atLOD} geosets (${atLOD - drawn} hidden)`;
}

/**
 * drawnText is the geoset table's "drawn" cell: "yes" or "hidden" at the
 * rendered LOD, empty at any other.
 * @param {boolean} atLOD
 * @param {number} alpha
 * @returns {string}
 */
function drawnText(atLOD, alpha) {
	if (!atLOD) {
		return "";
	}
	return alpha > 0 ? "yes" : "hidden";
}

/**
 * variantLabel names an edition button, e.g. "classic" or
 * "reforged (teen)".
 * @param {Edition} e
 * @returns {string}
 */
function variantLabel(e) {
	let label = e.edition;
	if (e.overlays.length > 0) {
		label += ` (${e.overlays.join(",")})`;
	}
	return label;
}

/**
 * addLine appends a text line to container.
 * @param {HTMLElement} container
 * @param {string} text
 */
function addLine(container, text) {
	const div = document.createElement("div");
	div.textContent = text;
	container.appendChild(div);
}

/**
 * renderModelPanel rebuilds container's contents for model, with the
 * "Rendering LOD ..." line and geoset alpha cells initially showing the
 * default sequence's start (model.geosets[].alpha, as served). Returns
 * controls.updatePose to refresh both from a real pose once one arrives.
 * @param {HTMLElement} container
 * @param {Model} model
 * @param {ModelPanelOptions} options
 * @returns {ModelPanelControls}
 */
export function renderModelPanel(container, model, options) {
	container.textContent = "";

	const path = document.createElement("div");
	path.className = "model-path";
	path.textContent = model.path;
	container.appendChild(path);

	if (model.editions.length > 1) {
		const editions = document.createElement("div");
		editions.className = "model-editions";
		for (const e of model.editions) {
			const btn = document.createElement("button");
			btn.textContent = variantLabel(e);
			const current = e.path === model.path;
			btn.disabled = current;
			if (current) {
				btn.classList.add("current");
			}
			btn.addEventListener("click", () => options.onSelectEdition(e.path));
			editions.appendChild(btn);
		}
		container.appendChild(editions);
	}

	const teamRow = document.createElement("div");
	teamRow.className = "model-team";
	const teamLabel = document.createElement("label");
	teamLabel.textContent = "team colour ";
	const teamSelect = document.createElement("select");
	playerNames.forEach((name, i) => {
		const opt = document.createElement("option");
		opt.value = String(i);
		opt.textContent = name;
		teamSelect.appendChild(opt);
	});
	teamSelect.value = String(options.player);
	teamSelect.addEventListener("change", () => options.onSelectPlayer(Number(teamSelect.value)));
	teamLabel.appendChild(teamSelect);
	teamRow.appendChild(teamLabel);
	container.appendChild(teamRow);

	if (model.lods.length > 1) {
		const lodRow = document.createElement("div");
		lodRow.className = "model-lod";
		const lodLabelEl = document.createElement("label");
		lodLabelEl.textContent = "level of detail ";
		const lodSelect = document.createElement("select");
		model.lods.forEach((l, i) => {
			const opt = document.createElement("option");
			opt.value = String(i);
			opt.textContent = lodLabel(l);
			lodSelect.appendChild(opt);
		});
		lodSelect.value = String(options.lodIndex);
		lodSelect.addEventListener("change", () => options.onSelectLOD(Number(lodSelect.value)));
		lodLabelEl.appendChild(lodSelect);
		lodRow.appendChild(lodLabelEl);
		container.appendChild(lodRow);
	}

	addLine(container, `Version: ${model.version}`);
	addLine(container, `Chunks: ${model.chunks}`);
	addLine(container, `Geosets: ${model.geosets.length}`);
	if (model.contents.explain !== "") {
		addLine(container, model.contents.explain);
	}

	/** @type {HTMLElement | null} */
	let renderingEl = null;
	/** @type {HTMLElement[]} */
	const alphaCells = [];
	/** @type {HTMLElement[]} */
	const drawnCells = [];
	const lod = options.lodIndex !== -1 ? model.lods[options.lodIndex].lod : model.lod;

	if (model.geosets.length > 0) {
		let atLOD = 0;
		let drawn = 0;
		for (const g of model.geosets) {
			if (g.lod !== lod) {
				continue;
			}
			atLOD++;
			if (g.alpha > 0) {
				drawn++;
			}
		}
		const defaultSeqName = model.defaultSequence !== -1 ? model.sequences[model.defaultSequence].name : "";
		const defaultFrame = model.defaultSequence !== -1 ? model.sequences[model.defaultSequence].start : 0;

		renderingEl = document.createElement("div");
		renderingEl.textContent = renderingLine(model, lod, atLOD, drawn, defaultSeqName, defaultFrame);
		container.appendChild(renderingEl);

		const table = document.createElement("table");
		table.className = "model-geosets";
		const thead = document.createElement("thead");
		thead.innerHTML = "<tr><th>geoset</th><th>material</th><th>lod</th><th>vertices</th><th>triangles</th><th>alpha</th><th>drawn</th><th>layers</th></tr>";
		table.appendChild(thead);

		const tbody = document.createElement("tbody");
		let totalVertices = 0;
		let totalTriangles = 0;
		model.geosets.forEach((g, i) => {
			const tr = document.createElement("tr");
			tr.innerHTML = `<td>${i}</td><td>${g.material}</td><td>${g.lod}</td><td>${g.vertices}</td><td>${g.triangles}</td>`;

			const alphaTd = document.createElement("td");
			alphaTd.textContent = g.alpha.toFixed(2);
			tr.appendChild(alphaTd);
			alphaCells.push(alphaTd);

			const drawnTd = document.createElement("td");
			drawnTd.textContent = drawnText(g.lod === lod, g.alpha);
			tr.appendChild(drawnTd);
			drawnCells.push(drawnTd);

			const layersTd = document.createElement("td");
			layersTd.textContent = g.layers.map((l) => l.label).join(" | ");
			tr.appendChild(layersTd);

			tbody.appendChild(tr);
			totalVertices += g.vertices;
			totalTriangles += g.triangles;
		});
		const totalRow = document.createElement("tr");
		totalRow.className = "model-geosets-total";
		totalRow.innerHTML = `<td>total</td><td></td><td></td><td>${totalVertices}</td><td>${totalTriangles}</td><td></td><td></td><td></td>`;
		tbody.appendChild(totalRow);
		table.appendChild(tbody);

		container.appendChild(table);
	}

	if (options.textures.length > 0) {
		const table = document.createElement("table");
		table.className = "model-textures";
		const thead = document.createElement("thead");
		thead.innerHTML = "<tr><th>texture</th><th>format</th><th>size</th><th>mips</th><th>upload</th></tr>";
		table.appendChild(thead);

		const tbody = document.createElement("tbody");
		for (const t of options.textures) {
			const tr = document.createElement("tr");
			for (const text of [t.path, t.format, `${t.width}x${t.height}`, String(t.mips), t.uploadPath]) {
				const td = document.createElement("td");
				td.textContent = text;
				tr.appendChild(td);
			}
			tbody.appendChild(tr);
		}
		table.appendChild(tbody);
		container.appendChild(table);
	}

	/**
	 * updatePose rewrites the "Rendering LOD ..." line and every geoset's
	 * alpha cell from a real pose.
	 * @param {Float32Array} alphas
	 * @param {string} seqName
	 * @param {number} frame
	 */
	function updatePose(alphas, seqName, frame) {
		if (!renderingEl) {
			return;
		}

		let atLOD = 0;
		let drawn = 0;
		model.geosets.forEach((g, i) => {
			alphaCells[i].textContent = alphas[i].toFixed(2);
			drawnCells[i].textContent = drawnText(g.lod === lod, alphas[i]);
			if (g.lod !== lod) {
				return;
			}
			atLOD++;
			if (alphas[i] > 0) {
				drawn++;
			}
		});

		renderingEl.textContent = renderingLine(model, lod, atLOD, drawn, seqName, frame);
	}

	return { updatePose };
}
