package settings

import (
	"modelView/extract/idxPackages"
	"os"
	"path/filepath"
	"testing"
)

const testBuildKey = "0123456789abcdef0123456789abcdef"

// makeFakeInstall builds a minimal install under dir: .build.info with
// testBuildKey, its build config and the 16 idx files idxPackages reads.
func makeFakeInstall(t *testing.T, dir string) {
	t.Helper()

	biData := "Build Key!HEX:16|CDN Key!HEX:16\n" + testBuildKey + "|" + testBuildKey + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".build.info"), []byte(biData), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgDir := filepath.Join(dir, "Data", "config", testBuildKey[0:2], testBuildKey[2:4])
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, testBuildKey), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	dataDir := filepath.Join(dir, "Data", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range idxPackages.Filenames() {
		if err := os.WriteFile(filepath.Join(dataDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckInstallValid(t *testing.T) {
	dir := t.TempDir()
	makeFakeInstall(t, dir)

	if err := CheckInstall(dir); err != nil {
		t.Errorf("CheckInstall on a valid install: %v", err)
	}
}

func TestCheckInstallMissingPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")

	if err := CheckInstall(dir); err == nil {
		t.Error("CheckInstall on a missing path: want error, got nil")
	}
}

func TestCheckInstallNotDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CheckInstall(file); err == nil {
		t.Error("CheckInstall on a regular file: want error, got nil")
	}
}

func TestCheckInstallMissingBuildInfo(t *testing.T) {
	dir := t.TempDir()
	makeFakeInstall(t, dir)
	if err := os.Remove(filepath.Join(dir, ".build.info")); err != nil {
		t.Fatal(err)
	}

	if err := CheckInstall(dir); err == nil {
		t.Error("CheckInstall without .build.info: want error, got nil")
	}
}

func TestCheckInstallMissingIdxFile(t *testing.T) {
	dir := t.TempDir()
	makeFakeInstall(t, dir)
	missing := idxPackages.Filenames()[0]
	if err := os.Remove(filepath.Join(dir, "Data", "data", missing)); err != nil {
		t.Fatal(err)
	}

	err := CheckInstall(dir)
	if err == nil {
		t.Fatal("CheckInstall with a missing idx file: want error, got nil")
	}
	if !contains(err.Error(), missing) {
		t.Errorf("CheckInstall error = %q, want it to name %q", err, missing)
	}
}

func TestCheckInstallRealWardata(t *testing.T) {
	dir := "../wardata"
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("wardata not found: %v", err)
	}

	if err := CheckInstall(dir); err != nil {
		t.Errorf("CheckInstall(%q): %v", dir, err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
