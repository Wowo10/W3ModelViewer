// Package root parses the decoded text root file into entries.
//
// The root is a CRLF text file with one line per file:
//
//	path|ckey|tag|installPath
//
// path is a mod chain (e.g. "War3.w3mod:_HD.w3mod:") followed by the name of
// the file inside that chain, split at the last ':'; files with no mod chain
// (the launcher's install files, outside the archive) have no ':' in path.
// ckey is the 16-byte content key, as 32 lower-case hex chars.
//
// tag is the locale (e.g. "enUS") for files under a "_Locales/<locale>.w3mod"
// chain segment, the platform ("Windows" or "OSX") for install files, and
// empty otherwise. installPath is set only on install files, and equals path.
package root

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const keySize = 16

// Entry is one file listed in the root.
type Entry struct {
	Path        string   // full path, e.g. War3.w3mod:_HD.w3mod:Units/…/Footman.mdx
	ModChain    string   // mods the file belongs to, with the trailing ':'; "" for install files
	Name        string   // path inside the mod chain
	CKey        [16]byte // content key
	Tag         string   // locale (enUS…) or platform (Windows, OSX); see package comment
	InstallPath string   // where install files go on disk; "" for archive files
	Variant     Variant  // edition and overlays derived from ModChain; see edition.go
}

// Root is the parsed root file, searchable by full path.
type Root struct {
	Entries []Entry
	byPath  map[string]int // index into Entries
}

// ReadRoot parses a decoded root file into a Root.
func ReadRoot(data []byte) Root {
	content := strings.TrimSuffix(string(data), "\r\n")
	lines := strings.Split(content, "\n")

	r := Root{
		Entries: make([]Entry, 0, len(lines)),
		byPath:  make(map[string]int, len(lines)),
	}

	variants := make(map[string]Variant) // one ParseModChain call per mod chain

	for i, line := range lines {
		entry := parseLine(strings.TrimSuffix(line, "\r"), i+1)

		variant, cached := variants[entry.ModChain]
		if !cached {
			variant = ParseModChain(entry.ModChain)
			variants[entry.ModChain] = variant
		}
		entry.Variant = variant

		if _, exists := r.byPath[entry.Path]; exists {
			panic(fmt.Sprintf("root: duplicate path %q on line %d", entry.Path, i+1))
		}

		r.byPath[entry.Path] = len(r.Entries)
		r.Entries = append(r.Entries, entry)
	}

	return r
}

// parseLine parses one root line, panicking with lineNum if it is malformed.
func parseLine(line string, lineNum int) Entry {
	fields := strings.Split(line, "|")
	if len(fields) != 4 {
		panic(fmt.Sprintf("root: line %d has %d fields, want 4: %q", lineNum, len(fields), line))
	}

	path := fields[0]

	ckeyBytes, err := hex.DecodeString(fields[1])
	if err != nil || len(ckeyBytes) != keySize {
		panic(fmt.Sprintf("root: line %d has a bad content key: %q", lineNum, fields[1]))
	}
	var ckey [keySize]byte
	copy(ckey[:], ckeyBytes)

	var modChain, name string
	if idx := strings.LastIndexByte(path, ':'); idx >= 0 {
		modChain, name = path[:idx+1], path[idx+1:]
	} else {
		name = path
	}

	return Entry{
		Path:        path,
		ModChain:    modChain,
		Name:        name,
		CKey:        ckey,
		Tag:         fields[2],
		InstallPath: fields[3],
	}
}

// Lookup finds the entry for a full path.
func (r Root) Lookup(path string) (Entry, bool) {
	idx, ok := r.byPath[path]
	if !ok {
		return Entry{}, false
	}
	return r.Entries[idx], true
}

// Variants returns every entry whose Name (the path inside the mod chain)
// equals name case-insensitively, in root order; the game's file system is
// case-insensitive, and the same name can appear with different case in
// different chains.
func (r Root) Variants(name string) []Entry {
	var variants []Entry
	for _, entry := range r.Entries {
		if strings.EqualFold(entry.Name, name) {
			variants = append(variants, entry)
		}
	}
	return variants
}
