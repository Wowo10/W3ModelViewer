package viewer

import (
	"math"

	"github.com/AllenDang/cimgui-go/imgui"
)

// playback holds the Animation window's play/pause/loop/speed state and the
// exact current frame and global clock, in milliseconds, kept as float64 so
// slow motion still advances. It survives model and edition switches.
type playback struct {
	playing   bool
	loop      bool
	speed     float32
	frame     float64
	global    float64
	scrubbing bool // the frame slider was active last frame
}

// newPlayback returns a playback that plays its model's default sequence in
// a loop at 1x as soon as it is picked.
func newPlayback() playback {
	return playback{playing: true, loop: true, speed: 1}
}

// advance steps frame and global by dt (wall-clock seconds) scaled by speed,
// when playing and not scrubbing. When frame runs past end, it wraps within
// [start, end] when loop is set, else clamps to end and stops playing.
func (p *playback) advance(dt float64, start, end int32) {
	if !p.playing || p.scrubbing {
		return
	}

	step := dt * 1000 * float64(p.speed)
	p.frame += step
	p.global += step

	s, e := float64(start), float64(end)
	if p.frame > e {
		if p.loop {
			if e == s {
				p.frame = s
			} else {
				p.frame = s + math.Mod(p.frame-s, e-s)
			}
		} else {
			p.frame = e
			p.playing = false
		}
	}
}

// tick advances play by dt (wall-clock seconds) and re-poses the model when
// it was playing, so a paused model costs nothing per frame.
func (p *modelPicker) tick(dt float64) {
	m := &p.model
	if len(m.geosets) == 0 || m.seq == -1 {
		return
	}

	s := m.sequences[m.seq]
	start, end := int32(s.Start), int32(s.End)
	if end < start {
		end = start // not seen in wardata; keep playback sane
	}

	wasPlaying := p.play.playing
	p.play.advance(dt, start, end)
	if wasPlaying {
		m.setPose(m.seq, int32(math.Floor(p.play.frame)), uint32(uint64(p.play.global)))
	}
}

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
		case len(m.geosets) == 0:
			imgui.Text("No geosets")
		case m.seq == -1:
			imgui.Text("No sequences: bind pose")
		default:
			if imgui.ComboStrarr("sequence", &m.seq, m.seqLabels, int32(len(m.seqLabels))) {
				m.setPose(m.seq, int32(m.sequences[m.seq].Start), 0)
			}

			s := m.sequences[m.seq]
			start, end := int32(s.Start), int32(s.End)
			if end < start {
				end = start // not seen in wardata; keep the slider usable
			}

			frame := m.frame
			if imgui.SliderInt("frame", &frame, start, end) {
				m.setPose(m.seq, frame, 0)
			}
			if imgui.Button("start") {
				m.setPose(m.seq, start, 0)
			}
		}
	}
	imgui.End()
}
