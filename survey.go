package main

import (
	"fmt"
	"io"
	"modelView/extract/casc"
	"modelView/extract/root"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	surveyHexRe = regexp.MustCompile(`[0-9a-f]{16,}`)
	surveyNumRe = regexp.MustCompile(`\d{2,}`)
)

// surveyGroup counts occurrences of a category and keeps up to 3 example
// paths, the first encountered for each.
type surveyGroup struct {
	count    int
	examples []string
}

func (g *surveyGroup) add(path string) {
	g.count++
	if len(g.examples) < 3 {
		g.examples = append(g.examples, path)
	}
}

// surveyFiles reads every distinct content key in the root once, recovering
// from per-file panics, and prints totals to stdout: ok (with decoded
// bytes), missing from encoding, missing from idx (grouped by edition/locale
// overlay) and failed decode (grouped by normalised panic message). recover
// cannot catch fatal runtime errors (e.g. out of memory), which would still
// stop the survey.
func surveyFiles(storage *casc.Storage, r root.Root) {
	seen := make(map[[16]byte]bool, len(r.Entries))

	okCount, okBytes := 0, 0
	missingEncoding := &surveyGroup{}
	missingIdx := make(map[string]*surveyGroup)
	failed := make(map[string]*surveyGroup)

	distinct := 0
	for _, entry := range r.Entries {
		if seen[entry.CKey] {
			continue
		}
		seen[entry.CKey] = true
		distinct++

		if distinct%10000 == 0 {
			fmt.Fprintf(os.Stderr, "surveyed %d\n", distinct)
		}

		encEntry, exists := storage.LookupCKey(entry.CKey)
		if !exists {
			missingEncoding.add(entry.Path)
			continue
		}

		ekey := encEntry.EKeys[0]
		if _, exists := storage.LookupEKey(ekey[:]); !exists {
			group(missingIdx, surveyIdxGroupKey(entry)).add(entry.Path)
			continue
		}

		n, msg, ok := surveyDecode(storage, entry.CKey)
		if ok {
			okCount++
			okBytes += n
		} else {
			group(failed, msg).add(entry.Path)
		}
	}

	w := os.Stdout
	fmt.Fprintln(w, "Distinct content keys:", distinct)
	fmt.Fprintf(w, "OK: %d (%d bytes decoded)\n", okCount, okBytes)
	printSurveyGroup(w, "Missing from encoding", missingEncoding)
	printSurveyGroups(w, "Missing from idx", missingIdx)
	printSurveyGroups(w, "Failed decode", failed)
}

// group returns the named group in groups, creating it if necessary.
func group(groups map[string]*surveyGroup, name string) *surveyGroup {
	g, exists := groups[name]
	if !exists {
		g = &surveyGroup{}
		groups[name] = g
	}
	return g
}

// surveyIdxGroupKey names the overlay or edition that explains why entry's
// encoding key is missing from the local idx: "install" for install files,
// its locale overlay (e.g. "locale:deDE") for non-installed locales, else its
// edition.
func surveyIdxGroupKey(entry root.Entry) string {
	if entry.Variant.Edition == root.Install {
		return "install"
	}
	for _, overlay := range entry.Variant.Overlays {
		if strings.HasPrefix(overlay, "locale:") {
			return overlay
		}
	}
	return string(entry.Variant.Edition)
}

// surveyDecode attempts to read and decode ckey, recovering from a panic.
// On success it returns the decoded size and ok=true; on panic it returns
// the normalised panic message and ok=false.
func surveyDecode(storage *casc.Storage, ckey [16]byte) (n int, msg string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			msg = normalizeSurveyMessage(fmt.Sprint(r))
			ok = false
		}
	}()

	data := storage.ReadByCKey(ckey)
	return len(data), "", true
}

// normalizeSurveyMessage replaces keys and counts in a panic message with
// placeholders so similar failures group together, e.g. "Unsupported BLTE
// chunk mode: 4" keeps its mode digit since it is a single digit, while
// "chunk 12 MD5 mismatch" becomes "chunk <n> MD5 mismatch".
func normalizeSurveyMessage(msg string) string {
	msg = surveyHexRe.ReplaceAllString(msg, "<key>")
	msg = surveyNumRe.ReplaceAllString(msg, "<n>")
	return msg
}

// printSurveyGroup prints a single, ungrouped count with its example paths.
func printSurveyGroup(w io.Writer, label string, g *surveyGroup) {
	fmt.Fprintf(w, "%s: %d\n", label, g.count)
	for _, path := range g.examples {
		fmt.Fprintf(w, "  %s\n", path)
	}
}

// printSurveyGroups prints label's total followed by its named groups,
// sorted by count descending then name, each with its example paths
// indented beneath.
func printSurveyGroups(w io.Writer, label string, groups map[string]*surveyGroup) {
	names := make([]string, 0, len(groups))
	total := 0
	for name, g := range groups {
		names = append(names, name)
		total += g.count
	}
	sort.Slice(names, func(i, j int) bool {
		gi, gj := groups[names[i]], groups[names[j]]
		if gi.count != gj.count {
			return gi.count > gj.count
		}
		return names[i] < names[j]
	})

	fmt.Fprintf(w, "%s: %d\n", label, total)
	for _, name := range names {
		g := groups[name]
		fmt.Fprintf(w, "  %6d %s\n", g.count, name)
		for _, path := range g.examples {
			fmt.Fprintf(w, "    %s\n", path)
		}
	}
}
