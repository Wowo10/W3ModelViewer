package viewer

import (
	"github.com/AllenDang/cimgui-go/imgui"
)

// drawAnimation shows the "Animation" window: a sequence combo and a frame
// slider that re-pose the loaded model (via selectedModel.setPose) on
// change, and a "start" button that resets the frame to the sequence's
// start. With no model loaded, or no sequences, it says so instead.
func (p *modelPicker) drawAnimation() {
	imgui.SetNextWindowPosV(imgui.Vec2{X: 500, Y: 460}, imgui.CondFirstUseEver, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 560, Y: 100}, imgui.CondFirstUseEver)
	if imgui.Begin("Animation") {
		m := &p.model
		switch {
		case p.loaded == -1:
			imgui.Text("Select a model")
		case m.seq == -1:
			imgui.Text("No sequences: bind pose")
		default:
			if imgui.ComboStrarr("sequence", &m.seq, m.seqLabels, int32(len(m.seqLabels))) {
				m.setPose(m.seq, int32(m.sequences[m.seq].Start))
			}

			s := m.sequences[m.seq]
			start, end := int32(s.Start), int32(s.End)
			if end < start {
				end = start // not seen in wardata; keep the slider usable
			}

			frame := m.frame
			if imgui.SliderInt("frame", &frame, start, end) {
				m.setPose(m.seq, frame)
			}
			if imgui.Button("start") {
				m.setPose(m.seq, start)
			}
		}
	}
	imgui.End()
}
