// Package console covers running the viewer without a console: on Windows
// the release is a GUI program that attaches to the terminal's console when
// there is one, and fatal panics go to a log file when there is none.
package console

import (
	"os"
	"runtime/debug"
)

var logPath string

// CrashLog sends fatal panic and runtime output from every goroutine to the
// file at path (appended to), in addition to stderr. When the file can't be
// opened nothing is logged.
func CrashLog(path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	if debug.SetCrashOutput(f, debug.CrashOptions{}) != nil {
		return
	}

	logPath = path
}

// Finish stops the crash log and removes the file if nothing was written to
// it. Call it on every normal exit, not with defer: a panic in main runs
// deferred calls before the crash output is written.
func Finish() {
	if logPath == "" {
		return
	}

	debug.SetCrashOutput(nil, debug.CrashOptions{})

	if info, err := os.Stat(logPath); err == nil && info.Size() == 0 {
		os.Remove(logPath)
	}

	logPath = ""
}
