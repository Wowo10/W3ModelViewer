package viewer

import (
	"time"

	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/catalog"
	"modelView/settings"
)

// appState drives the viewer through its phases: setup (asking for an
// install folder), loading (opening one in the background) and, once a
// model is loaded, the normal UI (ui). Exactly one of setup, loading or ui
// is active at a time.
type appState struct {
	start Startup
	quit  bool

	setup   *setupState
	loading *loadingState
	ui      *uiState

	storage *casc.Storage // set once loading finishes, closed on release
}

// loadingState tracks an install opening on a background goroutine.
type loadingState struct {
	dir     string
	started time.Time
	done    chan loadResult
}

type loadResult struct {
	storage *casc.Storage
	root    root.Root
	cat     *catalog.Catalog
}

func newAppState(start Startup) *appState {
	app := &appState{start: start}

	if start.Settings.InstallPath != "" && settings.CheckInstall(start.Settings.InstallPath) == nil {
		app.startLoading(start.Settings.InstallPath)
	} else {
		app.setup = newSetupState(start)
	}

	return app
}

// startLoading opens dir on a background goroutine, switching from setup to
// the loading phase.
func (app *appState) startLoading(dir string) {
	app.setup = nil
	done := make(chan loadResult, 1)
	app.loading = &loadingState{dir: dir, started: time.Now(), done: done}

	go func() {
		storage, r, cat := app.start.Open(dir)
		done <- loadResult{storage: storage, root: r, cat: cat}
	}()
}

// draw renders the active phase's ImGui windows, advancing from loading to
// the loaded UI once the background open finishes.
func (app *appState) draw() {
	switch {
	case app.setup != nil:
		app.setup.draw(app)
	case app.loading != nil:
		select {
		case res := <-app.loading.done:
			app.storage = res.storage
			app.ui = newUIState(res.storage, res.root, res.cat)
			app.loading = nil
		default:
			drawLoading(app.loading.dir, time.Since(app.loading.started))
		}
	}

	if app.ui != nil {
		app.ui.draw()
	}
}

// release frees whatever GL resources the loaded UI holds and closes the
// archive. Call it while the GL context is still current.
func (app *appState) release() {
	if app.ui != nil {
		app.ui.release()
	}
	if app.storage != nil {
		app.storage.Close()
	}
}
