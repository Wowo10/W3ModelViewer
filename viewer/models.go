package viewer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/mdx"
)

// modelEntry is one .mdx root entry, with a pre-lowered path for filtering.
type modelEntry struct {
	entry root.Entry
	lower string
}

// selectedModel is the parsed model shown in the "Model" window.
type selectedModel struct {
	path    string
	version uint32
	chunks  int
	geosets []mdx.Geoset
}

// modelPicker lists every .mdx root entry with a text filter and an edition
// selector, and holds the currently selected model's parsed data.
type modelPicker struct {
	storage *casc.Storage

	models   []modelEntry
	editions []string // "all" followed by every edition that has a model

	filter   string
	edition  int32
	visible  []int // indices into models matching filter and edition
	selected int   // index into models, -1 = none

	loaded int // index into models of the model currently in model, -1 = none
	model  selectedModel
}

// newModelPicker collects every root entry whose Name ends in ".mdx" (case-
// insensitive), sorted by Path, and the editions present among them.
func newModelPicker(storage *casc.Storage, r root.Root) *modelPicker {
	p := &modelPicker{storage: storage, selected: -1, loaded: -1}

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

// loadModel reads and parses the model at models[idx] through storage,
// keeping its version, chunk count and geosets in p.model.
func (p *modelPicker) loadModel(idx int) {
	entry := p.models[idx].entry
	data := p.storage.ReadByCKey(entry.CKey)
	m := mdx.Read(data)

	p.model = selectedModel{
		path:    entry.Path,
		version: m.Version,
		chunks:  len(m.Chunks),
		geosets: m.Geosets(),
	}
	p.loaded = idx
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

	if p.selected != -1 && p.selected != p.loaded {
		p.loadModel(p.selected)
	}

	p.drawModel()
}

// drawModel shows the "Model" window: the selected model's path, version,
// chunk count and a geoset/material/lod/vertices/triangles table with a
// total row, mirroring -mdxinfo.
func (p *modelPicker) drawModel() {
	imgui.SetNextWindowPosV(imgui.Vec2{X: 500, Y: 270}, imgui.CondFirstUseEver, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 420, Y: 400}, imgui.CondFirstUseEver)
	if imgui.Begin("Model") {
		if p.loaded == -1 {
			imgui.Text("Select a model")
		} else {
			m := p.model
			imgui.TextUnformatted(m.path)
			imgui.Text(fmt.Sprintf("Version: %d", m.version))
			imgui.Text(fmt.Sprintf("Chunks: %d", m.chunks))
			imgui.Text(fmt.Sprintf("Geosets: %d", len(m.geosets)))

			if len(m.geosets) > 0 {
				lod := lowestLOD(m.geosets)
				drawn := 0
				for _, g := range m.geosets {
					if g.LOD == lod {
						drawn++
					}
				}
				imgui.Text(fmt.Sprintf("Rendering LOD %d (%d of %d geosets)", lod, drawn, len(m.geosets)))
			}

			if len(m.geosets) > 0 {
				imgui.Separator()
				if imgui.BeginTableV("##geosets", 5, imgui.TableFlagsScrollY|imgui.TableFlagsRowBg|imgui.TableFlagsBorders, imgui.Vec2{}, 0) {
					imgui.TableSetupColumn("geoset")
					imgui.TableSetupColumn("material")
					imgui.TableSetupColumn("lod")
					imgui.TableSetupColumn("vertices")
					imgui.TableSetupColumn("triangles")
					imgui.TableHeadersRow()

					var totalVertices, totalTriangles int
					for i, g := range m.geosets {
						imgui.TableNextRow()
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", i))
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", g.MaterialID))
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", g.LOD))
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", g.VertexCount()))
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", g.TriangleCount()))

						totalVertices += g.VertexCount()
						totalTriangles += g.TriangleCount()
					}

					imgui.TableNextRow()
					imgui.TableNextColumn()
					imgui.TextUnformatted("total")
					imgui.TableNextColumn()
					imgui.TableNextColumn()
					imgui.TableNextColumn()
					imgui.Text(fmt.Sprintf("%d", totalVertices))
					imgui.TableNextColumn()
					imgui.Text(fmt.Sprintf("%d", totalTriangles))

					imgui.EndTable()
				}
			}
		}
	}
	imgui.End()
}
