package viewer

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.2-core/gl"
	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/texture"
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

	team teamTextures
}

// teamTextures is the selected player's TeamColorNN/TeamGlowNN, shared by
// every classic team-coloured layer so switching players doesn't rebuild
// the mesh.
type teamTextures struct {
	color, glow uint32
	player      int32 // which player these are, -1 = none loaded yet
}

func newUIState(storage *casc.Storage, r root.Root) *uiState {
	return &uiState{
		storage:    storage,
		root:       r,
		models:     newModelPicker(storage, r),
		clearColor: [3]float32{0.10, 0.11, 0.13},
		program:    newMeshProgram(),
		meshFor:    -1,
		team:       teamTextures{player: -1},
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
	ui.mesh = uploadMesh(ui.storage, ui.models.model.geosets, ui.models.model.alphas, ui.models.model.layers, ui.models.model.priority)
	if len(ui.mesh.geosets) > 0 && !strings.EqualFold(ui.models.model.name, ui.framedName) {
		ui.camera.frame(ui.mesh.min, ui.mesh.max)
		ui.framedName = ui.models.model.name
	}
	ui.meshFor = ui.models.loaded
}

// syncTeam uploads the selected player's TeamColorNN/TeamGlowNN textures
// when it differs from the player last loaded, deleting the previous pair
// first (and on first use, since player -1 never matches).
func (ui *uiState) syncTeam() {
	player := ui.models.player
	if player == ui.team.player {
		return
	}

	if ui.team.player != -1 {
		gl.DeleteTextures(1, &ui.team.color)
		gl.DeleteTextures(1, &ui.team.glow)
	}

	ui.team.color = ui.loadTeamTexture("TeamColor", player)
	ui.team.glow = ui.loadTeamTexture("TeamGlow", player)
	ui.team.player = player
}

// loadTeamTexture reads and uploads War3.w3mod's ReplaceableTextures/<name>/
// <name>NN.dds for the given player (00-24).
func (ui *uiState) loadTeamTexture(name string, player int32) uint32 {
	path := fmt.Sprintf("War3.w3mod:ReplaceableTextures/%s/%s%02d.dds", name, name, player)
	entry, ok := ui.root.Lookup(path)
	if !ok {
		panic(fmt.Sprintf("viewer: team texture not found: %s", path))
	}
	data := ui.storage.ReadByCKey(entry.CKey)
	return uploadTexture(texture.ReadDDS(data), data)
}

// release frees the GL resources held by the UI. Call it while the GL
// context is still current.
func (ui *uiState) release() {
	ui.mesh.release()
	ui.program.release()
	if ui.team.player != -1 {
		gl.DeleteTextures(1, &ui.team.color)
		gl.DeleteTextures(1, &ui.team.glow)
	}
}

func (ui *uiState) draw() {
	ui.models.draw()
}
