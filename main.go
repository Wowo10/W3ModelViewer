package main

import (
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
	flag.Parse()

	bInfo := readBuildInfo()

	readBuildConfig(bInfo.GetBuildKey())

	idxPackages.ReadIdxPackages()

	if *view {
		if err := viewer.Run(viewer.DefaultConfig()); err != nil {
			fmt.Fprintln(os.Stderr, "viewer:", err)
			os.Exit(1)
		}
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
	data, err := os.ReadFile("wardata/Data/config/" + filename[0:2] + "/" + filename[2:4] + "/" + filename)
	if err != nil {
		panic(err)
	}

	buildConfig := buildConfig.ReadBuildConfig(data)
	cKey, eKey := buildConfig.GetEncoding()
	fmt.Println("Client Key:", cKey)
	fmt.Println("Server Key:", eKey)

	return buildConfig
}
