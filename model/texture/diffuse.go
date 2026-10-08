package texture

import "modelView/model/mdx"

// Diffuse is where a geoset's diffuse texture comes from.
type Diffuse struct {
	Layer   int    // index into the material's Layers
	Texture int    // index into the model's textures (TEXS)
	CoordID uint32 // the layer's UV set
}

// FindDiffuse picks the diffuse of materials[materialID]: the first texture
// of the first layer whose first texture has a path, else of the first
// layer with a texture (a replaceable one). Classic team-coloured materials
// put a replaceable-id-1 layer under the diffuse (874 lowest-LOD classic
// geosets in wardata; see roadmap/04-tex/EPIC.md). ok is false when
// materialID is out of range or no layer has a first texture with an id
// inside textures.
func FindDiffuse(materials []mdx.Material, textures []mdx.Texture, materialID uint32) (Diffuse, bool) {
	if materialID >= uint32(len(materials)) {
		return Diffuse{}, false
	}

	var fallback Diffuse
	haveFallback := false
	for i, layer := range materials[materialID].Layers {
		if len(layer.Textures) == 0 {
			continue
		}
		id := layer.Textures[0].ID
		if id >= uint32(len(textures)) {
			continue
		}

		d := Diffuse{Layer: i, Texture: int(id), CoordID: layer.CoordID}
		if textures[id].Path != "" {
			return d, true
		}
		if !haveFallback {
			fallback, haveFallback = d, true
		}
	}

	return fallback, haveFallback
}
