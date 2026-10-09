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
	priority []int32         // one per geoset, its material's priority plane
	layers   [][]geosetLayer // one slice per geoset, one geosetLayer per material layer

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

// layerSource is what a layer samples.
type layerSource int

const (
	sourceGrey      layerSource = iota // flat grey; label says why
	sourceTexture                      // entry
	sourceTeamColor                    // replaceable 1
	sourceTeamGlow                     // replaceable 2
)

// geosetLayer is how one material layer of a geoset is drawn.
type geosetLayer struct {
	filter  mdx.FilterMode
	shading uint32
	alpha   float32 // the layer's own alpha; scaled by the geoset's current alpha at draw time
	source  layerSource
	entry   root.Entry // resolved .dds when source == sourceTexture
	uvSet   int        // the layer's coord id
	flags   uint32     // TEXS flags: bit 0 wrap width, bit 1 wrap height
	label   string     // "<filter>: <path | team colour | team glow | grey: reason>"
	mask    root.Entry // resolved ORM .dds when masked
	masked  bool       // slot 4 is replaceable 1 (team colour) and the slot 2 ORM resolved
}

// findGeosetLayers builds one geosetLayer per layer of materials[g.MaterialID],
// or nil when g.MaterialID is out of range.
func findGeosetLayers(res *texture.Resolver, model root.Entry, materials []mdx.Material, textures []mdx.Texture, g mdx.Geoset) []geosetLayer {
	if g.MaterialID >= uint32(len(materials)) {
		return nil
	}

	mLayers := materials[g.MaterialID].Layers
	layers := make([]geosetLayer, len(mLayers))
	for i, layer := range mLayers {
		layers[i] = findGeosetLayer(res, model, textures, g, layer)
	}
	return layers
}

// findGeosetLayer builds one geosetLayer from layer's first texture (slot 0
// in every layer seen in wardata): a resolved stored path, team colour/glow
// (replaceable 1/2; drawn grey until the team colour/glow textures are
// bound), or grey with the reason it can't be drawn: no texture, another
// replaceable id, an unresolved stored path, or a coord id outside the
// geoset's UV sets.
func findGeosetLayer(res *texture.Resolver, model root.Entry, textures []mdx.Texture, g mdx.Geoset, layer mdx.Layer) geosetLayer {
	l := geosetLayer{
		filter:  layer.FilterMode,
		shading: layer.ShadingFlags,
		alpha:   layer.Alpha,
		uvSet:   int(layer.CoordID),
	}

	if len(layer.Textures) == 0 {
		l.label = fmt.Sprintf("%s: grey: no texture", layer.FilterMode)
		return l
	}

	id := layer.Textures[0].ID
	if id >= uint32(len(textures)) {
		l.label = fmt.Sprintf("%s: grey: no texture", layer.FilterMode)
		return l
	}

	t := textures[id]
	if t.Path == "" && t.ReplaceableID != 1 && t.ReplaceableID != 2 {
		l.label = fmt.Sprintf("%s: grey: replaceable %d", layer.FilterMode, t.ReplaceableID)
		return l
	}
	if layer.CoordID >= uint32(len(g.UVs)) {
		l.label = fmt.Sprintf("%s: grey: no UV set %d", layer.FilterMode, layer.CoordID)
		return l
	}
	l.flags = t.Flags

	switch {
	case t.Path == "" && t.ReplaceableID == 1:
		l.source = sourceTeamColor
		l.label = fmt.Sprintf("%s: team colour", layer.FilterMode)
	case t.Path == "" && t.ReplaceableID == 2:
		l.source = sourceTeamGlow
		l.label = fmt.Sprintf("%s: team glow", layer.FilterMode)
	default:
		resolved, ok := res.Resolve(model, t)
		if !ok {
			l.label = fmt.Sprintf("%s: grey: unresolved %s", layer.FilterMode, t.Path)
			return l
		}
		l.source = sourceTexture
		l.entry = resolved
		l.label = fmt.Sprintf("%s: %s", layer.FilterMode, resolved.Path)

		if mask, ok := findTeamColorMask(res, model, textures, layer); ok {
			l.mask = mask
			l.masked = true
			l.label += " + team colour"
		}
	}

	return l
}

// findTeamColorMask looks for an HD layer's slot 4 (team colour mask) and
// slot 2 (ORM) textures, returning the ORM's resolved root entry when slot 4
// is a replaceable id 1 texture and the slot 2 texture has a stored path
// that resolves.
func findTeamColorMask(res *texture.Resolver, model root.Entry, textures []mdx.Texture, layer mdx.Layer) (root.Entry, bool) {
	var haveMask bool
	var ormID uint32
	var haveORM bool

	for _, lt := range layer.Textures {
		switch lt.Slot {
		case 4:
			if lt.ID < uint32(len(textures)) {
				t := textures[lt.ID]
				haveMask = t.Path == "" && t.ReplaceableID == 1
			}
		case 2:
			ormID, haveORM = lt.ID, true
		}
	}

	if !haveMask || !haveORM || ormID >= uint32(len(textures)) {
		return root.Entry{}, false
	}

	orm := textures[ormID]
	if orm.Path == "" {
		return root.Entry{}, false
	}
	return res.Resolve(model, orm)
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

	filter   string
	edition  int32
	visible  []int // indices into models matching filter and edition
	selected int   // index into models, -1 = none

	loaded int // index into models of the model currently in model, -1 = none
	model  selectedModel

	groupName string // entry.Name the current group was computed for
	group     []int  // indices into models of every edition of groupName, sorted

	counts map[[16]byte]modelCounts // by content key, filled as editions are seen

	player int32    // index into playerNames, default 0 = red; which TeamColorNN/TeamGlowNN to draw
	play   playback // the Animation window's play/pause/loop/speed state, kept across model switches
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
	version                   uint32
	geosets, lods             int
	lod                       uint32
	lodVertices, lodTriangles int
	vertices, triangles       int
}

// countGeosets computes modelCounts for a model's version and geosets.
func countGeosets(version uint32, geosets []mdx.Geoset) modelCounts {
	c := modelCounts{version: version, geosets: len(geosets)}
	if len(geosets) == 0 {
		return c
	}

	lods := map[uint32]bool{}
	c.lod = lowestLOD(geosets)
	for _, g := range geosets {
		lods[g.LOD] = true
		c.vertices += g.VertexCount()
		c.triangles += g.TriangleCount()
		if g.LOD == c.lod {
			c.lodVertices += g.VertexCount()
			c.lodTriangles += g.TriangleCount()
		}
	}
	c.lods = len(lods)
	return c
}

// newModelPicker collects every root entry whose Name ends in ".mdx" (case-
// insensitive), sorted by Path, and the editions present among them.
func newModelPicker(storage *casc.Storage, r root.Root) *modelPicker {
	p := &modelPicker{storage: storage, root: r, resolver: texture.NewResolver(r), selected: -1, loaded: -1, counts: map[[16]byte]modelCounts{}, play: newPlayback()}

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

// defaultSequenceIndex returns the index of the sequence mdx.DefaultSequence
// picks, or -1 when seqs is empty.
func defaultSequenceIndex(seqs []mdx.Sequence) int32 {
	def, ok := mdx.DefaultSequence(seqs)
	if !ok {
		return -1
	}
	for i, s := range seqs {
		if s == def {
			return int32(i)
		}
	}
	return -1
}

// loadModel reads and parses the model at models[idx] through storage,
// keeping its version, chunk count, geosets and skeleton in p.model, posed
// at the default sequence's start frame (identity matrices and alpha 1 when
// the model has no sequences).
func (p *modelPicker) loadModel(idx int) {
	entry := p.models[idx].entry
	data := p.storage.ReadByCKey(entry.CKey)
	m := mdx.Read(data)
	geosets := m.Geosets()

	var priority []int32
	var layers [][]geosetLayer
	var skeleton mdx.Skeleton
	var sequences []mdx.Sequence
	var geosetAnims []mdx.GeosetAnim
	var globals []uint32
	var pivots int

	if len(geosets) > 0 {
		materials := m.Materials()
		textures := m.Textures()
		priority = make([]int32, len(geosets))
		layers = make([][]geosetLayer, len(geosets))
		for i, g := range geosets {
			if g.MaterialID < uint32(len(materials)) {
				priority[i] = materials[g.MaterialID].PriorityPlane
			}
			layers[i] = findGeosetLayers(p.resolver, entry, materials, textures, g)
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

	p.model = selectedModel{
		path:        entry.Path,
		name:        entry.Name,
		version:     m.Version,
		chunks:      len(m.Chunks),
		geosets:     geosets,
		priority:    priority,
		layers:      layers,
		skeleton:    skeleton,
		pivots:      pivots,
		sequences:   sequences,
		seqLabels:   seqLabels,
		geosetAnims: geosetAnims,
		globals:     globals,
	}

	def := defaultSequenceIndex(sequences)
	start := int32(0)
	if def != -1 {
		start = int32(sequences[def].Start)
	}
	p.play.frame = float64(start)
	p.model.setPose(def, start, uint32(uint64(p.play.global)))

	p.loaded = idx
	p.counts[entry.CKey] = countGeosets(m.Version, geosets)
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
	p.counts[entry.CKey] = countGeosets(m.Version, m.Geosets())
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
			imgui.TextUnformatted(row.value(p.counts[p.models[idx].entry.CKey]))
		}
	}
}

// editionsTableRows are the rows of drawEditionsTable, built once since they
// hold no per-frame state.
var editionsTableRows = []struct {
	label string
	value func(modelCounts) string
}{
	{"version", func(c modelCounts) string { return fmt.Sprintf("%d", c.version) }},
	{"geosets", func(c modelCounts) string { return fmt.Sprintf("%d", c.geosets) }},
	{"LODs", func(c modelCounts) string { return fmt.Sprintf("%d", c.lods) }},
	{"rendered LOD", func(c modelCounts) string {
		if c.geosets == 0 {
			return "-"
		}
		return fmt.Sprintf("%d", c.lod)
	}},
	{"LOD vertices", func(c modelCounts) string { return fmt.Sprintf("%d", c.lodVertices) }},
	{"LOD triangles", func(c modelCounts) string { return fmt.Sprintf("%d", c.lodTriangles) }},
	{"total vertices", func(c modelCounts) string { return fmt.Sprintf("%d", c.vertices) }},
	{"total triangles", func(c modelCounts) string { return fmt.Sprintf("%d", c.triangles) }},
}

// drawFPS writes the frame rate right-aligned on the current line and puts
// the cursor back, so the window's content starts on that same line.
func drawFPS() {
	fr := imgui.CurrentIO().Framerate()
	text := fmt.Sprintf("%.0f FPS (%.1f ms)", fr, 1000/fr)

	pos := imgui.CursorPos()
	imgui.SetCursorPos(imgui.Vec2{X: pos.X + imgui.ContentRegionAvail().X - imgui.CalcTextSize(text).X, Y: pos.Y})
	imgui.TextDisabled(text)
	imgui.SetCursorPos(pos)
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
		drawFPS()

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

			imgui.Text(fmt.Sprintf("Version: %d", m.version))
			imgui.Text(fmt.Sprintf("Chunks: %d", m.chunks))
			imgui.Text(fmt.Sprintf("Geosets: %d", len(m.geosets)))

			if len(p.group) > 1 {
				imgui.Separator()
				p.drawEditionsTable()
			}

			if len(m.geosets) > 0 {
				lod := lowestLOD(m.geosets)
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
				if imgui.BeginTableV("##geosets", 7, imgui.TableFlagsScrollX|imgui.TableFlagsScrollY|imgui.TableFlagsRowBg|imgui.TableFlagsBorders, imgui.Vec2{}, 0) {
					imgui.TableSetupColumn("geoset")
					imgui.TableSetupColumn("material")
					imgui.TableSetupColumn("lod")
					imgui.TableSetupColumn("vertices")
					imgui.TableSetupColumn("triangles")
					imgui.TableSetupColumn("alpha")
					imgui.TableSetupColumn("layers")
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
						imgui.TableNextColumn()
						imgui.Text(fmt.Sprintf("%.2f", m.alphas[i]))
						imgui.TableNextColumn()
						labels := make([]string, len(m.layers[i]))
						for j, l := range m.layers[i] {
							labels[j] = l.label
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
