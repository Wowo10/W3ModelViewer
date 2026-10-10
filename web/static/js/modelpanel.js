// Renders the model panel overlay: path, edition buttons, team colour
// select, version/chunk/geoset counts, the rendering summary, a geoset
// table with a layers column and a textures table - mirroring
// viewer/models.go's "Model" window.

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
 * @property {TextureInfo[]} textures - every unique texture/mask the model's lowest-LOD layers use
 */

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
 * renderModelPanel rebuilds container's contents for model.
 * @param {HTMLElement} container
 * @param {Model} model
 * @param {ModelPanelOptions} options
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

	addLine(container, `Version: ${model.version}`);
	addLine(container, `Chunks: ${model.chunks}`);
	addLine(container, `Geosets: ${model.geosets.length}`);

	if (model.geosets.length > 0) {
		let atLOD = 0;
		let drawn = 0;
		for (const g of model.geosets) {
			if (g.lod !== model.lod) {
				continue;
			}
			atLOD++;
			if (g.alpha > 0) {
				drawn++;
			}
		}
		addLine(container, `Rendering LOD ${model.lod}: ${drawn} of ${atLOD} geosets (${atLOD - drawn} hidden)`);

		const table = document.createElement("table");
		table.className = "model-geosets";
		const thead = document.createElement("thead");
		thead.innerHTML = "<tr><th>geoset</th><th>material</th><th>lod</th><th>vertices</th><th>triangles</th><th>alpha</th><th>layers</th></tr>";
		table.appendChild(thead);

		const tbody = document.createElement("tbody");
		let totalVertices = 0;
		let totalTriangles = 0;
		model.geosets.forEach((g, i) => {
			const tr = document.createElement("tr");
			const layersText = g.layers.map((l) => l.label).join(" | ");
			const td = document.createElement("td");
			td.textContent = layersText;
			tr.innerHTML = `<td>${i}</td><td>${g.material}</td><td>${g.lod}</td><td>${g.vertices}</td><td>${g.triangles}</td><td>${g.alpha.toFixed(2)}</td>`;
			tr.appendChild(td);
			tbody.appendChild(tr);
			totalVertices += g.vertices;
			totalTriangles += g.triangles;
		});
		const totalRow = document.createElement("tr");
		totalRow.className = "model-geosets-total";
		totalRow.innerHTML = `<td>total</td><td></td><td></td><td>${totalVertices}</td><td>${totalTriangles}</td><td></td><td></td>`;
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
			tr.innerHTML = `<td>${t.path}</td><td>${t.format}</td><td>${t.width}x${t.height}</td><td>${t.mips}</td><td>${t.uploadPath}</td>`;
			tbody.appendChild(tr);
		}
		table.appendChild(tbody);
		container.appendChild(table);
	}
}
