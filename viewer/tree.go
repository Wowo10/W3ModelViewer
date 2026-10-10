package viewer

import (
	"fmt"
	"sort"
	"strings"

	"modelView/extract/root"
)

// treeDir is one directory in a modelTree: index 0 is the unnamed root.
type treeDir struct {
	name     string // this directory's own name, in the casing first seen
	parent   int    // index into dirs, -1 for the root
	children []int  // indices into dirs, sorted case-insensitively by name
	models   []int  // indices into modelPicker.models directly inside this directory

	count      int    // models shown in this directory and its subtree, set by refresh
	label      string // "<name> (<count>)###d<index>", rebuilt by refresh
	open       bool   // the user's toggle state with no filter
	filterOpen bool   // forced true while filtering; the user's toggles while filtering go here
}

// modelTree groups modelPicker's models into directories by the path inside
// their mod chain (entry.Name), case-insensitively, built once when the
// model list changes.
type modelTree struct {
	dirs       []treeDir
	modelDir   []int    // model index -> its directory index
	leafLabels []string // model index -> "<file>.mdx [edition, overlay, ...]"
}

// treeRow is one row of the flattened, currently-visible tree, drawn through
// a ListClipper: dir >= 0 for a directory row, model >= 0 for a model leaf.
type treeRow struct {
	depth int
	dir   int
	model int
}

// newModelTree groups models (already sorted by Path, as newModelPicker
// leaves them) into directories split from entry.Name, case-insensitively: a
// directory keeps the casing of the first model seen under it. Within a
// directory, subdirectories are sorted case-insensitively by name, then
// models case-insensitively by file name (models with the same file name
// keep models' existing Path order, i.e. edition/overlay order).
func newModelTree(models []modelEntry) modelTree {
	t := modelTree{dirs: []treeDir{{parent: -1}}}
	byPath := map[string]int{"": 0} // lower-cased dir path -> index into dirs

	t.modelDir = make([]int, len(models))
	t.leafLabels = make([]string, len(models))
	for i, m := range models {
		dir, file := splitName(m.entry.Name)

		parent, lowerPath := 0, ""
		if dir != "" {
			for _, part := range strings.Split(dir, "/") {
				if lowerPath == "" {
					lowerPath = strings.ToLower(part)
				} else {
					lowerPath += "/" + strings.ToLower(part)
				}
				idx, ok := byPath[lowerPath]
				if !ok {
					idx = len(t.dirs)
					t.dirs = append(t.dirs, treeDir{name: part, parent: parent})
					t.dirs[parent].children = append(t.dirs[parent].children, idx)
					byPath[lowerPath] = idx
				}
				parent = idx
			}
		}

		t.dirs[parent].models = append(t.dirs[parent].models, i)
		t.modelDir[i] = parent
		t.leafLabels[i] = leafLabel(file, m.entry.Variant)
	}

	for d := range t.dirs {
		sortChildren(t.dirs, d, models)
	}

	return t
}

// splitName splits entry.Name at its last '/' into directory and file name.
func splitName(name string) (dir, file string) {
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// leafLabel builds a model leaf's label: its file name, edition and overlays.
func leafLabel(file string, v root.Variant) string {
	label := fmt.Sprintf("%s [%s", file, v.Edition)
	for _, o := range v.Overlays {
		label += ", " + o
	}
	return label + "]"
}

// refresh recomputes every directory's count from shown (indexed like
// modelPicker.models) and rebuilds labels; while filtering, every
// directory's filterOpen is forced true so a filter opens every matching
// branch, leaving open (used with no filter) untouched.
func (t *modelTree) refresh(shown []bool, filtering bool) {
	for d := range t.dirs {
		t.dirs[d].count = 0
		if filtering {
			t.dirs[d].filterOpen = true
		}
	}

	for i, ok := range shown {
		if !ok {
			continue
		}
		for d := t.modelDir[i]; d != -1; d = t.dirs[d].parent {
			t.dirs[d].count++
		}
	}

	for d := 1; d < len(t.dirs); d++ {
		dir := &t.dirs[d]
		dir.label = fmt.Sprintf("%s (%d)###d%d", dir.name, dir.count, d)
	}
}

// rows flattens the tree into the rows currently visible: every directory
// with a non-zero count, and the shown models of every open one (open means
// filterOpen while filtering, else open), subdirectories before models, in
// a DFS from the root.
func (t *modelTree) rows(shown []bool, filtering bool) []treeRow {
	var rows []treeRow
	var walk func(d, depth int)
	walk = func(d, depth int) {
		for _, c := range t.dirs[d].children {
			cd := t.dirs[c]
			if cd.count == 0 {
				continue
			}
			rows = append(rows, treeRow{depth: depth, dir: c, model: -1})

			open := cd.open
			if filtering {
				open = cd.filterOpen
			}
			if open {
				walk(c, depth+1)
			}
		}
		for _, m := range t.dirs[d].models {
			if shown[m] {
				rows = append(rows, treeRow{depth: depth, dir: -1, model: m})
			}
		}
	}
	walk(0, 0)
	return rows
}

// sortChildren sorts dirs[d]'s children case-insensitively by name, and its
// models case-insensitively by file name (stable, so same-named models keep
// models' existing order).
func sortChildren(dirs []treeDir, d int, models []modelEntry) {
	children := dirs[d].children
	sort.Slice(children, func(i, j int) bool {
		return strings.ToLower(dirs[children[i]].name) < strings.ToLower(dirs[children[j]].name)
	})

	ms := dirs[d].models
	sort.SliceStable(ms, func(i, j int) bool {
		_, fi := splitName(models[ms[i]].entry.Name)
		_, fj := splitName(models[ms[j]].entry.Name)
		return strings.ToLower(fi) < strings.ToLower(fj)
	})
}
