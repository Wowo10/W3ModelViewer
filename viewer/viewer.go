// Package viewer hosts the interactive window: SDL2 owns the window and the
// input queue, go-gl issues the GL calls and cimgui-go draws the UI on top.
package viewer

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/impl/opengl3"
	"github.com/AllenDang/cimgui-go/impl/sdl2"
	"github.com/go-gl/gl/v3.2-core/gl"
	"github.com/veandco/go-sdl2/sdl"
	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/catalog"
	"modelView/settings"
)

// Config describes the window to open.
type Config struct {
	Title  string
	Width  int32
	Height int32
}

// DefaultConfig returns the settings used by the --view flag.
func DefaultConfig() Config {
	return Config{Title: "modelViewer", Width: 1280, Height: 720}
}

// Startup carries what the viewer needs before it can open an install:
// where modelViewer.json lives, what it held on start (or the error loading
// it), and how to open an install directory once one is chosen.
type Startup struct {
	SettingsPath string
	Settings     settings.Settings
	SettingsErr  error
	Open         func(dir string) (*casc.Storage, root.Root, *catalog.Catalog)
}

// glslVersion matches the 3.2 core profile requested below.
const glslVersion = "#version 150"

// bgColor is the GL clear color in every phase.
var bgColor = [3]float32{0.10, 0.11, 0.13}

// Run opens the window and pumps events until the user closes it. With a
// valid configured install it loads it in the background while the window
// shows a loading state; otherwise it shows a setup window asking for the
// install folder. The viewer does no file I/O itself, other than reading
// directories for the folder browser; start.Open does the rest.
func Run(cfg Config, start Startup) error {
	// The GL context is bound to one OS thread, so this goroutine must stay put.
	runtime.LockOSThread()

	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_EVENTS); err != nil {
		return fmt.Errorf("sdl init: %w", err)
	}
	defer sdl.Quit()

	if err := setGLAttributes(); err != nil {
		return err
	}

	window, err := createWindow(cfg)
	if err != nil {
		return err
	}
	defer window.Destroy()

	glCtx, err := window.GLCreateContext()
	if err != nil {
		return fmt.Errorf("create gl context: %w", err)
	}
	defer sdl.GLDeleteContext(glCtx)

	if err := initGL(window, glCtx); err != nil {
		return err
	}

	imgui.CreateContext()
	defer imgui.DestroyContext()

	configureImGui()

	if !sdl2.InitForOpenGL(sdl2.NewSDLWindowFromC(unsafe.Pointer(window)), uintptr(unsafe.Pointer(glCtx))) {
		return fmt.Errorf("imgui sdl2 backend init failed")
	}
	defer sdl2.Shutdown()

	if !opengl3.InitV(glslVersion) {
		return fmt.Errorf("imgui opengl3 backend init failed")
	}
	defer opengl3.Shutdown()

	app := newAppState(start)
	defer app.release()

	for !app.quit {
		pollEvents(window, app)

		if window.GetFlags()&sdl.WINDOW_MINIMIZED != 0 {
			sdl.Delay(10)
			continue
		}

		renderFrame(window, app)
	}

	return nil
}

func setGLAttributes() error {
	for _, attr := range []struct {
		key sdl.GLattr
		val int
	}{
		{sdl.GL_CONTEXT_MAJOR_VERSION, 3},
		{sdl.GL_CONTEXT_MINOR_VERSION, 2},
		{sdl.GL_CONTEXT_PROFILE_MASK, sdl.GL_CONTEXT_PROFILE_CORE},
		{sdl.GL_DOUBLEBUFFER, 1},
		{sdl.GL_DEPTH_SIZE, 24},
		{sdl.GL_STENCIL_SIZE, 8},
	} {
		if err := sdl.GLSetAttribute(attr.key, attr.val); err != nil {
			return fmt.Errorf("sdl gl attribute %d: %w", attr.key, err)
		}
	}
	return nil
}

func createWindow(cfg Config) (*sdl.Window, error) {
	window, err := sdl.CreateWindow(
		cfg.Title,
		sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
		cfg.Width, cfg.Height,
		sdl.WINDOW_OPENGL|sdl.WINDOW_RESIZABLE|sdl.WINDOW_SHOWN|sdl.WINDOW_ALLOW_HIGHDPI,
	)
	if err != nil {
		return nil, fmt.Errorf("create window: %w", err)
	}
	return window, nil
}

// initGL makes glCtx current, enables vsync if possible and loads the GL functions.
func initGL(window *sdl.Window, glCtx sdl.GLContext) error {
	if err := window.GLMakeCurrent(glCtx); err != nil {
		return fmt.Errorf("make gl context current: %w", err)
	}
	if err := sdl.GLSetSwapInterval(1); err != nil {
		// Not fatal: some drivers refuse vsync, we just run uncapped.
		fmt.Fprintln(os.Stderr, "viewer: vsync unavailable:", err)
	}

	if err := gl.Init(); err != nil {
		return fmt.Errorf("gl init: %w", err)
	}
	return nil
}

func configureImGui() {
	io := imgui.CurrentIO()
	io.SetConfigFlags(io.ConfigFlags() | imgui.ConfigFlagsNavEnableKeyboard)
	imgui.StyleColorsDark()
}

// pollEvents drains the SDL queue, forwarding everything to ImGui and
// flagging app.quit when the window is closed. The camera only reacts to
// mouse input once a model is loaded (app.ui is set).
func pollEvents(window *sdl.Window, app *appState) {
	for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
		processEvent(event)

		switch e := event.(type) {
		case *sdl.QuitEvent:
			app.quit = true
		case *sdl.WindowEvent:
			if e.Event == sdl.WINDOWEVENT_CLOSE && e.WindowID == windowID(window) {
				app.quit = true
			}
		case *sdl.MouseMotionEvent:
			if app.ui != nil && !imgui.CurrentIO().WantCaptureMouse() && e.State&sdl.ButtonLMask() != 0 {
				app.ui.camera.rotate(float32(e.XRel), float32(e.YRel))
			}
		case *sdl.MouseWheelEvent:
			if app.ui != nil && !imgui.CurrentIO().WantCaptureMouse() {
				steps := e.PreciseY
				if steps == 0 {
					steps = float32(e.Y)
				}
				app.ui.camera.zoom(steps)
			}
		}
	}
}

// renderFrame draws the current phase's ImGui windows (and syncs and draws
// the mesh once a model is loaded), then swaps buffers.
func renderFrame(window *sdl.Window, app *appState) {
	opengl3.NewFrame()
	sdl2.NewFrame()
	imgui.NewFrame()

	app.draw()

	var rebuilt bool
	if app.ui != nil {
		rebuilt = app.ui.syncMesh()
		app.ui.syncPose(rebuilt)
		app.ui.syncTeam()
	}

	imgui.Render()

	w, h := window.GLGetDrawableSize()
	gl.Viewport(0, 0, w, h)
	gl.ClearColor(bgColor[0], bgColor[1], bgColor[2], 1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	if app.ui != nil && h > 0 && len(app.ui.mesh.geosets) > 0 {
		view, proj := app.ui.camera.matrices(float32(w) / float32(h))
		app.ui.mesh.draw(app.ui.program, view, proj, app.ui.team, app.ui.models.model.alphas, app.ui.pose.texture)
	}

	opengl3.RenderDrawData(imgui.CurrentDrawData())

	window.GLSwap()
}

func windowID(window *sdl.Window) uint32 {
	id, err := window.GetID()
	if err != nil {
		return 0
	}
	return id
}
