package console

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFinishRemovesEmptyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")

	CrashLog(path)
	Finish()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("empty log left behind: %v", err)
	}
}

func TestFinishKeepsOlderLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.log")
	if err := os.WriteFile(path, []byte("old crash\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	CrashLog(path)
	Finish()

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "old crash\n" {
		t.Fatalf("log = %q, %v", got, err)
	}
}

func TestCrashLogPanic(t *testing.T) {
	if path := os.Getenv("CONSOLE_CRASH_LOG"); path != "" {
		CrashLog(path)
		go panic("boom from goroutine")
		time.Sleep(10 * time.Second)

		return
	}

	path := filepath.Join(t.TempDir(), "crash.log")

	cmd := exec.Command(os.Args[0], "-test.run=TestCrashLogPanic")
	cmd.Env = append(os.Environ(), "CONSOLE_CRASH_LOG="+path)

	if err := cmd.Run(); err == nil {
		t.Fatal("child did not crash")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(got), "panic: boom from goroutine") {
		t.Fatalf("log = %q", got)
	}
}
