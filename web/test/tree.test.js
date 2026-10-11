import test from "node:test";
import assert from "node:assert/strict";
import { splitName, leafLabel, buildTree, editionOptions, matchModels, countLine, refresh, rows } from "../static/js/tree.js";

/** @param {string} path @param {string} name @param {string} edition @param {string[]} [overlays] @param {?number} [geosets] @returns {import("../static/js/api.js").ModelListEntry} */
function model(path, name, edition, overlays = [], geosets = null) {
	return { path, name, edition, overlays, geosets };
}

// Mixed casing, nested dirs, two editions, a same-named pair, sorted by
// path as the server would return them.
const fixture = [
	model("War3.w3mod:Units/human/Footman/Footman.mdx", "Units/human/Footman/Footman.mdx", "classic"),
	model("War3.w3mod:Units/Human/Peasant/Peasant.mdx", "Units/Human/Peasant/Peasant.mdx", "classic"),
	model("War3.w3mod:_HD.w3mod:Units/Human/Footman/Footman.mdx", "Units/Human/Footman/Footman.mdx", "reforged"),
	model("War3.w3mod:Buildings/human/TownHall/townhall.mdx", "Buildings/human/TownHall/townhall.mdx", "classic", ["teen"]),
	model("War3.w3mod:Buildings/Human/TownHall/TOWNHALL.mdx", "Buildings/Human/TownHall/TOWNHALL.mdx", "classic"),
	model("War3.w3mod:Root.mdx", "Root.mdx", "classic"),
];

/** @param {import("../static/js/api.js").ModelListEntry[]} models @param {string} filter @param {string} edition */
function shownOf(models, filter, edition) {
	return matchModels(models, filter, edition, false).shown;
}

test("splitName: directory and file", () => {
	assert.deepEqual(splitName("Units/Human/Footman/Footman.mdx"), ["Units/Human/Footman", "Footman.mdx"]);
	assert.deepEqual(splitName("Root.mdx"), ["", "Root.mdx"]);
});

test("leafLabel: file, edition, overlays", () => {
	assert.equal(leafLabel("Footman.mdx", "classic", []), "Footman.mdx [classic]");
	assert.equal(leafLabel("townhall.mdx", "classic", ["teen"]), "townhall.mdx [classic, teen]");
});

test("buildTree: first-seen casing, structure, sort order", () => {
	const t = buildTree(fixture);

	// root has Buildings, Units (case-insensitive sort) as children, plus its
	// own direct model (Root.mdx has no dir, so it lives under root).
	assert.equal(t.dirs[0].parent, -1);
	const rootChildren = t.dirs[0].children.map((d) => t.dirs[d].name);
	assert.deepEqual(rootChildren, ["Buildings", "Units"]);
	assert.deepEqual(t.dirs[0].models, [5]); // Root.mdx

	// "Units" keeps the casing first seen (index 0: "Units", not "units").
	const unitsIdx = t.dirs[0].children[1];
	assert.equal(t.dirs[unitsIdx].name, "Units");

	// "human" (first seen lower-case, from index 0) holds Footman/Peasant,
	// sorted case-insensitively.
	const humanIdxUnderUnits = t.dirs[unitsIdx].children[0];
	assert.equal(t.dirs[humanIdxUnderUnits].name, "human");
	const peopleChildren = t.dirs[humanIdxUnderUnits].children.map((d) => t.dirs[d].name);
	assert.deepEqual(peopleChildren, ["Footman", "Peasant"]);

	// Footman directory (first seen as "Footman", the classic one) holds both
	// editions, in their original (path) order.
	const footmanDirIdx = t.dirs[humanIdxUnderUnits].children[0];
	assert.deepEqual(t.dirs[footmanDirIdx].models, [0, 2]);

	// TownHall.mdx and townhall.mdx: same file name case-insensitively, stable
	// order (the lower-cased "townhall.mdx" model comes first in the fixture).
	const buildingsIdx = t.dirs[0].children[0];
	const humanIdxUnderBuildings = t.dirs[buildingsIdx].children[0];
	const townHallIdx = t.dirs[humanIdxUnderBuildings].children[0];
	assert.deepEqual(t.dirs[townHallIdx].models, [3, 4]);

	assert.equal(t.leafLabels[3], "townhall.mdx [classic, teen]");
	assert.equal(t.modelDir[5], 0);
});

test("editionOptions: all + present editions, in root.Editions order", () => {
	assert.deepEqual(editionOptions(fixture), ["all", "classic", "reforged"]);
	assert.deepEqual(editionOptions([model("a", "a", "install")]), ["all", "install"]);
});

test("matchModels: lower-cased substring of path, exact edition", () => {
	const shownAll = shownOf(fixture, "", "all");
	assert.deepEqual(shownAll, [true, true, true, true, true, true]);

	const shownFootman = shownOf(fixture, "footman", "all");
	assert.deepEqual(shownFootman, [true, false, true, false, false, false]);

	const shownReforged = shownOf(fixture, "", "reforged");
	assert.deepEqual(shownReforged, [false, false, true, false, false, false]);

	const shownBoth = shownOf(fixture, "footman", "classic");
	assert.deepEqual(shownBoth, [true, false, false, false, false, false]);
});

test("refresh: counts per dir, with and without filter/edition", () => {
	const t = buildTree(fixture);
	const shown = shownOf(fixture, "", "all");
	refresh(t, shown, false);

	assert.equal(t.dirs[0].count, 6); // every model shown, up to the root
	const unitsIdx = t.dirs[0].children[1];
	assert.equal(t.dirs[unitsIdx].count, 3); // Footman x2 + Peasant

	const filtered = shownOf(fixture, "footman", "all");
	refresh(t, filtered, true);
	assert.equal(t.dirs[0].count, 2);
	assert.equal(t.dirs[unitsIdx].count, 2);
	const buildingsIdx = t.dirs[0].children[0];
	assert.equal(t.dirs[buildingsIdx].count, 0);
});

test("rows: empty filter shows only top-level dirs (closed by default)", () => {
	const t = buildTree(fixture);
	const shown = shownOf(fixture, "", "all");
	refresh(t, shown, false);
	const r = rows(t, shown, false);

	// Every top-level dir, plus the root's own model (Root.mdx); no subdirs'
	// contents since nothing is open.
	assert.deepEqual(
		r.map((row) => ({ depth: row.depth, dir: row.dir, model: row.model })),
		[
			{ depth: 0, dir: t.dirs[0].children[0], model: -1 }, // Buildings
			{ depth: 0, dir: t.dirs[0].children[1], model: -1 }, // Units
			{ depth: 0, dir: -1, model: 5 }, // Root.mdx
		],
	);
});

test("rows: a filter opens every matching branch and hides zero-count dirs", () => {
	const t = buildTree(fixture);
	const shown = shownOf(fixture, "footman", "all");
	refresh(t, shown, true);
	const r = rows(t, shown, true);

	const dirNames = r.filter((row) => row.dir !== -1).map((row) => t.dirs[row.dir].name);
	assert.deepEqual(dirNames, ["Units", "human", "Footman"]); // Buildings hidden (count 0)

	const modelRows = r.filter((row) => row.model !== -1).map((row) => row.model);
	assert.deepEqual(modelRows, [0, 2]);
});

test("toggling filterOpen while filtering leaves open alone and vice versa", () => {
	const t = buildTree(fixture);
	const unitsIdx = t.dirs[0].children[1];

	t.dirs[unitsIdx].open = true;
	const shown = shownOf(fixture, "", "all");
	refresh(t, shown, true); // filtering: forces filterOpen true, doesn't touch open
	assert.equal(t.dirs[unitsIdx].filterOpen, true);
	assert.equal(t.dirs[unitsIdx].open, true);

	t.dirs[unitsIdx].filterOpen = false; // user collapses it while filtering
	refresh(t, shown, false); // no longer filtering: doesn't touch filterOpen
	assert.equal(t.dirs[unitsIdx].filterOpen, false);
	assert.equal(t.dirs[unitsIdx].open, true);
});

// Known-empty (1, 3), known-geometry (0, 4) and unknown (2) models.
const geo = [
	model("a/Smoke.mdx", "A/Smoke.mdx", "classic", [], 5),
	model("a/Fx.mdx", "A/Fx.mdx", "classic", [], 0),
	model("b/Unknown.mdx", "B/Unknown.mdx", "reforged", [], null),
	model("b/Puff.mdx", "B/Puff.mdx", "reforged", [], 0),
	model("c/Footman.mdx", "C/Footman.mdx", "definitive", [], 8),
];

test("matchModels: models without geometry are hidden and counted, unknown ones shown", () => {
	const m = matchModels(geo, "", "all", false);
	assert.deepEqual(m.shown, [true, false, true, false, true]);
	assert.equal(m.hidden, 2);
});

test("matchModels: showEmpty shows everything and hides nothing", () => {
	const m = matchModels(geo, "", "all", true);
	assert.deepEqual(m.shown, [true, true, true, true, true]);
	assert.equal(m.hidden, 0);
});

test("matchModels: hidden counts only models matching filter and edition", () => {
	assert.equal(matchModels(geo, "", "reforged", false).hidden, 1);
	assert.equal(matchModels(geo, "fx", "all", false).hidden, 1);
	assert.equal(matchModels(geo, "footman", "all", false).hidden, 0);
});

test("refresh and rows: directories with only hidden models disappear", () => {
	const t = buildTree(geo);
	const { shown } = matchModels(geo, "", "all", false);
	refresh(t, shown, false);

	assert.equal(t.dirs[0].count, 3);
	const names = rows(t, shown, false).map((r) => t.dirs[r.dir].name);
	assert.deepEqual(names, ["A", "B", "C"]); // A keeps Smoke, B keeps Unknown

	const only = geo.map((m) => ({ ...m, geosets: m.name.startsWith("B/") ? 0 : m.geosets }));
	const m2 = matchModels(only, "", "all", false);
	refresh(t, m2.shown, false);
	assert.deepEqual(rows(t, m2.shown, false).map((r) => t.dirs[r.dir].name), ["A", "C"]);
});

test("countLine: with and without hidden models", () => {
	assert.equal(countLine(3, 5, 0), "3 of 5 models");
	assert.equal(countLine(3, 5, 2), "3 of 5 models (2 without geometry hidden)");
});
