package buildConfig

import "strings"

type BuildConfig map[string]string

const (
	encoding = "encoding"
)

func ReadBuildConfig(data []byte) BuildConfig {
	content := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	lines := strings.Split(content, "\n")

	BuildConfig := make(BuildConfig)

	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key != "" {
			BuildConfig[key] = value
		}
	}

	return BuildConfig
}

func (b BuildConfig) GetEncoding() (cKey, eKey string) {
	val := b[encoding]

	parts := strings.Split(val, " ")
	if len(parts) == 2 {
		cKey = parts[0]
		eKey = parts[1]
	}

	return cKey, eKey
}
