// Directory tree and filter, ported from viewer/tree.go and
// modelPicker.refresh/sortChildren (viewer/models.go). Pure data: no DOM.

/** @typedef {import("./api.js").ModelListEntry} ModelListEntry */

/**
 * @typedef {Object} TreeDir
 * @property {string} name - this directory's own name, in the casing first seen
 * @property {number} parent - index into dirs, -1 for the root
 * @property {number[]} children - indices into dirs, sorted case-insensitively by name
 * @property {number[]} models - indices into models directly inside this directory
 * @property {number} count - models shown in this directory and its subtree, set by refresh
 * @property {boolean} open - the user's toggle state with no filter
 * @property {boolean} filterOpen - forced true while filtering; the user's toggles while filtering go here
 */

/**
 * @typedef {Object} ModelTree
 * @property {TreeDir[]} dirs - index 0 is the unnamed root
 * @property {number[]} modelDir - model index -> its directory index
 * @property {string[]} leafLabels - model index -> "<file> [edition, overlay, ...]"
 */

/**
 * @typedef {Object} TreeRow
 * @property {number} depth
 * @property {number} dir - -1 for a model row
 * @property {number} model - -1 for a directory row
 */

/**
 * splitName splits a model's name (path inside its mod chain) at its last
 * '/' into directory and file name.
 * @param {string} name
 * @returns {[string, string]}
 */
export function splitName(name) {
	const i = name.lastIndexOf("/");
	if (i < 0) {
		return ["", name];
	}
	return [name.slice(0, i), name.slice(i + 1)];
}

/**
 * leafLabel builds a model leaf's label: its file name, edition and
 * overlays.
 * @param {string} file
 * @param {string} edition
 * @param {string[]} overlays
 * @returns {string}
 */
export function leafLabel(file, edition, overlays) {
	let label = `${file} [${edition}`;
	for (const o of overlays) {
		label += `, ${o}`;
	}
	return `${label}]`;
}

/**
 * buildTree groups models (assumed sorted by path, as the server leaves
 * them) into directories split from each model's name, case-insensitively:
 * a directory keeps the casing of the first model seen under it. Within a
 * directory, subdirectories are sorted case-insensitively by name, then
 * models case-insensitively by file name (stable, so models with the same
 * file name keep their existing order).
 * @param {ModelListEntry[]} models
 * @returns {ModelTree}
 */
export function buildTree(models) {
	/** @type {TreeDir[]} */
	const dirs = [{ name: "", parent: -1, children: [], models: [], count: 0, open: false, filterOpen: false }];
	const byPath = new Map([["", 0]]); // lower-cased dir path -> index into dirs

	/** @type {number[]} */
	const modelDir = new Array(models.length);
	/** @type {string[]} */
	const leafLabels = new Array(models.length);

	for (let i = 0; i < models.length; i++) {
		const m = models[i];
		const [dir, file] = splitName(m.name);

		let parent = 0;
		let lowerPath = "";
		if (dir !== "") {
			for (const part of dir.split("/")) {
				lowerPath = lowerPath === "" ? part.toLowerCase() : `${lowerPath}/${part.toLowerCase()}`;
				let idx = byPath.get(lowerPath);
				if (idx === undefined) {
					idx = dirs.length;
					dirs.push({ name: part, parent, children: [], models: [], count: 0, open: false, filterOpen: false });
					dirs[parent].children.push(idx);
					byPath.set(lowerPath, idx);
				}
				parent = idx;
			}
		}

		dirs[parent].models.push(i);
		modelDir[i] = parent;
		leafLabels[i] = leafLabel(file, m.edition, m.overlays);
	}

	for (let d = 0; d < dirs.length; d++) {
		sortChildren(dirs, d, models);
	}

	return { dirs, modelDir, leafLabels };
}

/**
 * sortChildren sorts dirs[d]'s children case-insensitively by name, and its
 * models case-insensitively by file name (stable).
 * @param {TreeDir[]} dirs
 * @param {number} d
 * @param {ModelListEntry[]} models
 */
function sortChildren(dirs, d, models) {
	dirs[d].children.sort((a, b) => {
		const na = dirs[a].name.toLowerCase();
		const nb = dirs[b].name.toLowerCase();
		return na < nb ? -1 : na > nb ? 1 : 0;
	});

	const ms = dirs[d].models;
	const fileOf = ms.map((m) => splitName(models[m].name)[1].toLowerCase());
	const order = ms.map((_, i) => i);
	order.sort((i, j) => {
		if (fileOf[i] < fileOf[j]) return -1;
		if (fileOf[i] > fileOf[j]) return 1;
		return i - j; // stable
	});
	const sorted = order.map((i) => ms[i]);
	for (let i = 0; i < ms.length; i++) {
		ms[i] = sorted[i];
	}
}

/**
 * editionOptions returns "all" followed by every edition present among
 * models, in root.Editions order (classic, reforged, definitive, install).
 * @param {ModelListEntry[]} models
 * @returns {string[]}
 */
export function editionOptions(models) {
	const order = ["classic", "reforged", "definitive", "install"];
	const seen = new Set(models.map((m) => m.edition));
	return ["all", ...order.filter((e) => seen.has(e))];
}

/**
 * matchModels returns, per model, whether it's shown: a lower-cased
 * substring match of path, plus edition "all" or an exact match.
 * @param {ModelListEntry[]} models
 * @param {string} filter
 * @param {string} edition
 * @returns {boolean[]}
 */
export function matchModels(models, filter, edition) {
	const lower = filter.toLowerCase();
	return models.map((m) => {
		if (lower !== "" && !m.path.toLowerCase().includes(lower)) {
			return false;
		}
		if (edition !== "all" && m.edition !== edition) {
			return false;
		}
		return true;
	});
}

/**
 * refresh recomputes every directory's count from shown (indexed like
 * models); while filtering, every directory's filterOpen is forced true so
 * a filter opens every matching branch, leaving open (used with no filter)
 * untouched. Mutates tree in place.
 * @param {ModelTree} tree
 * @param {boolean[]} shown
 * @param {boolean} filtering
 */
export function refresh(tree, shown, filtering) {
	for (const dir of tree.dirs) {
		dir.count = 0;
		if (filtering) {
			dir.filterOpen = true;
		}
	}

	for (let i = 0; i < shown.length; i++) {
		if (!shown[i]) {
			continue;
		}
		for (let d = tree.modelDir[i]; d !== -1; d = tree.dirs[d].parent) {
			tree.dirs[d].count++;
		}
	}
}

/**
 * rows flattens the tree into the rows currently visible: every directory
 * with a non-zero count, and the shown models of every open one (open means
 * filterOpen while filtering, else open), subdirectories before models, in
 * a DFS from the root.
 * @param {ModelTree} tree
 * @param {boolean[]} shown
 * @param {boolean} filtering
 * @returns {TreeRow[]}
 */
export function rows(tree, shown, filtering) {
	/** @type {TreeRow[]} */
	const out = [];

	/** @param {number} d @param {number} depth */
	function walk(d, depth) {
		for (const c of tree.dirs[d].children) {
			const cd = tree.dirs[c];
			if (cd.count === 0) {
				continue;
			}
			out.push({ depth, dir: c, model: -1 });

			const open = filtering ? cd.filterOpen : cd.open;
			if (open) {
				walk(c, depth + 1);
			}
		}
		for (const m of tree.dirs[d].models) {
			if (shown[m]) {
				out.push({ depth, dir: -1, model: m });
			}
		}
	}
	walk(0, 0);
	return out;
}
