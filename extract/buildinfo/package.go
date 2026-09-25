package buildinfo

import "strings"

type BuildInfo map[string]string

const (
	buildKey = "Build Key!HEX:16"
	cdnKey   = "CDN Key!HEX:16"
)

func ReadBuildInfo(data []byte) BuildInfo {
	content := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	lines := strings.Split(content, "\n")
	if len(lines) != 2 {
		return BuildInfo{}
	}

	info := make(BuildInfo)

	headers := strings.Split(lines[0], "|")
	contents := strings.Split(lines[1], "|")
	for i, header := range headers {
		info[header] = contents[i]
	}

	return info
}

func (b BuildInfo) GetBuildKey() string {
	return b[buildKey]
}

func (b BuildInfo) GetCDNKey() string {
	return b[cdnKey]
}
