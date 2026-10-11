package viewer

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
	"modelView/settings"
)

// setupState is the setup window: a path field the user edits, checked
// against settings.CheckInstall as they type.
type setupState struct {
	start Startup

	// configuredErr is why start.Settings.InstallPath failed, or nil when
	// it was empty.
	configuredErr error

	path     string
	checkErr error // settings.CheckInstall(path), re-run only when path changes
	saveErr  error // settings.Save's error, shown and left in setup when set
}

func newSetupState(start Startup) *setupState {
	s := &setupState{start: start}

	if start.Settings.InstallPath != "" {
		s.configuredErr = settings.CheckInstall(start.Settings.InstallPath)
	}

	s.path = prefillPath(start.Settings.InstallPath)
	s.checkPath()

	return s
}

// prefillPath is "./wardata" (made absolute) when it passes the install
// check, else configured (so the user sees why it failed), else empty.
func prefillPath(configured string) string {
	if settings.CheckInstall("./wardata") == nil {
		if abs, err := filepath.Abs("./wardata"); err == nil {
			return abs
		}
		return "./wardata"
	}
	return configured
}

func (s *setupState) checkPath() {
	s.checkErr = settings.CheckInstall(s.path)
}

// draw shows the setup window and, once the user picks a valid folder and
// confirms, starts loading it through app.startLoading.
func (s *setupState) draw(app *appState) {
	display := imgui.CurrentIO().DisplaySize()
	imgui.SetNextWindowPosV(imgui.Vec2{X: display.X / 2, Y: display.Y / 2}, imgui.CondFirstUseEver, imgui.Vec2{X: 0.5, Y: 0.5})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 560, Y: 0}, imgui.CondFirstUseEver)

	if imgui.Begin("Set up Warcraft III") {
		imgui.TextWrapped("Choose the Warcraft III install folder, the one with .build.info and Data/.")

		if s.start.SettingsErr != nil {
			imgui.TextColored(imgui.Vec4{X: 1, Y: 0.4, Z: 0.4, W: 1}, fmt.Sprintf("Couldn't read %s: %v (it will be replaced when you save)", s.start.SettingsPath, s.start.SettingsErr))
		} else if s.configuredErr != nil {
			imgui.TextColored(imgui.Vec4{X: 1, Y: 0.7, Z: 0.3, W: 1}, fmt.Sprintf("Configured path %q: %v", s.start.Settings.InstallPath, s.configuredErr))
		}

		if s.saveErr != nil {
			imgui.TextColored(imgui.Vec4{X: 1, Y: 0.4, Z: 0.4, W: 1}, fmt.Sprintf("Couldn't save %s: %v", s.start.SettingsPath, s.saveErr))
		}

		imgui.Separator()

		if imgui.InputTextWithHint("##path", "install folder", &s.path, imgui.InputTextFlagsNone, nil) {
			s.checkPath()
		}

		if s.checkErr != nil {
			imgui.TextColored(imgui.Vec4{X: 1, Y: 0.7, Z: 0.3, W: 1}, s.checkErr.Error())
		} else {
			imgui.TextColored(imgui.Vec4{X: 0.4, Y: 0.9, Z: 0.4, W: 1}, fmt.Sprintf("OK: build %s", buildKeyPrefix(s.path)))
		}

		imgui.BeginDisabledV(s.checkErr != nil)
		if imgui.Button("Open") {
			s.open(app)
		}
		imgui.EndDisabled()
	}
	imgui.End()
}

// buildKeyPrefix returns the first 8 characters of dir's build key, for the
// "OK: build ..." line; it assumes CheckInstall(dir) already passed.
func buildKeyPrefix(dir string) string {
	key := settings.BuildKey(dir)
	if len(key) < 8 {
		return key
	}
	return key[:8]
}

// open saves the confirmed path and starts loading it, staying in setup on
// a save error.
func (s *setupState) open(app *appState) {
	abs, err := filepath.Abs(s.path)
	if err != nil {
		s.saveErr = err
		return
	}

	if err := settings.Save(s.start.SettingsPath, settings.Settings{InstallPath: abs}); err != nil {
		s.saveErr = err
		return
	}

	app.startLoading(abs)
}

// drawLoading shows a centred "Loading <dir>..." message with the elapsed
// time, while an install opens on a background goroutine.
func drawLoading(dir string, elapsed time.Duration) {
	display := imgui.CurrentIO().DisplaySize()
	imgui.SetNextWindowPosV(imgui.Vec2{X: display.X / 2, Y: display.Y / 2}, imgui.CondAlways, imgui.Vec2{X: 0.5, Y: 0.5})

	flags := imgui.WindowFlagsNoResize | imgui.WindowFlagsNoMove | imgui.WindowFlagsNoCollapse | imgui.WindowFlagsAlwaysAutoResize
	if imgui.BeginV("Loading", nil, flags) {
		imgui.Text(fmt.Sprintf("Loading %s… (%.0fs)", dir, elapsed.Seconds()))
	}
	imgui.End()
}
