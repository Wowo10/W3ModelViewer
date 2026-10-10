package web

import (
	"encoding/hex"

	"modelView/extract/root"
	"modelView/model/mdx"
	"modelView/model/texture"
)

// textureRef is the JSON shape of a resolved texture: TextureRef in the
// package doc comment.
type textureRef struct {
	Path string `json:"path"`
	CKey string `json:"ckey"`
	URL  string `json:"url"`
}

// textureRefFor builds a textureRef for a resolved .dds root entry.
func (s *Server) textureRefFor(entry root.Entry) textureRef {
	ckey := hex.EncodeToString(entry.CKey[:])
	return textureRef{Path: entry.Path, CKey: ckey, URL: "/api/texture?ckey=" + ckey}
}

// modelListEntry is one entry of /api/models' "models" array.
type modelListEntry struct {
	Path     string   `json:"path"`
	Name     string   `json:"name"`
	Edition  string   `json:"edition"`
	Overlays []string `json:"overlays"`
}

// modelListResponse is /api/models' body.
type modelListResponse struct {
	Models []modelListEntry  `json:"models"`
	Team   []teamTextureRefs `json:"team"`
}

// editionRef is one entry of a modelResponse's "editions" array.
type editionRef struct {
	Path     string   `json:"path"`
	Edition  string   `json:"edition"`
	Overlays []string `json:"overlays"`
}

// sequenceRef is one entry of a modelResponse's "sequences" array.
type sequenceRef struct {
	Name  string `json:"name"`
	Key   string `json:"key"`
	Start uint32 `json:"start"`
	End   uint32 `json:"end"`
}

// meshOffsets is the JSON shape of MeshOffsets: byte offsets into the mesh
// buffer returned by /api/mesh.
type meshOffsets struct {
	Positions int   `json:"positions"`
	Normals   int   `json:"normals"`
	UVs       []int `json:"uvs"`
	Weights   int   `json:"weights"`
	Nodes     int   `json:"nodes"`
	Indices   int   `json:"indices"`
}

// layerRef is the JSON shape of a Layer.
type layerRef struct {
	Filter     uint32      `json:"filter"`
	Shading    uint32      `json:"shading"`
	Alpha      float32     `json:"alpha"`
	UVSet      int         `json:"uvSet"`
	WrapWidth  bool        `json:"wrapWidth"`
	WrapHeight bool        `json:"wrapHeight"`
	Source     string      `json:"source"`
	Label      string      `json:"label"`
	Texture    *textureRef `json:"texture"`
	Mask       *textureRef `json:"mask"`
}

// sourceName maps a texture.Source to the JSON "source" string.
func sourceName(src texture.Source) string {
	switch src {
	case texture.Texture:
		return "texture"
	case texture.TeamColor:
		return "teamColor"
	case texture.TeamGlow:
		return "teamGlow"
	default:
		return "grey"
	}
}

// layerRefFor builds a layerRef from a texture.Layer.
func (s *Server) layerRefFor(l texture.Layer) layerRef {
	ref := layerRef{
		Filter:     uint32(l.Filter),
		Shading:    l.Shading,
		Alpha:      l.Alpha,
		UVSet:      l.UVSet,
		WrapWidth:  l.Flags&1 != 0,
		WrapHeight: l.Flags&2 != 0,
		Source:     sourceName(l.Source),
		Label:      l.Label,
	}
	if l.Source == texture.Texture {
		t := s.textureRefFor(l.Entry)
		ref.Texture = &t
	}
	if l.Masked {
		m := s.textureRefFor(l.Mask)
		ref.Mask = &m
	}
	return ref
}

// geosetRef is the JSON shape of a Geoset.
type geosetRef struct {
	Material  uint32       `json:"material"`
	LOD       uint32       `json:"lod"`
	Vertices  int          `json:"vertices"`
	Triangles int          `json:"triangles"`
	Priority  int32        `json:"priority"`
	Alpha     float32      `json:"alpha"`
	Layers    []layerRef   `json:"layers"`
	Mesh      *meshOffsets `json:"mesh"`
}

// meshRef is a modelResponse's "mesh" field.
type meshRef struct {
	URL  string `json:"url"`
	Size int    `json:"size"`
}

// modelResponse is /api/model's body.
type modelResponse struct {
	Path            string        `json:"path"`
	Name            string        `json:"name"`
	Edition         string        `json:"edition"`
	Overlays        []string      `json:"overlays"`
	CKey            string        `json:"ckey"`
	Version         uint32        `json:"version"`
	Chunks          int           `json:"chunks"`
	Objects         int           `json:"objects"`
	LOD             uint32        `json:"lod"`
	Vertices        int           `json:"vertices"`
	Triangles       int           `json:"triangles"`
	Geosets         []geosetRef   `json:"geosets"`
	Sequences       []sequenceRef `json:"sequences"`
	DefaultSequence int32         `json:"defaultSequence"`
	GlobalSequences []uint32      `json:"globalSequences"`
	Editions        []editionRef  `json:"editions"`
	Mesh            meshRef       `json:"mesh"`
}

// overlaysOrEmpty returns entry.Variant.Overlays, or an empty (never nil)
// slice, so it marshals to [] rather than null.
func overlaysOrEmpty(overlays []string) []string {
	if overlays == nil {
		return []string{}
	}
	return overlays
}

// modelListEntryFor builds a modelListEntry for a .mdx root entry.
func modelListEntryFor(entry root.Entry) modelListEntry {
	return modelListEntry{
		Path:     entry.Path,
		Name:     entry.Name,
		Edition:  string(entry.Variant.Edition),
		Overlays: overlaysOrEmpty(entry.Variant.Overlays),
	}
}

// editionRefFor builds an editionRef for a root entry.
func editionRefFor(entry root.Entry) editionRef {
	return editionRef{
		Path:     entry.Path,
		Edition:  string(entry.Variant.Edition),
		Overlays: overlaysOrEmpty(entry.Variant.Overlays),
	}
}

// sequenceRefFor builds a sequenceRef for a parsed sequence.
func sequenceRefFor(s mdx.Sequence) sequenceRef {
	return sequenceRef{Name: s.Name, Key: mdx.SequenceKey(s.Name), Start: s.Start, End: s.End}
}
