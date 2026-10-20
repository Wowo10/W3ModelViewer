package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	tmp := t.TempDir()
	outside := t.TempDir()

	t.Run("exe outside tmp", func(t *testing.T) {
		exeDir := filepath.Join(outside, "bin")
		mustMkdirAll(t, exeDir)
		exe := filepath.Join(exeDir, "modelViewer")
		mustWriteFile(t, exe)

		got, err := dir(exe, tmp, "/some/wd")
		if err != nil {
			t.Fatal(err)
		}
		if got != exeDir {
			t.Errorf("dir() = %q, want %q", got, exeDir)
		}
	})

	t.Run("exe inside tmp", func(t *testing.T) {
		exeDir := filepath.Join(tmp, "go-build123", "b001", "exe")
		mustMkdirAll(t, exeDir)
		exe := filepath.Join(exeDir, "modelViewer")
		mustWriteFile(t, exe)
		wd := filepath.Join(outside, "repo")
		mustMkdirAll(t, wd)

		got, err := dir(exe, tmp, wd)
		if err != nil {
			t.Fatal(err)
		}
		if got != wd {
			t.Errorf("dir() = %q, want %q (wd)", got, wd)
		}
	})

	t.Run("exe is a symlink in tmp pointing outside", func(t *testing.T) {
		realDir := filepath.Join(outside, "real")
		mustMkdirAll(t, realDir)
		realExe := filepath.Join(realDir, "modelViewer")
		if err := os.WriteFile(realExe, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}

		linkDir := filepath.Join(tmp, "go-build456", "b001", "exe")
		mustMkdirAll(t, linkDir)
		linkExe := filepath.Join(linkDir, "modelViewer")
		if err := os.Symlink(realExe, linkExe); err != nil {
			t.Fatal(err)
		}
		wd := filepath.Join(outside, "repo2")
		mustMkdirAll(t, wd)

		got, err := dir(linkExe, tmp, wd)
		if err != nil {
			t.Fatal(err)
		}
		if got != realDir {
			t.Errorf("dir() = %q, want %q (real dir)", got, realDir)
		}
	})
}

func TestLoadCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s != Defaults() {
		t.Errorf("Settings = %+v, want %+v", s, Defaults())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Load didn't create %s: %v", path, err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s2 != s {
		t.Errorf("reloaded Settings = %+v, want %+v", s2, s)
	}
	if string(data) != mustMarshal(t, Defaults()) {
		t.Errorf("created file = %q, want %q", data, mustMarshal(t, Defaults()))
	}
	if want := "{\n  \"installPath\": \"\"\n}\n"; string(data) != want {
		t.Errorf("created file = %q, want %q", data, want)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	corrupt := []byte("not json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err == nil {
		t.Fatal("Load on corrupt file: want error, got nil")
	}
	if s != Defaults() {
		t.Errorf("Settings on corrupt file = %+v, want %+v", s, Defaults())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(corrupt) {
		t.Errorf("corrupt file was modified: got %q, want %q", data, corrupt)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	want := Settings{InstallPath: "/opt/Warcraft III"}

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustMarshal(t *testing.T, s Settings) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLoadKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	content := []byte("{\"installPath\": \"/x\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.InstallPath != "/x" {
		t.Errorf("Load = %+v, want installPath /x", s)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("file rewritten: %q", data)
	}
}

func TestLoadIgnoresPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	content := []byte("{\"installPath\": \"/x\", \"port\": 8001}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s != (Settings{InstallPath: "/x"}) {
		t.Errorf("Load = %+v, want installPath /x", s)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("file rewritten: %q", data)
	}
}
