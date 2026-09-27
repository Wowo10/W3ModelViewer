package viewer

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"
)

// uiState keeps whatever the panels need between frames.
type uiState struct {
	quit       bool
	showDemo   bool
	clearColor [3]float32
	scale      float32
}

func newUIState() *uiState {
	return &uiState{
		clearColor: [3]float32{0.10, 0.11, 0.13},
		scale:      1.0,
	}
}

func (ui *uiState) draw() {
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 340, Y: 240}, imgui.CondFirstUseEver)
	if imgui.Begin("modelViewer") {
		imgui.Text("SDL2 window, OpenGL 3.2 core, Dear ImGui.")
		imgui.Separator()

		imgui.Checkbox("ImGui demo window", &ui.showDemo)
		imgui.SliderFloat("scale", &ui.scale, 0.1, 10)
		imgui.ColorEdit3("clear color", &ui.clearColor)

		imgui.Separator()
		imgui.Text(fmt.Sprintf("%.1f FPS (%.2f ms/frame)",
			imgui.CurrentIO().Framerate(), 1000/imgui.CurrentIO().Framerate()))

		if imgui.Button("Quit") {
			ui.quit = true
		}
	}
	imgui.End()

	if ui.showDemo {
		imgui.ShowDemoWindowV(&ui.showDemo)
	}
}
