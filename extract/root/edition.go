package root

import "strings"

// Edition is the top-level graphics layer a file belongs to.
type Edition string

const (
	Classic    Edition = "classic"
	Reforged   Edition = "reforged"
	Definitive Edition = "definitive"
	Install    Edition = "install"
)

// Editions lists every valid Edition value, for validating -edition.
var Editions = []Edition{Classic, Reforged, Definitive, Install}

// Variant is a root entry's edition and the overlays stacked on it (e.g.
// teen, a locale, a tileset), derived from its mod chain.
type Variant struct {
	Edition  Edition
	Overlays []string
}

// OverlayString joins Overlays with commas, for -list's overlays column.
func (v Variant) OverlayString() string {
	return strings.Join(v.Overlays, ",")
}

// ParseModChain derives a Variant from a root entry's mod chain (e.g.
// "War3.w3mod:_HD.w3mod:"), based on findings against wardata (see this
// task's roadmap notes): chain segments after "War3.w3mod:" are "_HD.w3mod"
// (reforged), "_DE.w3mod" (definitive, with a non-empty directory also an
// overlay "map:<dir>"), "_Teen.w3mod" (overlay "teen"), "_Deprecated.w3mod"
// (overlay "deprecated"), "_Locales/<locale>.w3mod" (overlay
// "locale:<locale>"), "_Tilesets/<letter>.w3mod" (overlay "tileset:<letter>")
// and "_Balance/<name>.w3mod" (overlay "balance:<name>"). An empty chain is
// an install file (outside the archive). Any other segment (none seen in
// today's data) is kept verbatim as an overlay rather than panicking, since
// ReadRoot runs on every start and a future build adding a mod should not
// break the default output. "classic" is a label for the base layer, not a
// claim that every file in it is SD art: the base layer also holds data
// shared by every edition (sounds, scripts, maps).
func ParseModChain(chain string) Variant {
	if chain == "" {
		return Variant{Edition: Install}
	}

	v := Variant{Edition: Classic}

	segments := strings.Split(strings.TrimSuffix(chain, ":"), ":")
	for _, segment := range segments[1:] { // segments[0] is always "War3.w3mod"
		dir, base := splitSegment(strings.TrimSuffix(segment, ".w3mod"))

		switch {
		case base == "_HD":
			v.Edition = Reforged
		case base == "_DE":
			v.Edition = Definitive
			if dir != "" {
				v.Overlays = append(v.Overlays, "map:"+dir)
			}
		case base == "_Teen":
			v.Overlays = append(v.Overlays, "teen")
		case base == "_Deprecated":
			v.Overlays = append(v.Overlays, "deprecated")
		case dir == "_Locales":
			v.Overlays = append(v.Overlays, "locale:"+base)
		case dir == "_Tilesets":
			v.Overlays = append(v.Overlays, "tileset:"+base)
		case dir == "_Balance":
			v.Overlays = append(v.Overlays, "balance:"+base)
		default:
			v.Overlays = append(v.Overlays, segment)
		}
	}

	return v
}

// splitSegment splits a mod chain segment at its last '/' into dir and base,
// mirroring parseLine's split of path into modChain and name.
func splitSegment(segment string) (dir, base string) {
	if idx := strings.LastIndexByte(segment, '/'); idx >= 0 {
		return segment[:idx], segment[idx+1:]
	}
	return "", segment
}
