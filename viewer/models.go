package viewer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
	"modelView/extract/root"
)

// modelEntry is one .mdx root entry, with a pre-lowered path for filtering.
type modelEntry struct {
	entry root.Entry
	lower string
}

// modelPicker lists every .mdx root entry with a text filter and an edition
// selector.
type modelPicker struct {
	models   []modelEntry
	editions []string // "all" followed by every edition that has a model

	filter   string
	edition  int32
	visible  []int // indices into models matching filter and edition
	selected int   // index into models, -1 = none
}

// newModelPicker collects every root entry whose Name ends in ".mdx" (case-
// insensitive), sorted by Path, and the editions present among them.
func newModelPicker(r root.Root) *modelPicker {
	p := &modelPicker{selected: -1}

	seen := map[root.Edition]bool{}
	for _, entry := range r.Entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".mdx") {
			continue
		}
		p.models = append(p.models, modelEntry{entry: entry, lower: strings.ToLower(entry.Path)})
		seen[entry.Variant.Edition] = true
	}

	sort.Slice(p.models, func(i, j int) bool { return p.models[i].entry.Path < p.models[j].entry.Path })

	p.editions = []string{"all"}
	for _, e := range root.Editions {
		if seen[e] {
			p.editions = append(p.editions, string(e))
		}
	}

	p.refresh()
	return p
}

// refresh recomputes visible from filter and edition.
func (p *modelPicker) refresh() {
	filter := strings.ToLower(p.filter)
	edition := p.editions[p.edition]

	p.visible = p.visible[:0]
	for i, m := range p.models {
		if filter != "" && !strings.Contains(m.lower, filter) {
			continue
		}
		if edition != "all" && string(m.entry.Variant.Edition) != edition {
			continue
		}
		p.visible = append(p.visible, i)
	}
}

// draw shows the "Models" window: a filter box, an edition combo and a
// clipped, scrolling list of matching paths.
func (p *modelPicker) draw() {
	imgui.SetNextWindowPosV(imgui.Vec2{X: 10, Y: 10}, imgui.CondFirstUseEver, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 480, Y: 660}, imgui.CondFirstUseEver)
	if imgui.Begin("Models") {
		changed := imgui.InputTextWithHint("##filter", "filter", &p.filter, imgui.InputTextFlagsNone, nil)
		if imgui.ComboStrarr("edition", &p.edition, p.editions, int32(len(p.editions))) {
			changed = true
		}
		if changed {
			p.refresh()
		}

		imgui.Text(fmt.Sprintf("%d of %d models", len(p.visible), len(p.models)))
		imgui.Separator()

		if imgui.BeginChildStrV("##modelList", imgui.Vec2{}, imgui.ChildFlagsNone, 0) {
			clipper := imgui.NewListClipper()
			clipper.Begin(int32(len(p.visible)))
			for clipper.Step() {
				for i := clipper.DisplayStart(); i < clipper.DisplayEnd(); i++ {
					idx := p.visible[i]
					if imgui.SelectableBoolV(p.models[idx].entry.Path, idx == p.selected, 0, imgui.Vec2{}) {
						p.selected = idx
					}
				}
			}
			clipper.Destroy()
		}
		imgui.EndChild()
	}
	imgui.End()
}
