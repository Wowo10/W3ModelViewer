package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustPanic returns the message f panics with, failing if it doesn't.
func mustPanic(t *testing.T, f func()) (msg string) {
	t.Helper()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want panic, got none")
		}
		msg = fmt.Sprint(r)
	}()
	f()

	return ""
}

func requireContains(t *testing.T, msg string, parts ...string) {
	t.Helper()

	for _, p := range parts {
		if !strings.Contains(msg, p) {
			t.Errorf("panic %q lacks %q", msg, p)
		}
	}
}

func TestLoadPortMissingKeepsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	content := []byte("{\"installPath\": \"/x\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 8001 || s.InstallPath != "/x" {
		t.Errorf("Load = %+v, want port 8001 and installPath /x", s)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Errorf("file rewritten: %q", data)
	}
}

func TestLoadPortSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(`{"port": 9000}`), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 9000 {
		t.Errorf("Port = %d, want 9000", s.Port)
	}
}

func TestWebAddr(t *testing.T) {
	path := "/cfg/" + FileName

	if got := WebAddr(path, Settings{Port: 9000}, ""); got != "127.0.0.1:9000" {
		t.Errorf("WebAddr = %q, want 127.0.0.1:9000", got)
	}
	if got := WebAddr(path, Settings{Port: 9000}, "localhost:1234"); got != "localhost:1234" {
		t.Errorf("WebAddr with -addr = %q, want localhost:1234", got)
	}
	for _, port := range []int{1, 65535} {
		WebAddr(path, Settings{Port: port}, "")
	}
	for _, port := range []int{0, -1, 65536} {
		msg := mustPanic(t, func() { WebAddr(path, Settings{Port: port}, "") })
		requireContains(t, msg, path, fmt.Sprint(port))
	}
}

func TestWebInstall(t *testing.T) {
	path := "/cfg/" + FileName

	t.Run("empty", func(t *testing.T) {
		msg := mustPanic(t, func() { WebInstall(path, Defaults()) })
		requireContains(t, msg, path, "installPath", "make run")
	})

	t.Run("invalid", func(t *testing.T) {
		dir := t.TempDir()
		msg := mustPanic(t, func() { WebInstall(path, Settings{InstallPath: dir, Port: 8001}) })
		requireContains(t, msg, path, dir, "no .build.info", "make run")
	})

	t.Run("valid", func(t *testing.T) {
		dir := t.TempDir()
		makeFakeInstall(t, dir)
		if got := WebInstall(path, Settings{InstallPath: dir, Port: 8001}); got != dir {
			t.Errorf("WebInstall = %q, want %q", got, dir)
		}
	})
}
