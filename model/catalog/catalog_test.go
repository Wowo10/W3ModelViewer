package catalog

import (
	"encoding/binary"
	"testing"

	"modelView/model/mdx"
)

// buildMDX assembles a minimal MDX file: magic, a VERS 1800 chunk, then one
// chunk per entry in chunks, in order.
func buildMDX(chunks map[string][]byte) []byte {
	data := []byte("MDLX")

	vers := make([]byte, 4)
	binary.LittleEndian.PutUint32(vers, 1800)
	data = appendChunk(data, "VERS", vers)

	for tag, body := range chunks {
		data = appendChunk(data, tag, body)
	}

	return data
}

func appendChunk(data []byte, tag string, body []byte) []byte {
	data = append(data, []byte(tag)...)
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(body)))
	data = append(data, size...)
	return append(data, body...)
}

// record builds one inclusive-size record: size, then size-4 filler bytes.
// topByte is or'd into the size field's top byte (for CAMS).
func record(size int, topByte byte) []byte {
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b, uint32(size))
	b[3] |= topByte
	return b
}

func TestInspectRecordCounts(t *testing.T) {
	var pre2 []byte
	pre2 = append(pre2, record(12, 0)...)
	pre2 = append(pre2, record(20, 0)...)

	var corn []byte
	corn = append(corn, record(16, 0)...)
	corn = append(corn, record(16, 0)...)
	corn = append(corn, record(16, 0)...)

	data := buildMDX(map[string][]byte{"PRE2": pre2, "CORN": corn})
	info := Inspect(mdx.Read(data))

	if info.Geosets != 0 {
		t.Errorf("Geosets = %d, want 0", info.Geosets)
	}
	if info.Emitters != 2 {
		t.Errorf("Emitters = %d, want 2", info.Emitters)
	}
	if info.PopcornFX != 3 {
		t.Errorf("PopcornFX = %d, want 3", info.PopcornFX)
	}
}

func TestInspectCamsTopByte(t *testing.T) {
	var cams []byte
	cams = append(cams, record(696, 0x03)...) // top byte set, as seen in wardata
	cams = append(cams, record(300, 0x01)...)

	data := buildMDX(map[string][]byte{"CAMS": cams})
	info := Inspect(mdx.Read(data))

	if info.Cameras != 2 {
		t.Errorf("Cameras = %d, want 2", info.Cameras)
	}
}

func TestInspectRecordPastChunkEndPanics(t *testing.T) {
	bad := record(20, 0)
	bad = bad[:16] // chunk ends before the declared size

	data := buildMDX(map[string][]byte{"RIBB": bad})

	defer func() {
		if recover() == nil {
			t.Error("Inspect did not panic on a record running past the chunk end")
		}
	}()
	Inspect(mdx.Read(data))
}

func TestExplain(t *testing.T) {
	if got := (Info{Geosets: 3, PopcornFX: 1}).Explain(); got != "" {
		t.Errorf("Explain(with geosets) = %q, want \"\"", got)
	}

	if got := (Info{}).Explain(); got != "No geometry and nothing else to show" {
		t.Errorf("Explain(empty) = %q", got)
	}

	got := Info{PopcornFX: 3, Emitters: 1}.Explain()
	want := "No geometry: 3 PopcornFX emitters (CORN), 1 particle emitter (PRE2)"
	if got != want {
		t.Errorf("Explain(mixed) = %q, want %q", got, want)
	}
}
