package buildConfig

import "strings"

type BuildConfig map[string]string

const (
	encoding = "encoding"
)

func ReadBuildConfig(data []byte) BuildConfig {
	content := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	lines := strings.Split(content, "\n")

	config := make(BuildConfig)

	for _, line := range lines {
		key, value, ok := parseLine(line)
		if ok {
			config[key] = value
		}
	}

	return config
}

// parseLine splits a "key = value" line; ok is false for lines without a key.
func parseLine(line string) (key, value string, ok bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])

	return key, value, key != ""
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
