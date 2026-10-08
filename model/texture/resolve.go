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

// Resolver maps an MDX texture reference to the root entry holding its
// stored .dds, by the rule in roadmap/04-tex/EPIC.md: swap backslashes for
// '/' and the extension for .dds, then look up case-insensitively in the
// model's own mod chain, then each parent chain (the chain with its last
// segment repeatedly dropped) down to "War3.w3mod:".
type Resolver struct {
	root   root.Root
	byFold map[string]int // lower(entry.Path) -> index into root.Entries
}

// NewResolver indexes every root entry's path folded to lower case, for
// case-insensitive lookup (850 of wardata's resolutions only match this
// way). On a fold collision the first entry wins; none are seen in
// wardata, and this never panics since a future build adding one shouldn't
// break startup for callers that build a Resolver there.
func NewResolver(r root.Root) *Resolver {
	byFold := make(map[string]int, len(r.Entries))
	for i, entry := range r.Entries {
		fold := strings.ToLower(entry.Path)
		if _, exists := byFold[fold]; !exists {
			byFold[fold] = i
		}
	}
	return &Resolver{root: r, byFold: byFold}
}

// Resolve finds the root entry for t's reference from model (the entry for
// the .mdx that references t). It returns false when t is a replaceable
// texture other than team colour (id 1), model has no mod chain (an
// install file), or no chain in model's parent chain down to "War3.w3mod:"
// holds the resolved name.
func (res *Resolver) Resolve(model root.Entry, t mdx.Texture) (root.Entry, bool) {
	if model.ModChain == "" {
		return root.Entry{}, false
	}

	name := t.Path
	if name == "" {
		if t.ReplaceableID != 1 {
			return root.Entry{}, false
		}
		name = teamColorPath
	} else {
		name = strings.ReplaceAll(name, `\`, "/")
		ext := path.Ext(name)
		name = name[:len(name)-len(ext)] + ".dds"
	}

	for chain := model.ModChain; ; chain = parentChain(chain) {
		if idx, ok := res.byFold[strings.ToLower(chain+name)]; ok {
			return res.root.Entries[idx], true
		}
		if chain == "War3.w3mod:" {
			return root.Entry{}, false
		}
	}
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
