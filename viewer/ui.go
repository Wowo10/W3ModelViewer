package viewer

import (
	"strings"

	"modelView/extract/casc"
	"modelView/extract/root"
)

// uiState keeps whatever the panels need between frames.
type uiState struct {
	storage    *casc.Storage
	root       root.Root
	models     *modelPicker
	quit       bool
	clearColor [3]float32

	program    *meshProgram
	mesh       modelMesh
	meshFor    int    // index into models of the model mesh was built from, -1 = none
	framedName string // entry.Name the camera was last framed for
	camera     orbitCamera
}

func newUIState(storage *casc.Storage, r root.Root) *uiState {
	return &uiState{
		storage:    storage,
		root:       r,
		models:     newModelPicker(storage, r),
		clearColor: [3]float32{0.10, 0.11, 0.13},
		program:    newMeshProgram(),
		meshFor:    -1,
	}
}

// syncMesh uploads the currently loaded model's geosets when the picker has
// loaded a different model than the mesh was built from, releasing the
// previous mesh first. The camera is only reframed when the new mesh is of a
// different asset than the one last framed (by entry.Name, case-
// insensitively), so switching editions of the same asset keeps the camera.
func (ui *uiState) syncMesh() {
	if ui.models.loaded == ui.meshFor {
		return
	}

	ui.mesh.release()
	ui.mesh = uploadMesh(ui.storage, ui.models.model.geosets, ui.models.model.alphas, ui.models.model.diffuse)
	if len(ui.mesh.geosets) > 0 && !strings.EqualFold(ui.models.model.name, ui.framedName) {
		ui.camera.frame(ui.mesh.min, ui.mesh.max)
		ui.framedName = ui.models.model.name
	}
	ui.meshFor = ui.models.loaded
}

// release frees the GL resources held by the UI. Call it while the GL
// context is still current.
func (ui *uiState) release() {
	ui.mesh.release()
	ui.program.release()
}

func (ui *uiState) draw() {
	ui.models.draw()
}
