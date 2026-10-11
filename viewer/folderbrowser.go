package viewer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
	"modelView/settings"
)

// folderBrowser is the ImGui folder picker used when no native dialog is
// available (or it fails): the current directory, its subdirectories to
// click into, and its install check.
type folderBrowser struct {
	dir      string
	entries  []string // subdirectory names, sorted case-insensitively
	readErr  error
	checkErr error // settings.CheckInstall(dir)
}

// newFolderBrowser starts at start's directory if it exists, else the home
// directory.
func newFolderBrowser(start string) *folderBrowser {
	dir := start
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if home, err := os.UserHomeDir(); err == nil {
			dir = home
		}
	}

	fb := &folderBrowser{}
	fb.setDir(dir)
	return fb
}

func (fb *folderBrowser) setDir(dir string) {
	fb.dir = dir
	fb.checkErr = settings.CheckInstall(dir)

	entries, err := os.ReadDir(dir)
	fb.readErr = err
	if err != nil {
		fb.entries = nil
		return
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	fb.entries = names
}

// draw shows the browser window. Selecting a folder or cancelling closes it
// (s.browser is set to nil).
func (fb *folderBrowser) draw(s *setupState) {
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 480, Y: 420}, imgui.CondFirstUseEver)

	if imgui.Begin("Choose folder") {
		imgui.TextWrapped(fb.dir)

		if imgui.Button("Up") {
			fb.setDir(filepath.Dir(fb.dir))
		}
		imgui.SameLineV(0, -1)
		if imgui.Button("Home") {
			if home, err := os.UserHomeDir(); err == nil {
				fb.setDir(home)
			}
		}

		imgui.Separator()

		if fb.readErr != nil {
			imgui.TextColored(imgui.Vec4{X: 1, Y: 0.4, Z: 0.4, W: 1}, fb.readErr.Error())
		} else {
			if imgui.BeginChildStrV("##dirs", imgui.Vec2{X: 0, Y: -imgui.CurrentStyle().FramePadding().Y*2 - imgui.CalcTextSize("x").Y*3}, imgui.ChildFlagsBorders, 0) {
				for _, name := range fb.entries {
					imgui.SelectableBool(name)
					if imgui.IsItemClicked() {
						fb.setDir(filepath.Join(fb.dir, name))
					}
				}
			}
			imgui.EndChild()
		}

		imgui.Separator()

		if fb.checkErr != nil {
			imgui.TextColored(imgui.Vec4{X: 1, Y: 0.7, Z: 0.3, W: 1}, fb.checkErr.Error())
		} else {
			imgui.TextColored(imgui.Vec4{X: 0.4, Y: 0.9, Z: 0.4, W: 1}, "OK")
		}

		if imgui.Button("Select this folder") {
			s.path = fb.dir
			s.checkPath()
			s.browser = nil
		}
		imgui.SameLineV(0, -1)
		if imgui.Button("Cancel") {
			s.browser = nil
		}
	}
	imgui.End()
}
