package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"modelView/extract/blte"
	"modelView/extract/buildConfig"
	"modelView/extract/buildinfo"
	"modelView/extract/casc"
	"modelView/extract/encoding"
	"modelView/extract/idxPackages"
	"modelView/extract/root"
	"modelView/model/mdx"
	"modelView/model/texture"
	"modelView/viewer"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// out is where -extract reports; stdout, or stderr when -list, -variants or
// -mdxinfo own stdout.
var out io.Writer = os.Stdout

func main() {
	view := flag.Bool("view", false, "open the model viewer window")
	verify := flag.Bool("verify", true, "cross-check idx entries against archive headers")
	debug := flag.Bool("debug", false, "print debug output from the extract packages")
	list := flag.Bool("list", false, "list every root file: path, size, content key, edition, overlays (tab-separated); other output goes to stderr")
	filter := flag.String("filter", "", "with -list, only list files whose path inside the mod chain matches this glob; * and ? also match /")
	edition := flag.String("edition", "", "with -list, only list files of this edition: classic, reforged, definitive or install")
	variants := flag.String("variants", "", "print every edition/overlay version of this path without its mod chain (same format as -list); other output goes to stderr")
	extract := flag.String("extract", "", "extract the root file at this full path (including mod chain) to -out")
	outDir := flag.String("out", ".", "directory to extract into, with -extract")
	mdxinfo := flag.String("mdxinfo", "", "print the MDX version, chunk list (tag, size), textures (each with its resolved .dds path and format, or \"unresolved\"), materials, sequences, default sequence, geoset vertex, triangle and alpha counts and a skinning summary of the model at this full path (including mod chain), with per-LOD totals when a model has several LODs; other output goes to stderr")
	survey := flag.Bool("survey", false, "read every distinct content key in the root once and print ok/missing/failed totals to stdout")
	flag.Parse()

	if *list || *variants != "" || *mdxinfo != "" {
		out = os.Stderr
	}

	idxPackages.DEBUG = *debug
	blte.DEBUG = *debug
	encoding.DEBUG = *debug

	bInfo := readBuildInfo()

	config := readBuildConfig(bInfo.GetBuildKey())
	ckey, ekey := config.GetEncoding()

	storage := casc.Open(*verify)
	defer storage.Close()

	encodingEntry := findEncodingPackage(storage, ekey)
	readEncoding(storage, encodingEntry, ckey)

	findRootPackage(storage, config.GetRoot())
	r := readRoot(storage, config.GetRoot())

	if *extract != "" {
		extractFile(storage, r, *extract, *outDir)
	}

	if *list {
		listFiles(storage, r, *filter, *edition)
	}

	if *variants != "" {
		printVariants(storage, r, *variants)
	}

	if *mdxinfo != "" {
		printMDXInfo(storage, r, *mdxinfo)
	}

	if *survey {
		surveyFiles(storage, r)
	}

	if *view {
		runViewer(storage, r)
	}
}

func readBuildInfo() buildinfo.BuildInfo {
	data, err := os.ReadFile("wardata/.build.info")
	if err != nil {
		panic(err)
	}

	buildInfo := buildinfo.ReadBuildInfo(data)

	return buildInfo
}

func readBuildConfig(filename string) buildConfig.BuildConfig {
	data, err := os.ReadFile(configPath(filename))
	if err != nil {
		panic(err)
	}

	config := buildConfig.ReadBuildConfig(data)

	return config
}

// configPath maps a config hash to its CDN-style location, e.g. ab/cd/abcd....
func configPath(filename string) string {
	return "wardata/Data/config/" + filename[0:2] + "/" + filename[2:4] + "/" + filename
}

// findEncodingPackage looks up the encoding file's location in the local idx packages.
func findEncodingPackage(storage *casc.Storage, ekey string) idxPackages.IndexEntry {
	ekeyDecoded := decodeKey(ekey)

	packageEntry, exists := storage.LookupEKey(ekeyDecoded)
	if !exists {
		panic(fmt.Sprintf("Encoding key %x not found in index packages", ekeyDecoded[:9]))
	}

	return packageEntry
}

// readEncoding decodes the encoding file. With verify set, its MD5 must match the content key.
func readEncoding(storage *casc.Storage, entry idxPackages.IndexEntry, ckey string) {
	storage.LoadEncoding(entry, [16]byte(decodeKey(ckey)))
}

// findRootPackage resolves the root content key through the encoding file to its idx entry.
func findRootPackage(storage *casc.Storage, ckey string) idxPackages.IndexEntry {
	encEntry, exists := storage.LookupCKey([16]byte(decodeKey(ckey)))
	if !exists {
		panic(fmt.Sprintf("Root key %s not found in encoding file", ckey))
	}

	ekey := encEntry.EKeys[0]

	packageEntry, exists := storage.LookupEKey(ekey[:])
	if !exists {
		panic(fmt.Sprintf("Root encoding key %x not found in index packages", ekey[:9]))
	}

	return packageEntry
}

// readRoot reads and parses the root file.
func readRoot(storage *casc.Storage, ckey string) root.Root {
	return root.ReadRoot(storage.ReadByCKey([16]byte(decodeKey(ckey))))
}

// listFiles prints one line per root entry to stdout via writeEntry. With
// glob set, only entries whose Name (the path inside the mod chain) matches
// it are printed; with edition set, only entries of that edition (ANDed with
// glob). Overlays are not filtered: -edition reforged includes _HD:_Teen and
// _HD:_Locales/* entries.
func listFiles(storage *casc.Storage, r root.Root, glob, edition string) {
	var re *regexp.Regexp
	if glob != "" {
		re = globRegexp(glob)
	}

	if edition != "" {
		validateEdition(edition)
	}

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	for _, entry := range r.Entries {
		if re != nil && !re.MatchString(entry.Name) {
			continue
		}
		if edition != "" && string(entry.Variant.Edition) != edition {
			continue
		}

		writeEntry(w, storage, entry)
	}
}

// printVariants prints every entry whose Name matches name (case-
// insensitively) via writeEntry, sorted by edition (classic, reforged,
// definitive, install), then overlays, then path. It panics if name matches
// nothing, like extractFile.
func printVariants(storage *casc.Storage, r root.Root, name string) {
	entries := r.Variants(name)
	if len(entries) == 0 {
		panic(fmt.Sprintf("No variants found for %q", name))
	}

	root.SortVariants(entries)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	for _, entry := range entries {
		writeEntry(w, storage, entry)
	}
}

// validateEdition panics with the valid names if edition isn't one of them.
func validateEdition(edition string) {
	for _, e := range root.Editions {
		if string(e) == edition {
			return
		}
	}
	panic(fmt.Sprintf("Unknown edition %q, want one of %v", edition, root.Editions))
}

// writeEntry prints one root entry: path, size, content key, edition,
// overlays, tab-separated.
func writeEntry(w io.Writer, storage *casc.Storage, entry root.Entry) {
	encEntry, exists := storage.LookupCKey(entry.CKey)
	if !exists {
		panic(fmt.Sprintf("Content key %x (%s) not found in encoding file", entry.CKey, entry.Path))
	}

	fmt.Fprintf(w, "%s\t%d\t%x\t%s\t%s\n", entry.Path, encEntry.Size, entry.CKey, entry.Variant.Edition, entry.Variant.OverlayString())
}

// readFile looks up the root file at path and reads its decoded bytes.
func readFile(storage *casc.Storage, r root.Root, path string) (root.Entry, []byte) {
	entry, exists := r.Lookup(path)
	if !exists {
		panic(fmt.Sprintf("Path %q not found in root", path))
	}

	encEntry, exists := storage.LookupCKey(entry.CKey)
	if !exists {
		panic(fmt.Sprintf("Content key %x not found in encoding file", entry.CKey))
	}
	ekey := encEntry.EKeys[0]
	if _, exists := storage.LookupEKey(ekey[:]); !exists {
		panic(fmt.Sprintf("%s (content key %x) is not stored locally: encoding key %x not in any idx", path, entry.CKey, ekey))
	}

	return entry, storage.ReadByCKey(entry.CKey)
}

// extractFile reads the root file at path and writes its decoded bytes to
// dir, under its name inside the mod chain. It reports when the data starts
// with MDLX (a model).
func extractFile(storage *casc.Storage, r root.Root, path, dir string) {
	entry, data := readFile(storage, r, path)

	dest := filepath.Join(dir, filepath.FromSlash(entry.Name))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		panic(err)
	}

	model := ""
	if bytes.HasPrefix(data, []byte("MDLX")) {
		model = ", model"
	}
	fmt.Fprintf(out, "Extracted %s (%d bytes%s) to %s\n", path, len(data), model, dest)
}

// printMDXInfo reads the MDX model at path and prints its version, chunk
// list (tag, size, in file order), textures (with each one's resolved .dds
// path and format, or "unresolved"), materials, sequences, default sequence,
// geoset vertex/triangle/alpha counts and a skinning summary to stdout, with
// per-LOD totals when a model has several LODs.
func printMDXInfo(storage *casc.Storage, r root.Root, path string) {
	entry, data := readFile(storage, r, path)
	model := mdx.Read(data)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	fmt.Fprintln(w, path)
	fmt.Fprintln(w, "Version:", model.Version)
	fmt.Fprintln(w, "Chunks:", len(model.Chunks))
	for _, chunk := range model.Chunks {
		fmt.Fprintf(w, "  %-4s %9d\n", chunk.Tag, len(chunk.Data))
	}

	if textures := model.Textures(); textures != nil {
		resolver := texture.NewResolver(r)

		fmt.Fprintln(w, "Textures:", len(textures))
		fmt.Fprintf(w, "  %7s %11s %5s path\n", "texture", "replaceable", "flags")
		for i, t := range textures {
			line := fmt.Sprintf("  %7d %11d %5d", i, t.ReplaceableID, t.Flags)
			if t.Path != "" {
				line += " " + t.Path
			}
			fmt.Fprintln(w, line)

			resolved, ok := resolver.Resolve(entry, t)
			if !ok {
				fmt.Fprintln(w, strings.Repeat(" ", 28)+"-> unresolved")
				continue
			}
			dds := texture.ReadDDS(storage.ReadByCKey(resolved.CKey))
			fmt.Fprintf(w, "%s-> %s %s %dx%d %d mips\n", strings.Repeat(" ", 28), resolved.Path, dds.Format, dds.Width, dds.Height, len(dds.Mips))
		}
	}

	if materials := model.Materials(); materials != nil {
		fmt.Fprintln(w, "Materials:", len(materials))
		fmt.Fprintf(w, "  %8s %5s %-14s%7s  %5s %5s %s\n", "material", "layer", "filter", "shading", "alpha", "coord", "textures")
		for mi, mat := range materials {
			for li, l := range mat.Layers {
				var texIDs []string
				for ti, t := range l.Textures {
					if t.Slot == uint32(ti) {
						texIDs = append(texIDs, fmt.Sprintf("%d", t.ID))
					} else {
						texIDs = append(texIDs, fmt.Sprintf("%d/%d", t.ID, t.Slot))
					}
				}
				shading := fmt.Sprintf("0x%x", l.ShadingFlags)
				fmt.Fprintf(w, "  %8d %5d %-14s%7s  %5.2f %5d %s\n", mi, li, l.FilterMode, shading, l.Alpha, l.CoordID, strings.Join(texIDs, " "))
			}
		}
	}

	geosets := model.Geosets()
	if len(geosets) == 0 {
		fmt.Fprintln(w, "Geosets:", len(geosets))
		return
	}

	seqs := model.Sequences()
	fmt.Fprintln(w, "Sequences:", len(seqs))
	for _, s := range seqs {
		fmt.Fprintf(w, "  %-32s %6d %6d\n", s.Name, s.Start, s.End)
	}
	def, ok := mdx.DefaultSequence(seqs)
	if ok {
		fmt.Fprintf(w, "Default sequence: %s (%d-%d)\n", def.Name, def.Start, def.End)
	} else {
		fmt.Fprintln(w, "Default sequence: none (all geosets visible)")
	}

	if globals := model.GlobalSequences(); globals != nil {
		lens := make([]string, len(globals))
		for i, l := range globals {
			lens[i] = fmt.Sprintf("%d", l)
		}
		fmt.Fprintf(w, "Global sequences: %d (len %s)\n", len(globals), strings.Join(lens, " "))
	} else {
		fmt.Fprintln(w, "Global sequences: 0")
	}
	fmt.Fprintln(w, "Pivots:", len(model.Pivots()))

	bones, helpers := model.Bones(), model.Helpers()
	fmt.Fprintf(w, "Nodes: %d bones, %d helpers\n", len(bones), len(helpers))
	fmt.Fprintf(w, "  %-11s %6s %6s %5s %6s %7s %6s %6s\n", "track", "tracks", "keys", "none", "linear", "hermite", "bezier", "global")
	printTrackCounts(w, "translation", collectTrackInterps(bones, helpers, func(n mdx.Node) *mdx.Track[[3]float32] { return n.Translation }))
	printTrackCounts(w, "rotation", collectTrackInterps(bones, helpers, func(n mdx.Node) *mdx.Track[[4]float32] { return n.Rotation }))
	printTrackCounts(w, "scale", collectTrackInterps(bones, helpers, func(n mdx.Node) *mdx.Track[[3]float32] { return n.Scale }))

	fmt.Fprintln(w, "Geosets:", len(geosets))

	alphas := mdx.GeosetAlphas(len(geosets), model.GeosetAnims(), def, ok)

	fmt.Fprintf(w, "  %6s %8s %3s %9s %9s %5s\n", "geoset", "material", "lod", "vertices", "triangles", "alpha")
	var totalVertices, totalTriangles, hidden int
	lodVertices := map[uint32]int{}
	lodTriangles := map[uint32]int{}
	for i, g := range geosets {
		fmt.Fprintf(w, "  %6d %8d %3d %9d %9d %5.2f\n", i, g.MaterialID, g.LOD, g.VertexCount(), g.TriangleCount(), alphas[i])
		totalVertices += g.VertexCount()
		totalTriangles += g.TriangleCount()
		lodVertices[g.LOD] += g.VertexCount()
		lodTriangles[g.LOD] += g.TriangleCount()
		if alphas[i] <= 0 {
			hidden++
		}
	}
	fmt.Fprintf(w, "  %-19s %9d %9d\n", "total", totalVertices, totalTriangles)

	if len(lodVertices) > 1 {
		lods := make([]uint32, 0, len(lodVertices))
		for lod := range lodVertices {
			lods = append(lods, lod)
		}
		sort.Slice(lods, func(i, j int) bool { return lods[i] < lods[j] })
		for _, lod := range lods {
			fmt.Fprintf(w, "  %-19s %9d %9d\n", fmt.Sprintf("lod %d total", lod), lodVertices[lod], lodTriangles[lod])
		}
	}

	if ok {
		fmt.Fprintf(w, "hidden in %s: %d of %d\n", def.Name, hidden, len(geosets))
	}

	var skinGeosets, matrixGroupGeosets int
	influencesPerVertex := map[int]int{}
	for _, g := range geosets {
		if g.Skin != nil {
			skinGeosets++
		} else {
			matrixGroupGeosets++
		}
		influences := g.Influences()
		for v := 0; v < g.VertexCount(); v++ {
			used := 0
			for i := 0; i < mdx.MaxInfluences; i++ {
				if influences.Weights[v*mdx.MaxInfluences+i] > 0 {
					used++
				}
			}
			influencesPerVertex[used]++
		}
	}
	counts := make([]int, 0, len(influencesPerVertex))
	for n := range influencesPerVertex {
		counts = append(counts, n)
	}
	sort.Ints(counts)
	parts := make([]string, 0, len(counts))
	for _, n := range counts {
		parts = append(parts, fmt.Sprintf("%d:%d", n, influencesPerVertex[n]))
	}
	fmt.Fprintf(w, "Skinning: %d SKIN geosets, %d matrix-group geosets, influences per vertex %s\n",
		skinGeosets, matrixGroupGeosets, strings.Join(parts, " "))
}

// trackCounts tallies one kind of track (translation, rotation or scale)
// across a model's bones and helpers, for printMDXInfo.
type trackCounts struct {
	tracks, keys                          int
	none, linear, hermite, bezier, global int
}

// collectTrackInterps tallies the tracks returned by get over every bone
// and helper node.
func collectTrackInterps[T any](bones []mdx.Bone, helpers []mdx.Node, get func(mdx.Node) *mdx.Track[T]) trackCounts {
	var c trackCounts
	add := func(n mdx.Node) {
		t := get(n)
		if t == nil {
			return
		}
		c.tracks++
		c.keys += len(t.Keys)
		switch t.Interpolation {
		case mdx.InterpNone:
			c.none++
		case mdx.InterpLinear:
			c.linear++
		case mdx.InterpHermite:
			c.hermite++
		case mdx.InterpBezier:
			c.bezier++
		}
		if t.GlobalSequence >= 0 {
			c.global++
		}
	}
	for _, b := range bones {
		add(b.Node)
	}
	for _, h := range helpers {
		add(h)
	}
	return c
}

// printTrackCounts prints one row of the node track table in printMDXInfo.
func printTrackCounts(w io.Writer, name string, c trackCounts) {
	fmt.Fprintf(w, "  %-11s %6d %6d %5d %6d %7d %6d %6d\n", name, c.tracks, c.keys, c.none, c.linear, c.hermite, c.bezier, c.global)
}

// globRegexp compiles a shell-style glob into an anchored regexp, where *
// matches any run of characters (including /) and ? matches any one
// character.
func globRegexp(glob string) *regexp.Regexp {
	pattern := regexp.QuoteMeta(glob)
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	pattern = strings.ReplaceAll(pattern, `\?`, ".")
	return regexp.MustCompile("^" + pattern + "$")
}

func decodeKey(key string) []byte {
	decoded, err := hex.DecodeString(key)
	if err != nil {
		panic(err)
	}
	return decoded
}

func runViewer(storage *casc.Storage, r root.Root) {
	if err := viewer.Run(viewer.DefaultConfig(), storage, r); err != nil {
		fmt.Fprintln(os.Stderr, "viewer:", err)
		os.Exit(1)
	}
}
