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
	"modelView/viewer"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// out is where startup output goes; stdout by default, stderr with -list so
// that -list output on stdout can be piped cleanly.
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
	mdxinfo := flag.String("mdxinfo", "", "print the MDX version, chunk list (tag, size), sequences, default sequence and geoset vertex, triangle and alpha counts of the model at this full path (including mod chain), with per-LOD totals when a model has several LODs; other output goes to stderr")
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
	// fmt.Println(buildInfo)
	fmt.Fprintln(out, "Build Key:", buildInfo.GetBuildKey())
	fmt.Fprintln(out, "CDN Key:", buildInfo.GetCDNKey())

	return buildInfo
}

func readBuildConfig(filename string) buildConfig.BuildConfig {
	data, err := os.ReadFile(configPath(filename))
	if err != nil {
		panic(err)
	}

	config := buildConfig.ReadBuildConfig(data)
	cKey, eKey := config.GetEncoding()
	fmt.Fprintln(out, "Client Key:", cKey)
	fmt.Fprintln(out, "Server Key:", eKey)

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

	fmt.Fprintf(out, "Encoding key %x found in archive %d at offset %d with size %d\n", ekeyDecoded[:9], packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

// readEncoding decodes the encoding file. With verify set, its MD5 must match the content key.
func readEncoding(storage *casc.Storage, entry idxPackages.IndexEntry, ckey string) {
	enc := storage.LoadEncoding(entry, [16]byte(decodeKey(ckey)))
	fmt.Fprintln(out, "Encoding entries:", len(enc))
}

// findRootPackage resolves the root content key through the encoding file to its idx entry.
func findRootPackage(storage *casc.Storage, ckey string) idxPackages.IndexEntry {
	encEntry, exists := storage.LookupCKey([16]byte(decodeKey(ckey)))
	if !exists {
		panic(fmt.Sprintf("Root key %s not found in encoding file", ckey))
	}

	ekey := encEntry.EKeys[0]
	fmt.Fprintf(out, "Root key %s has encoding key %x, size %d\n", ckey, ekey, encEntry.Size)

	packageEntry, exists := storage.LookupEKey(ekey[:])
	if !exists {
		panic(fmt.Sprintf("Root encoding key %x not found in index packages", ekey[:9]))
	}

	fmt.Fprintf(out, "Root key %x found in archive %d at offset %d with size %d\n", ekey[:9], packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

// readRoot reads and parses the root file, and prints the entry count and the
// count per mod chain, largest first.
func readRoot(storage *casc.Storage, ckey string) root.Root {
	r := root.ReadRoot(storage.ReadByCKey([16]byte(decodeKey(ckey))))

	fmt.Fprintln(out, "Root entries:", len(r.Entries))

	counts := make(map[string]int)
	for _, entry := range r.Entries {
		counts[entry.ModChain]++
	}

	chains := make([]string, 0, len(counts))
	for chain := range counts {
		chains = append(chains, chain)
	}
	sort.Slice(chains, func(i, j int) bool {
		if counts[chains[i]] != counts[chains[j]] {
			return counts[chains[i]] > counts[chains[j]]
		}
		return chains[i] < chains[j]
	})

	fmt.Fprintln(out, "Root entries per mod chain:")
	for _, chain := range chains {
		name := chain
		if name == "" {
			name = "(none)"
		}
		fmt.Fprintf(out, "  %5d %s\n", counts[chain], name)
	}

	return r
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
// list (tag, size, in file order), sequences, default sequence and geoset
// vertex/triangle/alpha counts to stdout, with per-LOD totals when a model
// has several LODs.
func printMDXInfo(storage *casc.Storage, r root.Root, path string) {
	_, data := readFile(storage, r, path)
	model := mdx.Read(data)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	fmt.Fprintln(w, path)
	fmt.Fprintln(w, "Version:", model.Version)
	fmt.Fprintln(w, "Chunks:", len(model.Chunks))
	for _, chunk := range model.Chunks {
		fmt.Fprintf(w, "  %-4s %9d\n", chunk.Tag, len(chunk.Data))
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
