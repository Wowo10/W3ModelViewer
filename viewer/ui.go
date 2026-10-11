package viewer

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v3.2-core/gl"
	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/catalog"
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

	pose *poseBuffer

	team teamTextures
}

// teamTextures is the selected player's TeamColorNN/TeamGlowNN, shared by
// every classic team-coloured layer so switching players doesn't rebuild
// the mesh.
type teamTextures struct {
	color, glow uint32
	player      int32 // which player these are, -1 = none loaded yet
}

func newUIState(storage *casc.Storage, r root.Root, cat *catalog.Catalog) *uiState {
	return &uiState{
		storage:    storage,
		root:       r,
		models:     newModelPicker(storage, r, cat),
		clearColor: [3]float32{0.10, 0.11, 0.13},
		program:    newMeshProgram(),
		meshFor:    -1,
		pose:       newPoseBuffer(),
		team:       teamTextures{player: -1},
	}
}

// syncMesh uploads the currently loaded model's geosets at its rendered LOD
// when the picker has loaded a different model than the mesh was built
// from, or the loaded model's rendered LOD differs from the mesh's,
// releasing the previous mesh first. The camera is only reframed when the
// new mesh is of a different asset than the one last framed (by
// entry.Name, case-insensitively), so switching editions of the same asset,
// or switching LOD, keeps the camera. It reports whether the mesh was
// rebuilt, so syncPose knows to upload the new mesh's pose even if
// model.pose happens to match the last one it saw.
func (ui *uiState) syncMesh() bool {
	model := &ui.models.model
	var lod uint32
	if model.lod != -1 {
		lod = model.lods[model.lod].LOD
	}
	if ui.models.loaded == ui.meshFor && lod == ui.mesh.lod {
		return false
	}

	ui.mesh.release()
	ui.mesh = uploadMesh(ui.storage, model.geosets, model.alphas, model.layers, model.priority, lod)
	if ui.mesh.bounded && !strings.EqualFold(model.name, ui.framedName) {
		ui.camera.frame(ui.mesh.min, ui.mesh.max)
		ui.framedName = model.name
	}
	ui.meshFor = ui.models.loaded
	return true
}

// syncPose uploads the selected model's current node matrices to the pose
// buffer when the mesh was just rebuilt (rebuilt) or the pose changed
// (model.pose differs from the last upload).
func (ui *uiState) syncPose(rebuilt bool) {
	pose := ui.models.model.pose
	if !rebuilt && pose == ui.pose.pose {
		return
	}
	ui.pose.upload(ui.models.model.matrices)
	ui.pose.pose = pose
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
	ui.pose.release()
	if ui.team.player != -1 {
		gl.DeleteTextures(1, &ui.team.color)
		gl.DeleteTextures(1, &ui.team.glow)
	}
}

func (ui *uiState) draw() {
	ui.models.draw()
	drawFPS()
}

// drawFPS draws the frame rate on the foreground draw list, above every
// window, in the main viewport's top-right corner, on a dark translucent
// box so it stays readable over any window or the model.
func drawFPS() {
	fr := imgui.CurrentIO().Framerate()
	text := fmt.Sprintf("%.0f FPS (%.1f ms)", fr, 1000/fr)
	size := imgui.CalcTextSize(text)

	const margin, pad = 10, 4
	display := imgui.CurrentIO().DisplaySize()
	max := imgui.Vec2{X: display.X - margin, Y: margin + pad*2 + size.Y}
	min := imgui.Vec2{X: max.X - pad*2 - size.X, Y: margin}

	dl := imgui.ForegroundDrawListViewportPtr()
	dl.AddRectFilledV(min, max, imgui.ColorU32Vec4(imgui.Vec4{X: 0, Y: 0, Z: 0, W: 0.6}), 3, imgui.DrawFlagsNone)
	dl.AddTextVec2(imgui.Vec2{X: min.X + pad, Y: min.Y + pad}, imgui.ColorU32Col(imgui.ColText), text)
}
