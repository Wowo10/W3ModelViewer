package settings

import "fmt"

// WebInstall returns s.InstallPath for the web viewer. It panics, naming
// path (the file s was read from) and how to fix it, when the install path
// is empty or CheckInstall rejects it.
func WebInstall(path string, s Settings) string {
	const fix = `run "make run" and pick the folder, or set "installPath" in that file`

	if s.InstallPath == "" {
		panic(fmt.Sprintf(`%s has no installPath: %s`, path, fix))
	}
	if err := CheckInstall(s.InstallPath); err != nil {
		panic(fmt.Sprintf(`installPath %q in %s is not a usable Warcraft III install: %v; %s`, s.InstallPath, path, err, fix))
	}

	return s.InstallPath
}

// WebAddr returns the address the web viewer listens on: addr when it's
// non-empty (the -addr override), else 127.0.0.1:<port>. It panics, naming
// path, when s.Port is outside 1-65535, even if addr overrides it.
func WebAddr(path string, s Settings, addr string) string {
	if s.Port < 1 || s.Port > 65535 {
		panic(fmt.Sprintf("port %d in %s is out of range 1-65535", s.Port, path))
	}
	if addr != "" {
		return addr
	}

	return fmt.Sprintf("127.0.0.1:%d", s.Port)
}
