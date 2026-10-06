package main

import (
	"encoding/hex"
	"flag"
	"fmt"
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

func main() {
	view := flag.Bool("view", false, "open the model viewer window")
	verify := flag.Bool("verify", true, "cross-check idx entries against archive headers")
	debug := flag.Bool("debug", false, "print debug output from the extract packages")
	flag.Parse()

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
	readRoot(storage, config.GetRoot())

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
	fmt.Println("Build Key:", buildInfo.GetBuildKey())
	fmt.Println("CDN Key:", buildInfo.GetCDNKey())

	return buildInfo
}

func readBuildConfig(filename string) buildConfig.BuildConfig {
	data, err := os.ReadFile(configPath(filename))
	if err != nil {
		panic(err)
	}

	config := buildConfig.ReadBuildConfig(data)
	cKey, eKey := config.GetEncoding()
	fmt.Println("Client Key:", cKey)
	fmt.Println("Server Key:", eKey)

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

	fmt.Printf("Encoding key %x found in archive %d at offset %d with size %d\n", ekeyDecoded[:9], packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

// readEncoding decodes the encoding file. With verify set, its MD5 must match the content key.
func readEncoding(storage *casc.Storage, entry idxPackages.IndexEntry, ckey string) {
	enc := storage.LoadEncoding(entry, [16]byte(decodeKey(ckey)))
	fmt.Println("Encoding entries:", len(enc))
}

// findRootPackage resolves the root content key through the encoding file to its idx entry.
func findRootPackage(storage *casc.Storage, ckey string) idxPackages.IndexEntry {
	encEntry, exists := storage.LookupCKey([16]byte(decodeKey(ckey)))
	if !exists {
		panic(fmt.Sprintf("Root key %s not found in encoding file", ckey))
	}

	ekey := encEntry.EKeys[0]
	fmt.Printf("Root key %s has encoding key %x, size %d\n", ckey, ekey, encEntry.Size)

	packageEntry, exists := storage.LookupEKey(ekey[:])
	if !exists {
		panic(fmt.Sprintf("Root encoding key %x not found in index packages", ekey[:9]))
	}

	fmt.Printf("Root key %x found in archive %d at offset %d with size %d\n", ekey[:9], packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

// readRoot reads and parses the root file, and prints the entry count and the
// count per mod chain, largest first.
func readRoot(storage *casc.Storage, ckey string) root.Root {
	r := root.ReadRoot(storage.ReadByCKey([16]byte(decodeKey(ckey))))

	fmt.Println("Root entries:", len(r.Entries))

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

	fmt.Println("Root entries per mod chain:")
	for _, chain := range chains {
		name := chain
		if name == "" {
			name = "(none)"
		}
		fmt.Printf("  %5d %s\n", counts[chain], name)
	}

	return r
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
