package main

import (
	"bufio"
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
	"modelView/viewer"
	"os"
	"sort"
)

// out is where startup output goes; stdout by default, stderr with -list so
// that -list output on stdout can be piped cleanly.
var out io.Writer = os.Stdout

func main() {
	view := flag.Bool("view", false, "open the model viewer window")
	verify := flag.Bool("verify", true, "cross-check idx entries against archive headers")
	debug := flag.Bool("debug", false, "print debug output from the extract packages")
	list := flag.Bool("list", false, "list every root file: path, size, content key (tab-separated); other output goes to stderr")
	flag.Parse()

	if *list {
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

	if *list {
		listFiles(storage, r)
	}

	if *view {
		runViewer()
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

// listFiles prints one line per root entry to stdout: path, size, content
// key, tab-separated.
func listFiles(storage *casc.Storage, r root.Root) {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	for _, entry := range r.Entries {
		encEntry, exists := storage.LookupCKey(entry.CKey)
		if !exists {
			panic(fmt.Sprintf("Content key %x (%s) not found in encoding file", entry.CKey, entry.Path))
		}

		fmt.Fprintf(w, "%s\t%d\t%x\n", entry.Path, encEntry.Size, entry.CKey)
	}
}

func decodeKey(key string) []byte {
	decoded, err := hex.DecodeString(key)
	if err != nil {
		panic(err)
	}
	return decoded
}

func runViewer() {
	if err := viewer.Run(viewer.DefaultConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "viewer:", err)
		os.Exit(1)
	}
}
