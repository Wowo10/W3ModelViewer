// Package settings holds modelViewer.json: the Warcraft III install path
// the desktop viewer reads the game from.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileName is the config file's name.
const FileName = "modelViewer.json"

// DefaultPort is the web viewer's port when modelViewer.json has none.
const DefaultPort = 8001

// Settings is the content of modelViewer.json.
type Settings struct {
	InstallPath string `json:"installPath"`
	Port        int    `json:"port"`
}

// Defaults returns the Settings of a file that has no fields yet.
func Defaults() Settings {
	return Settings{Port: DefaultPort}
}

// Path returns where modelViewer.json lives: next to the running
// executable, or in the current working directory when the executable is
// under the system temporary directory (as it is under "go run").
func Path() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir, err := dir(exe, os.TempDir(), wd)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, FileName), nil
}

// dir resolves symlinks of exe and tmp and returns wd when the executable's
// directory is inside tmp, else the executable's directory.
func dir(exe, tmp, wd string) (string, error) {
	resolvedExe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(resolvedExe)

	tmpDir, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		return "", err
	}

	if rel, err := filepath.Rel(tmpDir, exeDir); err == nil && !outside(rel) {
		return wd, nil
	}

	return exeDir, nil
}

// outside reports whether rel (from filepath.Rel) climbs out of its base,
// i.e. is ".." or starts with "../".
func outside(rel string) bool {
	return rel == ".." || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)
}

// Load reads path. A missing file is created with Defaults and returned as
// such; fields missing from an existing file keep their Defaults value (the
// file is not rewritten). An unreadable or invalid file is left untouched;
// Load returns Defaults and an error naming path.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := Save(path, Defaults()); err != nil {
			return Defaults(), err
		}
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}

	s := Defaults()
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), fmt.Errorf("%s: %w", path, err)
	}

	return s, nil
}

// Save writes s to path as indented JSON.
func Save(path string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o644)
}
