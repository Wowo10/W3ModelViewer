package settings

import (
	"fmt"
	"modelView/extract/buildinfo"
	"modelView/extract/idxPackages"
	"os"
	"path/filepath"
)

// CheckInstall reports why dir isn't a usable Warcraft III install, or nil
// when it is: dir exists and is a directory, has a .build.info with a
// 32-char hex build key, that build's config under Data/config and every
// idx file idxPackages reads under Data/data.
func CheckInstall(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s does not exist", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	biPath := filepath.Join(dir, ".build.info")
	data, err := os.ReadFile(biPath)
	if err != nil {
		return fmt.Errorf("no .build.info")
	}

	key, err := readBuildKey(data)
	if err != nil {
		return fmt.Errorf(".build.info has no build key")
	}

	cfgPath := filepath.Join(dir, "Data", "config", key[0:2], key[2:4], key)
	if _, err := os.Stat(cfgPath); err != nil {
		return fmt.Errorf("no build config Data/config/%s/%s/%s", key[0:2], key[2:4], key)
	}

	dataDir := filepath.Join(dir, "Data", "data")
	for _, name := range idxPackages.Filenames() {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			return fmt.Errorf("no Data/data/%s", name)
		}
	}

	return nil
}

// readBuildKey parses data as a .build.info file and returns its build key,
// recovering from the panics buildinfo.ReadBuildInfo raises on malformed
// input. It requires a 32-char hex key.
func readBuildKey(data []byte) (key string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("malformed .build.info: %v", r)
		}
	}()

	info := buildinfo.ReadBuildInfo(data)
	key = info.GetBuildKey()
	if len(key) != 32 {
		return "", fmt.Errorf("build key %q is not 32 hex characters", key)
	}
	for _, c := range key {
		if !isHex(c) {
			return "", fmt.Errorf("build key %q is not hex", key)
		}
	}

	return key, nil
}

func isHex(c rune) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
