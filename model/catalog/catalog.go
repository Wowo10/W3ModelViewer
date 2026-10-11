// Package catalog records, for each distinct MDX content key, whether it has
// geometry to draw and what else it holds, so the viewers can hide and
// explain models with no geosets (e.g. particle effects, which hold only
// emitters and helpers).
//
// Inspect reads a parsed mdx.Model's geoset, LOD, bone and helper counts
// directly, and walks four more chunks tag by tag: PRE2 (classic particle
// emitters), CORN (PopcornFX emitters), RIBB (ribbon emitters), LITE
// (lights) and ATCH (attachment points) are each an array of records packed
// back to back, each starting with its own uint32 inclusive size (covering
// the whole record, size field included); CAMS records carry the same
// layout but with an unexplained top byte set in the size field, so its
// size is masked to the low 24 bits before use (empirically exact on every
// CAMS chunk seen in wardata). A record whose size runs past its chunk's
// end is a panic, like the other mdx parsers.
package catalog

import (
	"encoding/binary"
	"fmt"
	"strings"

	"modelView/model/mdx"
)

// Info is what Inspect records for one MDX content key.
type Info struct {
	Geosets int
	LODs    int

	Emitters    int // PRE2, classic particle emitters
	PopcornFX   int // CORN, the HD particle system
	Ribbons     int // RIBB
	Lights      int // LITE
	Attachments int // ATCH
	Cameras     int // CAMS
	Bones       int // BONE
	Helpers     int // HELP
}

// Inspect builds an Info from a parsed model.
func Inspect(m mdx.Model) Info {
	geosets := m.Geosets()
	info := Info{
		Geosets: len(geosets),
		LODs:    len(mdx.LODTotals(geosets)),
		Bones:   len(m.Bones()),
		Helpers: len(m.Helpers()),
	}

	for _, c := range m.Chunks {
		switch c.Tag {
		case "PRE2":
			info.Emitters = countRecords(c)
		case "CORN":
			info.PopcornFX = countRecords(c)
		case "RIBB":
			info.Ribbons = countRecords(c)
		case "LITE":
			info.Lights = countRecords(c)
		case "ATCH":
			info.Attachments = countRecords(c)
		case "CAMS":
			info.Cameras = countRecordsMasked(c, 0x00FFFFFF)
		}
	}

	return info
}

// countRecords walks a chunk of back-to-back records each starting with a
// uint32 inclusive size, and returns how many there are. It panics
// ("catalog: <tag> ...") on a record smaller than its size field or running
// past the chunk end.
func countRecords(c mdx.Chunk) int {
	return countRecordsMasked(c, 0xFFFFFFFF)
}

// countRecordsMasked is countRecords with the size field masked before use,
// for CAMS's unexplained top byte.
func countRecordsMasked(c mdx.Chunk, mask uint32) int {
	data := c.Data
	count, pos := 0, 0
	for pos < len(data) {
		if pos+4 > len(data) {
			panic(fmt.Sprintf("catalog: %s record %d: truncated size field", c.Tag, count))
		}
		size := binary.LittleEndian.Uint32(data[pos:pos+4]) & mask
		end := pos + int(size)
		if size < 4 || end > len(data) {
			panic(fmt.Sprintf("catalog: %s record %d: runs past chunk end", c.Tag, count))
		}
		pos = end
		count++
	}
	return count
}

// explainPart is one non-zero count Explain may mention.
type explainPart struct {
	count    int
	singular string // plural is singular+"s"
	tag      string
}

// Explain returns "" when the model has geometry, else a short description
// of what it holds instead, e.g. "No geometry: 3 PopcornFX emitters (CORN),
// 1 particle emitter (PRE2)", or "No geometry and nothing else to show" when
// it has none of the counted chunks either.
func (i Info) Explain() string {
	if i.Geosets > 0 {
		return ""
	}

	parts := []explainPart{
		{i.PopcornFX, "PopcornFX emitter", "CORN"},
		{i.Emitters, "particle emitter", "PRE2"},
		{i.Ribbons, "ribbon emitter", "RIBB"},
		{i.Lights, "light", "LITE"},
		{i.Cameras, "camera", "CAMS"},
		{i.Attachments, "attachment point", "ATCH"},
		{i.Bones, "bone", "BONE"},
		{i.Helpers, "helper", "HELP"},
	}

	var found []string
	for _, p := range parts {
		if p.count == 0 {
			continue
		}
		noun := p.singular
		if p.count != 1 {
			noun += "s"
		}
		found = append(found, fmt.Sprintf("%d %s (%s)", p.count, noun, p.tag))
	}

	if len(found) == 0 {
		return "No geometry and nothing else to show"
	}
	return "No geometry: " + strings.Join(found, ", ")
}
