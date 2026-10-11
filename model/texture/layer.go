package texture

import (
	"fmt"

	"modelView/extract/root"
	"modelView/model/mdx"
)

// Source is what a Layer samples.
type Source int

const (
	Grey      Source = iota // flat grey; Label says why
	Texture                 // Entry
	TeamColor               // replaceable 1
	TeamGlow                // replaceable 2
)

// Layer is how one material layer of a geoset is drawn.
type Layer struct {
	Filter  mdx.FilterMode
	Shading uint32
	Alpha   float32 // the layer's own alpha; scaled by the geoset's current alpha at draw time
	Source  Source
	Entry   root.Entry // resolved .dds when Source == Texture
	UVSet   int        // the layer's coord id
	Flags   uint32     // TEXS flags: bit 0 wrap width, bit 1 wrap height
	Label   string     // "<filter>: <path | team colour | team glow | grey: reason>"
	Mask    root.Entry // resolved ORM .dds when Masked
	Masked  bool       // slot 4 is replaceable 1 (team colour) and the slot 2 ORM resolved
}

// GeosetLayers builds one Layer per layer of materials[g.MaterialID], or nil
// when g.MaterialID is out of range.
func (res *Resolver) GeosetLayers(model root.Entry, materials []mdx.Material, textures []mdx.Texture, g mdx.Geoset) []Layer {
	if g.MaterialID >= uint32(len(materials)) {
		return nil
	}

	mLayers := materials[g.MaterialID].Layers
	layers := make([]Layer, len(mLayers))
	for i, layer := range mLayers {
		layers[i] = res.geosetLayer(model, textures, g, layer)
	}
	return layers
}

// geosetLayer builds one Layer from layer's first texture (slot 0 in every
// layer seen in wardata): a resolved stored path, team colour/glow
// (replaceable 1/2; drawn grey until the team colour/glow textures are
// bound), or grey with the reason it can't be drawn: no texture, another
// replaceable id, a coord id outside the geoset's UV sets, or a stored path
// that's missing from the archive or ambiguous after every Resolve fallback
// (see roadmap/04-tex/TEX-05-texture-fallbacks.md).
func (res *Resolver) geosetLayer(model root.Entry, textures []mdx.Texture, g mdx.Geoset, layer mdx.Layer) Layer {
	l := Layer{
		Filter:  layer.FilterMode,
		Shading: layer.ShadingFlags,
		Alpha:   layer.Alpha,
		UVSet:   int(layer.CoordID),
	}

	if len(layer.Textures) == 0 {
		l.Label = fmt.Sprintf("%s: grey: no texture", layer.FilterMode)
		return l
	}

	id := layer.Textures[0].ID
	if id >= uint32(len(textures)) {
		l.Label = fmt.Sprintf("%s: grey: no texture", layer.FilterMode)
		return l
	}

	t := textures[id]
	if t.Path == "" && t.ReplaceableID != 1 && t.ReplaceableID != 2 {
		l.Label = fmt.Sprintf("%s: grey: replaceable %d", layer.FilterMode, t.ReplaceableID)
		return l
	}
	if layer.CoordID >= uint32(len(g.UVs)) {
		l.Label = fmt.Sprintf("%s: grey: no UV set %d", layer.FilterMode, layer.CoordID)
		return l
	}
	l.Flags = t.Flags

	switch {
	case t.Path == "" && t.ReplaceableID == 1:
		l.Source = TeamColor
		l.Label = fmt.Sprintf("%s: team colour", layer.FilterMode)
	case t.Path == "" && t.ReplaceableID == 2:
		l.Source = TeamGlow
		l.Label = fmt.Sprintf("%s: team glow", layer.FilterMode)
	default:
		resolution := res.Resolve(model, t)
		if !resolution.OK() {
			l.Label = fmt.Sprintf("%s: grey: %s", layer.FilterMode, unresolvedReason(t.Path, resolution))
			return l
		}
		l.Source = Texture
		l.Entry = resolution.Entry
		l.Label = fmt.Sprintf("%s: %s%s", layer.FilterMode, resolution.Entry.Path, HowSuffix(resolution.How))

		if mask, ok := res.findTeamColorMask(model, textures, layer); ok {
			l.Mask = mask
			l.Masked = true
			l.Label += " + team colour"
		}
	}

	return l
}

// findTeamColorMask looks for an HD layer's slot 4 (team colour mask) and
// slot 2 (ORM) textures, returning the ORM's resolved root entry when slot 4
// is a replaceable id 1 texture and the slot 2 texture has a stored path
// that resolves.
func (res *Resolver) findTeamColorMask(model root.Entry, textures []mdx.Texture, layer mdx.Layer) (root.Entry, bool) {
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
	resolution := res.Resolve(model, orm)
	return resolution.Entry, resolution.OK()
}

// unresolvedReason describes why refPath didn't resolve: missing from the
// archive entirely, ambiguous among several same-named .dds, or plain
// unresolved (a replaceable id, or own/other-edition chains don't have it
// and no .dds shares its base name under this model's conditions).
func unresolvedReason(refPath string, r Resolution) string {
	switch r.How {
	case Missing:
		return fmt.Sprintf("%s missing from the archive", refPath)
	case Ambiguous:
		return fmt.Sprintf("unresolved %s (%d .dds named %s)", refPath, r.Candidates, r.Base)
	default:
		return fmt.Sprintf("unresolved %s", refPath)
	}
}
