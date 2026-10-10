// Renders the model panel overlay: path, edition buttons, version/chunk/
// geoset counts, the rendering summary and a geoset table - mirroring
// viewer/models.go's "Model" window (layers column left to WEB-05).

/** @typedef {import("./api.js").Model} Model */
/** @typedef {import("./api.js").Edition} Edition */

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
 * @param {(path: string) => void} onSelectEdition
 */
export function renderModelPanel(container, model, onSelectEdition) {
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
			btn.addEventListener("click", () => onSelectEdition(e.path));
			editions.appendChild(btn);
		}
		container.appendChild(editions);
	}

	addLine(container, `Version: ${model.version}`);
	addLine(container, `Chunks: ${model.chunks}`);
	addLine(container, `Geosets: ${model.geosets.length}`);

	if (model.geosets.length === 0) {
		return;
	}

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
	thead.innerHTML = "<tr><th>geoset</th><th>material</th><th>lod</th><th>vertices</th><th>triangles</th><th>alpha</th></tr>";
	table.appendChild(thead);

	const tbody = document.createElement("tbody");
	let totalVertices = 0;
	let totalTriangles = 0;
	model.geosets.forEach((g, i) => {
		const tr = document.createElement("tr");
		tr.innerHTML = `<td>${i}</td><td>${g.material}</td><td>${g.lod}</td><td>${g.vertices}</td><td>${g.triangles}</td><td>${g.alpha.toFixed(2)}</td>`;
		tbody.appendChild(tr);
		totalVertices += g.vertices;
		totalTriangles += g.triangles;
	});
	const totalRow = document.createElement("tr");
	totalRow.className = "model-geosets-total";
	totalRow.innerHTML = `<td>total</td><td></td><td></td><td>${totalVertices}</td><td>${totalTriangles}</td><td></td>`;
	tbody.appendChild(totalRow);
	table.appendChild(tbody);

	container.appendChild(table);
}
