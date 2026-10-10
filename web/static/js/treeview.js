// Virtualised directory tree list: renders only the rows in (and around)
// the scroll viewport, so a filter matching ~17.7K rows stays cheap.

/** @typedef {import("./tree.js").ModelTree} ModelTree */
/** @typedef {import("./tree.js").TreeRow} TreeRow */

const ROW_HEIGHT = 22; // px
const OVERSCAN = 10; // extra rows rendered above/below the viewport
const INDENT = 16; // px per depth level

/**
 * @typedef {Object} TreeViewHandlers
 * @property {(dirIndex: number) => void} onToggleDir
 * @property {(modelIndex: number) => void} onSelectModel
 */

/**
 * TreeView keeps a spacer sized to the full row count and only populates
 * rowsEl with the rows currently in view, translated into place.
 */
export class TreeView {
	/**
	 * @param {HTMLElement} scrollEl - the scrollable container
	 * @param {HTMLElement} spacerEl - sized to rows.length * ROW_HEIGHT
	 * @param {HTMLElement} rowsEl - absolutely positioned inside spacerEl, holds the visible rows
	 * @param {TreeViewHandlers} handlers
	 */
	constructor(scrollEl, spacerEl, rowsEl, handlers) {
		this.scrollEl = scrollEl;
		this.spacerEl = spacerEl;
		this.rowsEl = rowsEl;
		this.handlers = handlers;

		/** @type {TreeRow[]} */
		this.rows = [];
		/** @type {ModelTree | null} */
		this.tree = null;
		this.selected = -1;
		this.filtering = false;

		scrollEl.addEventListener("scroll", () => this.render());
	}

	/**
	 * @param {TreeRow[]} rows
	 * @param {ModelTree} tree
	 * @param {number} selected - selected model index, -1 = none
	 * @param {boolean} filtering
	 */
	setData(rows, tree, selected, filtering) {
		this.rows = rows;
		this.tree = tree;
		this.selected = selected;
		this.filtering = filtering;
		this.spacerEl.style.height = `${rows.length * ROW_HEIGHT}px`;
		this.render();
	}

	render() {
		const tree = this.tree;
		if (!tree) {
			return;
		}

		const scrollTop = this.scrollEl.scrollTop;
		const viewHeight = this.scrollEl.clientHeight;
		const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
		const end = Math.min(this.rows.length, Math.ceil((scrollTop + viewHeight) / ROW_HEIGHT) + OVERSCAN);

		this.rowsEl.style.transform = `translateY(${start * ROW_HEIGHT}px)`;
		this.rowsEl.textContent = "";

		for (let i = start; i < end; i++) {
			const row = this.rows[i];
			const el = document.createElement("div");
			el.className = "tree-row";
			el.style.height = `${ROW_HEIGHT}px`;
			el.style.paddingLeft = `${row.depth * INDENT}px`;

			if (row.dir !== -1) {
				const dir = tree.dirs[row.dir];
				const open = this.filtering ? dir.filterOpen : dir.open;
				el.classList.add("tree-dir");
				el.textContent = `${open ? "▾" : "▸"} ${dir.name} (${dir.count})`;
				el.addEventListener("click", () => this.handlers.onToggleDir(row.dir));
			} else {
				el.classList.add("tree-model");
				if (row.model === this.selected) {
					el.classList.add("selected");
				}
				el.textContent = tree.leafLabels[row.model];
				el.addEventListener("click", () => this.handlers.onSelectModel(row.model));
			}

			this.rowsEl.appendChild(el);
		}
	}
}
