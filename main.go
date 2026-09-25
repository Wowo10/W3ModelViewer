package main

import (
	"fmt"
	"modelView/extract/buildinfo"
	"os"
)

func main() {
	bInfo := readBuildInfo()

	bInfo.GetBuildKey()
}

func readBuildInfo() buildinfo.BuildInfo {
	data, err := os.ReadFile("wardata/.build.info")
	if err != nil {
		panic(err)
	}

	buildInfo := buildinfo.ReadBuildInfo(data)
	fmt.Println(buildInfo)
	fmt.Println("Build Key:", buildInfo.GetBuildKey())
	fmt.Println("CDN Key:", buildInfo.GetCDNKey())

	return buildInfo
}
