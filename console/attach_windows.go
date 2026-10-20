package console

import (
	"os"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uint32(0)

var attachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

// Attach gives a GUI-subsystem program (linked with -H windowsgui) its
// output back when it is started from a terminal: handles that aren't
// usable are reopened on the parent's console, and os.Stdout and os.Stderr
// follow. Redirected handles are left alone. It returns whether stderr goes
// somewhere (a redirect, an inherited console or the attached one). A
// console-subsystem build has usable handles, so there it does nothing.
func Attach() bool {
	outOK, errOK := usable(windows.STD_OUTPUT_HANDLE), usable(windows.STD_ERROR_HANDLE)
	if outOK && errOK {
		return true
	}

	if r, _, _ := attachConsole.Call(uintptr(attachParentProcess)); r == 0 {
		return errOK
	}

	if !outOK {
		reopen(windows.STD_OUTPUT_HANDLE, &os.Stdout, "/dev/stdout")
	}

	if !errOK {
		errOK = reopen(windows.STD_ERROR_HANDLE, &os.Stderr, "/dev/stderr")
	}

	return errOK
}

func usable(std uint32) bool {
	h, err := windows.GetStdHandle(std)
	if err != nil || h == 0 || h == windows.InvalidHandle {
		return false
	}

	t, err := windows.GetFileType(h)

	return err == nil && t != windows.FILE_TYPE_UNKNOWN
}

// reopen opens the attached console's output, makes it the process's std
// handle (the Go runtime writes panics through it) and points *f at it.
func reopen(std uint32, f **os.File, name string) bool {
	conout, err := windows.UTF16PtrFromString("CONOUT$")
	if err != nil {
		return false
	}

	h, err := windows.CreateFile(conout, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return false
	}

	if windows.SetStdHandle(std, h) != nil {
		windows.CloseHandle(h)

		return false
	}

	*f = os.NewFile(uintptr(h), name)

	return true
}
