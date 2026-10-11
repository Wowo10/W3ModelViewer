// Package texture resolves an MDX texture reference to its stored .dds root
// entry and reads a DDS header. See roadmap/04-tex/EPIC.md for the path
// rule, the resolution counts and the DDS formats found in wardata.
package texture

import (
	"path"
	"strings"

	"modelView/extract/root"
	"modelView/model/mdx"
)

// teamColorPath is the stored name replaceable id 1 (team colour) maps to;
// it exists only in the base game (see Resolve).
const teamColorPath = "ReplaceableTextures/TeamColor/TeamColor00.dds"

// How says which step of Resolve found a Resolution, or why it didn't.
// See roadmap/04-tex/TEX-05-texture-fallbacks.md for the fallback order.
type How int

const (
	Unresolved   How = iota // replaceable other than 1, or an install file
	OwnChain                // the model's own chain or a parent
	OtherEdition            // the same name in another edition's chain
	ByFileName              // the file's base name, unique in the archive (or unique to the model's edition)
	Ambiguous               // several .dds share the base name and none is the model's own edition
	Missing                 // no .dds anywhere has that base name
)

// Resolution is the outcome of resolving one texture reference.
type Resolution struct {
	Entry      root.Entry
	How        How
	Candidates int    // .dds entries sharing the base name, set for Ambiguous
	Base       string // the stored base name tried, set for Ambiguous and Missing
}

// OK reports whether Resolution.Entry holds a usable texture.
func (r Resolution) OK() bool {
	return r.How == OwnChain || r.How == OtherEdition || r.How == ByFileName
}

// HowSuffix labels a resolved Resolution by how it was found: "" for the
// model's own chain, " (other edition)" or " (by file name)" for a
// fallback.
func HowSuffix(how How) string {
	switch how {
	case OtherEdition:
		return " (other edition)"
	case ByFileName:
		return " (by file name)"
	default:
		return ""
	}
}

// otherEditionChains lists, per edition, the other edition chains to try
// after the model's own chain fails, in order. Install never reaches this
// table (Resolve returns Unresolved first).
var otherEditionChains = map[root.Edition][]string{
	root.Classic:    {"War3.w3mod:_DE.w3mod:", "War3.w3mod:_HD.w3mod:"},
	root.Reforged:   {"War3.w3mod:_DE.w3mod:"},
	root.Definitive: {"War3.w3mod:_HD.w3mod:"},
}

// Resolver maps an MDX texture reference to the root entry holding its
// stored .dds, by the rule in roadmap/04-tex/EPIC.md: swap backslashes for
// '/' and the extension for .dds, then look up case-insensitively in the
// model's own mod chain, then each parent chain (the chain with its last
// segment repeatedly dropped) down to "War3.w3mod:". When that fails,
// Resolve falls back to the other edition chains, then a unique file name
// (roadmap/04-tex/TEX-05-texture-fallbacks.md).
type Resolver struct {
	root   root.Root
	byFold map[string]int   // lower(entry.Path) -> index into root.Entries
	byBase map[string][]int // lower(path.Base(entry.Name)) -> indices, archive .dds entries only
}

// NewResolver indexes every root entry's path folded to lower case, for
// case-insensitive lookup (850 of wardata's resolutions only match this
// way), and every archive .dds entry's base name, for the by-file-name
// fallback. On a fold collision the first entry wins; none are seen in
// wardata, and this never panics since a future build adding one shouldn't
// break startup for callers that build a Resolver there.
func NewResolver(r root.Root) *Resolver {
	byFold := make(map[string]int, len(r.Entries))
	byBase := make(map[string][]int)
	for i, entry := range r.Entries {
		fold := strings.ToLower(entry.Path)
		if _, exists := byFold[fold]; !exists {
			byFold[fold] = i
		}

		if entry.ModChain != "" && strings.HasSuffix(strings.ToLower(entry.Name), ".dds") {
			base := strings.ToLower(path.Base(entry.Name))
			byBase[base] = append(byBase[base], i)
		}
	}
	return &Resolver{root: r, byFold: byFold, byBase: byBase}
}

// Resolve finds the root entry for t's reference from model (the entry for
// the .mdx that references t). It returns Unresolved when t is a
// replaceable texture other than team colour (id 1) or model has no mod
// chain (an install file). Team colour only ever resolves through the
// own/parent chain walk (OwnChain), since it exists in the base game.
func (res *Resolver) Resolve(model root.Entry, t mdx.Texture) Resolution {
	if model.ModChain == "" {
		return Resolution{How: Unresolved}
	}

	name := t.Path
	if name == "" {
		if t.ReplaceableID != 1 {
			return Resolution{How: Unresolved}
		}
		name = teamColorPath
	} else {
		name = strings.ReplaceAll(name, `\`, "/")
		ext := path.Ext(name)
		name = name[:len(name)-len(ext)] + ".dds"
	}

	if entry, ok := res.resolveChain(model.ModChain, name); ok {
		return Resolution{Entry: entry, How: OwnChain}
	}
	if t.Path == "" {
		return Resolution{How: Unresolved}
	}

	for _, chain := range otherEditionChains[model.Variant.Edition] {
		if entry, ok := res.resolveChain(chain, name); ok {
			return Resolution{Entry: entry, How: OtherEdition}
		}
	}

	return res.resolveByFileName(model.Variant.Edition, name)
}

// resolveChain looks up name in chain, then each parent chain down to
// "War3.w3mod:".
func (res *Resolver) resolveChain(chain, name string) (root.Entry, bool) {
	for ; ; chain = parentChain(chain) {
		if idx, ok := res.byFold[strings.ToLower(chain+name)]; ok {
			return res.root.Entries[idx], true
		}
		if chain == "War3.w3mod:" {
			return root.Entry{}, false
		}
	}
}

// resolveByFileName tries name's base alone: Missing with no candidate,
// ByFileName with exactly one, or exactly one whose edition is edition,
// else Ambiguous.
func (res *Resolver) resolveByFileName(edition root.Edition, name string) Resolution {
	rawBase := path.Base(name)
	base := strings.ToLower(rawBase)
	candidates := res.byBase[base]
	if len(candidates) == 0 {
		return Resolution{How: Missing, Base: rawBase}
	}
	if len(candidates) == 1 {
		return Resolution{Entry: res.root.Entries[candidates[0]], How: ByFileName}
	}

	ownEdition := -1
	for _, idx := range candidates {
		if res.root.Entries[idx].Variant.Edition == edition {
			if ownEdition >= 0 {
				return Resolution{How: Ambiguous, Candidates: len(candidates), Base: rawBase}
			}
			ownEdition = idx
		}
	}
	if ownEdition >= 0 {
		return Resolution{Entry: res.root.Entries[ownEdition], How: ByFileName}
	}
	return Resolution{How: Ambiguous, Candidates: len(candidates), Base: rawBase}
}

// parentChain drops the last ':'-terminated segment of chain, e.g.
// "War3.w3mod:_HD.w3mod:_Teen.w3mod:" -> "War3.w3mod:_HD.w3mod:".
func parentChain(chain string) string {
	trimmed := strings.TrimSuffix(chain, ":")
	idx := strings.LastIndexByte(trimmed, ':')
	if idx < 0 {
		return "War3.w3mod:" // every chain starts with War3.w3mod
	}
	return trimmed[:idx+1]
}
