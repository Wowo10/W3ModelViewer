package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"modelView/extract/blte"
	"modelView/extract/buildConfig"
	"modelView/extract/buildinfo"
	"modelView/extract/encoding"
	"modelView/extract/idxPackages"
	"modelView/viewer"
	"os"
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

	packages := idxPackages.ReadIdxPackages(*verify)

	encodingEntry := findEncodingPackage(packages, ekey)
	enc := readEncoding(encodingEntry, ckey, *verify)

	findRootPackage(packages, enc, config.GetRoot())

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
func findEncodingPackage(packages map[[9]byte]idxPackages.IndexEntry, ekey string) idxPackages.IndexEntry {
	ekeyDecoded, err := hex.DecodeString(ekey)
	if err != nil {
		panic(err)
	}
	key := [9]byte(ekeyDecoded[:9])

	packageEntry, exists := packages[key]
	if !exists {
		panic(fmt.Sprintf("Encoding key %x not found in index packages", key))
	}

	fmt.Printf("Encoding key %x found in archive %d at offset %d with size %d\n", key, packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

// readEncoding decodes the encoding file. With verify set, its MD5 must match the content key.
func readEncoding(entry idxPackages.IndexEntry, ckey string, verify bool) encoding.Encoding {
	data := blte.Decode(idxPackages.ReadEntry(entry), verify)

	if verify {
		sum := md5.Sum(data)
		if hex.EncodeToString(sum[:]) != ckey {
			panic(fmt.Sprintf("Encoding MD5 mismatch: got %x, want %s", sum, ckey))
		}
	}

	enc := encoding.ReadEncoding(data)
	fmt.Println("Encoding entries:", len(enc))

	return enc
}

// findRootPackage resolves the root content key through the encoding file to its idx entry.
func findRootPackage(packages map[[9]byte]idxPackages.IndexEntry, enc encoding.Encoding, ckey string) idxPackages.IndexEntry {
	ckeyDecoded, err := hex.DecodeString(ckey)
	if err != nil {
		panic(err)
	}

	encEntry, exists := enc.Lookup([16]byte(ckeyDecoded))
	if !exists {
		panic(fmt.Sprintf("Root key %s not found in encoding file", ckey))
	}

	ekey := encEntry.EKeys[0]
	fmt.Printf("Root key %s has encoding key %x, size %d\n", ckey, ekey, encEntry.Size)

	key := [9]byte(ekey[:9])
	packageEntry, exists := packages[key]
	if !exists {
		panic(fmt.Sprintf("Root encoding key %x not found in index packages", key))
	}

	fmt.Printf("Root key %x found in archive %d at offset %d with size %d\n", key, packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

func runViewer() {
	if err := viewer.Run(viewer.DefaultConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "viewer:", err)
		os.Exit(1)
	}
}
