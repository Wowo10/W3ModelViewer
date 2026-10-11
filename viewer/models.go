package viewer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/mdx"
	"modelView/model/texture"
)

// modelEntry is one .mdx root entry, with a pre-lowered path for filtering.
type modelEntry struct {
	entry root.Entry
	lower string
}

// selectedModel is the parsed model shown in the "Model" and "Animation"
// windows, posed at (seq, frame).
type selectedModel struct {
	path     string
	name     string // entry.Name, used to find other editions of this asset
	version  uint32
	chunks   int
	geosets  []mdx.Geoset
	priority []int32           // one per geoset, its material's priority plane
	layers   [][]texture.Layer // one slice per geoset, one texture.Layer per material layer

	lods      []mdx.LODTotal // this model's LODs with totals, built once by loadModel
	lodLabels []string       // one per entry in lods, e.g. "LOD 1: 11566 vertices, 15686 triangles"
	lod       int32          // index into lods of the LOD to render, -1 when there are no geosets

	skeleton    mdx.Skeleton
	pivots      int // len(mdx.Model.Pivots()), the matrices array size
	sequences   []mdx.Sequence
	seqLabels   []string // one per sequence, "name [start-end]", built once by loadModel
	geosetAnims []mdx.GeosetAnim
	globals     []uint32

	seq      int32 // index into sequences, -1 when there are none
	frame    int32
	sequence string // sequences[seq].Name, "" when seq == -1

	global uint32 // the global clock passed to the last setPose

	alphas   []float32  // one per geoset, evaluated at (seq, frame)
	matrices []mdx.Mat4 // one per object id, evaluated at (seq, frame)
	pose     int        // bumped by setPose, so the viewer knows to re-upload
}

// identityMat4 is the bind-pose matrix used for every object id when a
// model has no sequences.
var identityMat4 = mdx.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

// setPose recomputes alphas and matrices at (seq, frame, global) and bumps
// pose. seq must be -1 (no sequences) or a valid index into m.sequences.
func (m *selectedModel) setPose(seq, frame int32, global uint32) {
	m.seq, m.frame, m.global = seq, frame, global

	ok := seq != -1
	var at mdx.AnimTime
	if ok {
		s := m.sequences[seq]
		m.sequence = s.Name
		at = mdx.AnimTime{Sequence: s, Frame: uint32(frame), Global: global}
	} else {
		m.sequence = ""
	}

	m.alphas = mdx.GeosetAlphasAtTime(len(m.geosets), m.geosetAnims, m.globals, at, ok)

	if ok {
		m.matrices = m.skeleton.WorldMatrices(at)
	} else {
		m.matrices = make([]mdx.Mat4, m.pivots)
		for i := range m.matrices {
			m.matrices[i] = identityMat4
		}
	}

	m.pose++
}

// modelPicker lists every .mdx root entry with a text filter and an edition
// selector, and holds the currently selected model's parsed data and its
// group of editions.
type modelPicker struct {
	storage  *casc.Storage
	root     root.Root
	resolver *texture.Resolver

	models   []modelEntry
	byPath   map[string]int // models[byPath[path]].entry.Path == path
	editions []string       // "all" followed by every edition that has a model
	tree     modelTree      // directories grouped from models, built once

	filter    string
	filtering bool // p.filter != "" as of the last refresh; while true, toggling a directory touches filterOpen, not open
	edition   int32
	visible   []int     // indices into models matching filter and edition
	shown     []bool    // indices into models, true when matching filter and edition
	rows      []treeRow // the tree's rows currently visible, rebuilt by refresh and by toggling a directory
	selected  int       // index into models, -1 = none

	loaded int // index into models of the model currently in model, -1 = none
	model  selectedModel

	groupName string // entry.Name the current group was computed for
	group     []int  // indices into models of every edition of groupName, sorted

	counts map[[16]byte]modelCounts // by content key, filled as editions are seen

	player int32    // index into playerNames, default 0 = red; which TeamColorNN/TeamGlowNN to draw
	play   playback // the Animation window's play/pause/loop/speed state, kept across model switches

	lod uint32 // the user's chosen LOD value, sticky across model/edition switches; 0 at start
}

// playerNames is the team colour combo's entries: the 24 standard player
// colours (00-23, in the game's order) then "neutral" (24; 24-27 are one
// identical dark grey texture, see roadmap/04-tex/EPIC.md), indexed directly
// by TeamColor/TeamGlow number.
var playerNames = []string{
	"red", "blue", "teal", "purple", "yellow", "orange", "green", "pink",
	"gray", "light blue", "dark green", "brown", "maroon", "navy",
	"turquoise", "violet", "wheat", "peach", "mint", "lavender", "coal",
	"snow", "emerald", "peanut", "neutral",
}

// modelCounts summarizes one edition's geometry for the side-by-side
// editions table.
type modelCounts struct {
	version             uint32
	geosets             int
	lods                []mdx.LODTotal
	vertices, triangles int
}

// countGeosets computes modelCounts for a model's version and geosets,
// given its LODTotals (the caller's, so it isn't recomputed).
func countGeosets(version uint32, geosets []mdx.Geoset, lods []mdx.LODTotal) modelCounts {
	c := modelCounts{version: version, geosets: len(geosets), lods: lods}
	for _, g := range geosets {
		c.vertices += g.VertexCount()
		c.triangles += g.TriangleCount()
	}
	return c
}

// pickLOD returns the index into lods of the entry whose LOD == want, else
// 0 (the lowest LOD); -1 when lods is empty.
func pickLOD(lods []mdx.LODTotal, want uint32) int {
	if len(lods) == 0 {
		return -1
	}
	for i, t := range lods {
		if t.LOD == want {
			return i
		}
	}
	return 0
}

// newModelPicker collects every root entry whose Name ends in ".mdx" (case-
// insensitive), sorted by Path, and the editions present among them.
func newModelPicker(storage *casc.Storage, r root.Root) *modelPicker {
	p := &modelPicker{storage: storage, root: r, resolver: texture.NewResolver(r), selected: -1, loaded: -1, counts: map[[16]byte]modelCounts{}, play: newPlayback()}
	p.model.lod = -1

	seen := map[root.Edition]bool{}
	for _, entry := range r.Entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".mdx") {
			continue
		}
		p.models = append(p.models, modelEntry{entry: entry, lower: strings.ToLower(entry.Path)})
		seen[entry.Variant.Edition] = true
	}

	sort.Slice(p.models, func(i, j int) bool { return p.models[i].entry.Path < p.models[j].entry.Path })

	p.byPath = make(map[string]int, len(p.models))
	for i, m := range p.models {
		p.byPath[m.entry.Path] = i
	}

	p.editions = []string{"all"}
	for _, e := range root.Editions {
		if seen[e] {
			p.editions = append(p.editions, string(e))
		}
	}

	p.tree = newModelTree(p.models)

	p.refresh()
	return p
}

// refresh recomputes visible and shown from filter and edition, then the
// tree's counts and rows.
func (p *modelPicker) refresh() {
	filter := strings.ToLower(p.filter)
	edition := p.editions[p.edition]
	p.filtering = filter != ""

	p.visible = p.visible[:0]
	if p.shown == nil {
		p.shown = make([]bool, len(p.models))
	}
	for i, m := range p.models {
		match := true
		if filter != "" && !strings.Contains(m.lower, filter) {
			match = false
		}
		if edition != "all" && string(m.entry.Variant.Edition) != edition {
			match = false
		}
		p.shown[i] = match
		if match {
			p.visible = append(p.visible, i)
		}
	}

	p.tree.refresh(p.shown, p.filtering)
	p.rows = p.tree.rows(p.shown, p.filtering)
}

// sequenceIndexByName returns the index of the first sequence whose name
// matches name case-insensitively, else the first whose normalised name
// (mdx.SequenceKey) matches, or -1 when neither does (including when name is
// "").
func sequenceIndexByName(seqs []mdx.Sequence, name string) int32 {
	if name == "" {
		return -1
	}
	for i, s := range seqs {
		if strings.EqualFold(s.Name, name) {
			return int32(i)
		}
	}
	key := mdx.SequenceKey(name)
	if key == "" {
		return -1
	}
	for i, s := range seqs {
		if mdx.SequenceKey(s.Name) == key {
			return int32(i)
		}
	}
	return -1
}

// loadModel reads and parses the model at models[idx] through storage,
// keeping its version, chunk count, geosets and skeleton in p.model, posed
// at the previous model's sequence by name when the new model has one, else
// by normalised name, else the default sequence's start frame (identity
// matrices and alpha 1 when the model has no sequences).
func (p *modelPicker) loadModel(idx int) {
	previousSequence := p.model.sequence
	entry := p.models[idx].entry
	data := p.storage.ReadByCKey(entry.CKey)
	m := mdx.Read(data)
	geosets := m.Geosets()

	var priority []int32
	var layers [][]texture.Layer
	var skeleton mdx.Skeleton
	var sequences []mdx.Sequence
	var geosetAnims []mdx.GeosetAnim
	var globals []uint32
	var pivots int

	if len(geosets) > 0 {
		materials := m.Materials()
		textures := m.Textures()
		priority = make([]int32, len(geosets))
		layers = make([][]texture.Layer, len(geosets))
		for i, g := range geosets {
			if g.MaterialID < uint32(len(materials)) {
				priority[i] = materials[g.MaterialID].PriorityPlane
			}
			layers[i] = p.resolver.GeosetLayers(entry, materials, textures, g)
		}

		skeleton = mdx.NewSkeleton(m)
		pivots = len(m.Pivots())
		sequences = m.Sequences()
		geosetAnims = m.GeosetAnims()
		globals = m.GlobalSequences()
	}

	seqLabels := make([]string, len(sequences))
	for i, s := range sequences {
		start, end := int32(s.Start), int32(s.End)
		seqLabels[i] = fmt.Sprintf("%s (%d ms) [%d-%d]", s.Name, end-start, start, end)
	}

	lods := mdx.LODTotals(geosets)
	lodLabels := make([]string, len(lods))
	for i, t := range lods {
		lodLabels[i] = fmt.Sprintf("LOD %d: %d vertices, %d triangles", t.LOD, t.Vertices, t.Triangles)
	}

	p.model = selectedModel{
		path:        entry.Path,
		name:        entry.Name,
		version:     m.Version,
		chunks:      len(m.Chunks),
		geosets:     geosets,
		priority:    priority,
		layers:      layers,
		lods:        lods,
		lodLabels:   lodLabels,
		lod:         int32(pickLOD(lods, p.lod)),
		skeleton:    skeleton,
		pivots:      pivots,
		sequences:   sequences,
		seqLabels:   seqLabels,
		geosetAnims: geosetAnims,
		globals:     globals,
	}

	seq := sequenceIndexByName(sequences, previousSequence)
	if seq == -1 {
		seq = mdx.DefaultSequenceIndex(sequences)
	}
	start := int32(0)
	if seq != -1 {
		start = int32(sequences[seq].Start)
	}
	p.play.frame = float64(start)
	p.model.setPose(seq, start, uint32(uint64(p.play.global)))

	p.loaded = idx
	p.counts[entry.CKey] = countGeosets(m.Version, geosets, lods)
}

// loadGroup recomputes group as every edition of models[idx]'s asset,
// sorted like -variants, when the asset's name (case-insensitively) differs
// from the group already held.
func (p *modelPicker) loadGroup(idx int) {
	name := p.models[idx].entry.Name
	if strings.EqualFold(name, p.groupName) {
		return
	}

	variants := p.root.Variants(name)
	root.SortVariants(variants)

	group := make([]int, len(variants))
	for i, v := range variants {
		group[i] = p.byPath[v.Path]
		p.countEdition(group[i])
	}

	p.group = group
	p.groupName = name
}

// countEdition fills counts for models[idx]'s content key if it isn't
// already cached, reading and parsing the model if necessary.
func (p *modelPicker) countEdition(idx int) {
	entry := p.models[idx].entry
	if _, ok := p.counts[entry.CKey]; ok {
		return
	}

	data := p.storage.ReadByCKey(entry.CKey)
	m := mdx.Read(data)
	geosets := m.Geosets()
	p.counts[entry.CKey] = countGeosets(m.Version, geosets, mdx.LODTotals(geosets))
}

// variantLabel names an edition radio button, e.g. "classic" or
// "reforged (teen)".
func variantLabel(entry root.Entry) string {
	label := string(entry.Variant.Edition)
	if overlays := entry.Variant.OverlayString(); overlays != "" {
		label += " (" + overlays + ")"
	}
	return label
}

// draw shows the "Models" window: a filter box, an edition combo and a
// collapsible, clipped directory tree of matching models (model counts per
// directory; while filtering, every branch containing a match is open).
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
			indent := imgui.CurrentStyle().IndentSpacing()
			rowsDirty := false

			clipper := imgui.NewListClipper()
			clipper.Begin(int32(len(p.rows)))
			for clipper.Step() {
				for i := clipper.DisplayStart(); i < clipper.DisplayEnd(); i++ {
					row := p.rows[i]
					imgui.SetCursorPosX(float32(row.depth) * indent)

					if row.dir != -1 {
						dir := &p.tree.dirs[row.dir]
						open := dir.open
						if p.filtering {
							open = dir.filterOpen
						}
						imgui.SetNextItemOpen(open)
						imgui.TreeNodeExStrV(dir.label, imgui.TreeNodeFlagsNoTreePushOnOpen|imgui.TreeNodeFlagsSpanAvailWidth)
						if imgui.IsItemToggledOpen() {
							if p.filtering {
								dir.filterOpen = !dir.filterOpen
							} else {
								dir.open = !dir.open
							}
							rowsDirty = true
						}
						continue
					}

					flags := imgui.TreeNodeFlagsNoTreePushOnOpen | imgui.TreeNodeFlagsLeaf | imgui.TreeNodeFlagsSpanAvailWidth
					if row.model == p.selected {
						flags |= imgui.TreeNodeFlagsSelected
					}
					imgui.TreeNodeExStrV(p.tree.leafLabels[row.model], flags)
					if imgui.IsItemClicked() {
						p.selected = row.model
					}
				}
			}
			clipper.Destroy()

			if rowsDirty {
				p.rows = p.tree.rows(p.shown, p.filtering)
			}
		}
		imgui.EndChild()
	}
	imgui.End()

	if p.selected != -1 && p.selected != p.loaded {
		p.loadModel(p.selected)
		p.loadGroup(p.selected)
	}

	p.tick(float64(imgui.CurrentIO().DeltaTime()))

	p.drawModel()
	p.drawAnimation()
}

// drawEditionsTable shows counts for every edition in p.group side by side,
// one column per edition (the selected edition's header marked with "*"),
// with a row per count: version, geosets, LODs, rendered LOD, vertices and
// triangles at the rendered LOD, and total vertices and triangles.
func (p *modelPicker) drawEditionsTable() {
	id := fmt.Sprintf("##editions-%s", p.groupName)
	if !imgui.BeginTableV(id, int32(1+len(p.group)), imgui.TableFlagsBorders|imgui.TableFlagsRowBg, imgui.Vec2{}, 0) {
		return
	}
	defer imgui.EndTable()

	imgui.TableSetupColumn("")
	for _, idx := range p.group {
		header := variantLabel(p.models[idx].entry)
		if idx == p.selected {
			header += " *"
		}
		imgui.TableSetupColumn(header)
	}
	imgui.TableHeadersRow()

	for _, row := range editionsTableRows {
		imgui.TableNextRow()
		imgui.TableNextColumn()
		imgui.TextUnformatted(row.label)
		for _, idx := range p.group {
			imgui.TableNextColumn()
			imgui.TextUnformatted(row.value(p.counts[p.models[idx].entry.CKey], p.lod))
		}
	}
}

// editionsTableRows are the rows of drawEditionsTable, built once since they
// hold no per-frame state. Each value is given the edition's modelCounts
// and the viewer's chosen LOD value; the three LOD rows look up that
// edition's rendered LOD with pickLOD (its own lowest LOD when the edition
// doesn't have the chosen value).
var editionsTableRows = []struct {
	label string
	value func(modelCounts, uint32) string
}{
	{"version", func(c modelCounts, lod uint32) string { return fmt.Sprintf("%d", c.version) }},
	{"geosets", func(c modelCounts, lod uint32) string { return fmt.Sprintf("%d", c.geosets) }},
	{"LODs", func(c modelCounts, lod uint32) string { return fmt.Sprintf("%d", len(c.lods)) }},
	{"rendered LOD", func(c modelCounts, lod uint32) string {
		i := pickLOD(c.lods, lod)
		if i == -1 {
			return "-"
		}
		return fmt.Sprintf("%d", c.lods[i].LOD)
	}},
	{"LOD vertices", func(c modelCounts, lod uint32) string {
		i := pickLOD(c.lods, lod)
		if i == -1 {
			return "0"
		}
		return fmt.Sprintf("%d", c.lods[i].Vertices)
	}},
	{"LOD triangles", func(c modelCounts, lod uint32) string {
		i := pickLOD(c.lods, lod)
		if i == -1 {
			return "0"
		}
		return fmt.Sprintf("%d", c.lods[i].Triangles)
	}},
	{"total vertices", func(c modelCounts, lod uint32) string { return fmt.Sprintf("%d", c.vertices) }},
	{"total triangles", func(c modelCounts, lod uint32) string { return fmt.Sprintf("%d", c.triangles) }},
}

// drawModel shows the "Model" window: the selected model's path, version,
// chunk count and a geoset/material/lod/vertices/triangles/layers table
// with a total row, mirroring -mdxinfo; the layers column names each
// geoset's material layers (filter: resolved texture, team colour/glow, or
// why it is drawn grey), joined with " | ", scrollable for long paths.
func (p *modelPicker) drawModel() {
	imgui.SetNextWindowPosV(imgui.Vec2{X: 500, Y: 10}, imgui.CondFirstUseEver, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 560, Y: 440}, imgui.CondFirstUseEver)
	if imgui.Begin("Model") {
		if p.loaded == -1 {
			imgui.Text("Select a model")
		} else {
			m := p.model
			imgui.TextUnformatted(m.path)

			if len(p.group) > 1 {
				imgui.Text("Editions:")
				for i, idx := range p.group {
					if i > 0 {
						imgui.SameLine()
					}
					entry := p.models[idx].entry
					if imgui.RadioButtonBool(variantLabel(entry)+"##"+entry.Path, idx == p.selected) {
						p.selected = idx
					}
				}
			}

			imgui.ComboStrarr("team colour", &p.player, playerNames, int32(len(playerNames)))

			if len(m.lods) > 1 {
				if imgui.ComboStrarr("LOD", &p.model.lod, m.lodLabels, int32(len(m.lodLabels))) {
					p.lod = m.lods[p.model.lod].LOD
				}
			}
			m = p.model

			imgui.Text(fmt.Sprintf("Version: %d", m.version))
			imgui.Text(fmt.Sprintf("Chunks: %d", m.chunks))
			imgui.Text(fmt.Sprintf("Geosets: %d", len(m.geosets)))

			if len(p.group) > 1 {
				imgui.Separator()
				p.drawEditionsTable()
			}

			if len(m.geosets) > 0 {
				lod := m.lods[m.lod].LOD
				atLOD, drawn := 0, 0
				for i, g := range m.geosets {
					if g.LOD != lod {
						continue
					}
					atLOD++
					if m.alphas[i] > 0 {
						drawn++
					}
				}
				hidden := atLOD - drawn
				switch {
				case m.sequence == "":
					imgui.Text(fmt.Sprintf("Rendering LOD %d, no sequences: %d of %d geosets", lod, atLOD, len(m.geosets)))
				case drawn == 0:
					imgui.Text(fmt.Sprintf("Rendering LOD %d in %q at frame %d: nothing visible", lod, m.sequence, m.frame))
				default:
					imgui.Text(fmt.Sprintf("Rendering LOD %d in %q at frame %d: %d of %d geosets (%d hidden)", lod, m.sequence, m.frame, drawn, atLOD, hidden))
				}
			}

			if len(m.geosets) > 0 {
				imgui.Separator()
				if imgui.BeginTableV("##geosets", 8, imgui.TableFlagsScrollX|imgui.TableFlagsScrollY|imgui.TableFlagsRowBg|imgui.TableFlagsBorders, imgui.Vec2{}, 0) {
					imgui.TableSetupColumn("geoset")
					imgui.TableSetupColumn("material")
					imgui.TableSetupColumn("lod")
					imgui.TableSetupColumn("drawn")
					imgui.TableSetupColumn("vertices")
					imgui.TableSetupColumn("triangles")
					imgui.TableSetupColumn("alpha")
					imgui.TableSetupColumn("layers")
					imgui.TableHeadersRow()

					renderedLOD := m.lods[m.lod].LOD
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
						switch {
						case g.LOD != renderedLOD:
							imgui.TextUnformatted("")
						case m.alphas[i] > 0:
							imgui.TextUnformatted("yes")
						default:
							imgui.TextUnformatted("hidden")
						}
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", g.VertexCount()))
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%d", g.TriangleCount()))
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%.2f", m.alphas[i]))
						imgui.TableNextColumn()
						labels := make([]string, len(m.layers[i]))
						for j, l := range m.layers[i] {
							labels[j] = l.Label
						}
						imgui.TextUnformatted(strings.Join(labels, " | "))

						totalVertices += g.VertexCount()
						totalTriangles += g.TriangleCount()
					}

					imgui.TableNextRow()
					imgui.TableNextColumn()
					imgui.TextUnformatted("total")
					imgui.TableNextColumn()
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
