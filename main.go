package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"modelView/extract/buildConfig"
	"modelView/extract/buildinfo"
	"modelView/extract/idxPackages"
	"modelView/viewer"
	"os"
)

func main() {
	view := flag.Bool("view", false, "open the model viewer window")
	verify := flag.Bool("verify", true, "cross-check idx entries against archive headers")
	flag.Parse()

	bInfo := readBuildInfo()

	config := readBuildConfig(bInfo.GetBuildKey())
	_, ekey := config.GetEncoding()

	findEncodingPackage(ekey, *verify)

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
func findEncodingPackage(ekey string, verify bool) idxPackages.IndexEntry {
	ekeyDecoded, err := hex.DecodeString(ekey)
	if err != nil {
		panic(err)
	}
	key := [9]byte(ekeyDecoded[:9])

	packages := idxPackages.ReadIdxPackages(verify)

	packageEntry, exists := packages[key]
	if !exists {
		panic(fmt.Sprintf("Encoding key %x not found in index packages", key))
	}

	fmt.Printf("Encoding key %x found in archive %d at offset %d with size %d\n", key, packageEntry.Archive, packageEntry.Offset, packageEntry.Size)

	return packageEntry
}

func runViewer() {
	if err := viewer.Run(viewer.DefaultConfig()); err != nil {
		fmt.Fprintln(os.Stderr, "viewer:", err)
		os.Exit(1)
	}
}
