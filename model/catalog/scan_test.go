package catalog

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"modelView/extract/buildConfig"
	"modelView/extract/buildinfo"
	"modelView/extract/casc"
	"modelView/extract/root"
)

// TestMain chdirs to the repo root (go test runs in the package dir),
// matching web's TestMain, so wardata-relative paths resolve the same way.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Println("catalog: chdir ../..:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func ckey(b byte) [16]byte {
	var k [16]byte
	k[0] = b
	return k
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog-abc.json")

	c := &Catalog{infos: map[[16]byte]Info{
		ckey(1): {Geosets: 3},
		ckey(2): {PopcornFX: 1},
	}}
	c.save(path, "abc")

	want := []byte{1, 2}
	got, ok := load(path, "abc", [][16]byte{ckey(1), ckey(2)})
	if !ok {
		t.Fatal("load after save: ok = false")
	}
	for _, b := range want {
		if got[ckey(b)] != c.infos[ckey(b)] {
			t.Errorf("load()[%d] = %v, want %v", b, got[ckey(b)], c.infos[ckey(b)])
		}
	}
}

func TestLoadIgnoresBadFiles(t *testing.T) {
	dir := t.TempDir()
	want := [][16]byte{ckey(1)}

	if _, ok := load(filepath.Join(dir, "missing.json"), "abc", want); ok {
		t.Error("load(missing file) = true")
	}

	garbage := filepath.Join(dir, "garbage.json")
	os.WriteFile(garbage, []byte("not json"), 0o644)
	if _, ok := load(garbage, "abc", want); ok {
		t.Error("load(garbage) = true")
	}

	wrongFormat := filepath.Join(dir, "wrongformat.json")
	os.WriteFile(wrongFormat, []byte(`{"format":2,"build":"abc","models":{}}`), 0o644)
	if _, ok := load(wrongFormat, "abc", want); ok {
		t.Error("load(wrong format) = true")
	}

	wrongBuild := filepath.Join(dir, "wrongbuild.json")
	os.WriteFile(wrongBuild, []byte(`{"format":1,"build":"xyz","models":{}}`), 0o644)
	if _, ok := load(wrongBuild, "abc", want); ok {
		t.Error("load(wrong build) = true")
	}

	missingKey := filepath.Join(dir, "missingkey.json")
	os.WriteFile(missingKey, []byte(`{"format":1,"build":"abc","models":{}}`), 0o644)
	if _, ok := load(missingKey, "abc", want); ok {
		t.Error("load(missing a wanted key) = true")
	}
}

func TestLoadAllowsUnstoredKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog-abc.json")

	c := &Catalog{
		infos:    map[[16]byte]Info{ckey(1): {Geosets: 3}},
		unstored: map[[16]byte]bool{ckey(2): true},
	}
	c.save(path, "abc")

	got, ok := load(path, "abc", [][16]byte{ckey(1), ckey(2)})
	if !ok {
		t.Fatal("load: ok = false, want true (key 2 is recorded as unstored)")
	}
	if got[ckey(1)] != c.infos[ckey(1)] {
		t.Errorf("load()[1] = %v, want %v", got[ckey(1)], c.infos[ckey(1)])
	}
	if _, ok := got[ckey(2)]; ok {
		t.Error("load()[2] present, want absent: an unstored key stays unknown to Lookup")
	}
}

// TestScanWardata runs a real scan over a handful of .mdx entries, including
// a known no-geometry model, then checks a second Start on the same cache
// directory reports FromCache. It is skipped when wardata isn't present.
func TestScanWardata(t *testing.T) {
	if _, err := os.Stat("wardata/.build.info"); err != nil {
		t.Skipf("catalog: skipping, wardata/.build.info not found: %v", err)
	}

	storage, fullRoot := openTestStorage(t)
	defer storage.Close()

	const smokeBlack = "War3.w3mod:_DE.w3mod:SharedFX/Building_Smoke_large/SmokeBlackLarge.mdx"
	const footman = "War3.w3mod:Units/Human/Footman/Footman.mdx"

	r := filteredRoot(fullRoot, smokeBlack, footman)

	dir := t.TempDir()
	c := Start(storage, r, "test-build", dir)

	deadline := time.After(30 * time.Second)
	for {
		if _, _, finished := c.Progress(); finished {
			break
		}
		select {
		case <-deadline:
			t.Fatal("scan did not finish in time")
		case <-time.After(10 * time.Millisecond):
		}
	}

	smokeEntry, ok := fullRoot.Lookup(smokeBlack)
	if !ok {
		t.Fatal("SmokeBlackLarge not found in root")
	}
	smokeInfo, ok := c.Lookup(smokeEntry.CKey)
	if !ok {
		t.Fatal("SmokeBlackLarge not in catalog")
	}
	if smokeInfo.Geosets != 0 || smokeInfo.PopcornFX != 1 {
		t.Errorf("SmokeBlackLarge Info = %+v, want Geosets 0, PopcornFX 1", smokeInfo)
	}

	footmanEntry, ok := fullRoot.Lookup(footman)
	if !ok {
		t.Fatal("Footman not found in root")
	}
	footmanInfo, ok := c.Lookup(footmanEntry.CKey)
	if !ok {
		t.Fatal("Footman not in catalog")
	}
	if footmanInfo.Geosets == 0 {
		t.Error("Footman Info.Geosets = 0, want > 0")
	}

	c2 := Start(storage, r, "test-build", dir)
	for {
		if _, _, finished := c2.Progress(); finished {
			break
		}
		select {
		case <-deadline:
			t.Fatal("second scan did not finish in time")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !c2.FromCache() {
		t.Error("second Start: FromCache() = false, want true")
	}
}

// filteredRoot returns a root.Root over fullRoot's .mdx entries under
// "SharedFX/" (a few hundred, exercising the scan's workers), plus the
// given extra full paths (e.g. Footman, outside SharedFX).
func filteredRoot(fullRoot root.Root, extra ...string) root.Root {
	want := map[string]bool{}
	for _, p := range extra {
		want[p] = true
	}

	r := root.Root{}
	for _, e := range fullRoot.Entries {
		if want[e.Path] || strings.HasPrefix(e.Name, "SharedFX/") {
			r.Entries = append(r.Entries, e)
		}
	}
	return r
}

// openTestStorage mirrors web.openTestStorage: main.go's start-up with
// verify off, for fast test runs against real wardata.
func openTestStorage(t *testing.T) (*casc.Storage, root.Root) {
	t.Helper()

	biData, err := os.ReadFile("wardata/.build.info")
	if err != nil {
		t.Fatal(err)
	}
	bInfo := buildinfo.ReadBuildInfo(biData)

	key := bInfo.GetBuildKey()
	cfgData, err := os.ReadFile("wardata/Data/config/" + key[0:2] + "/" + key[2:4] + "/" + key)
	if err != nil {
		t.Fatal(err)
	}
	config := buildConfig.ReadBuildConfig(cfgData)
	ckeyHex, ekeyHex := config.GetEncoding()

	storage := casc.Open("wardata", false)

	ekey := decodeHex(t, ekeyHex)
	encPkg, ok := storage.LookupEKey(ekey)
	if !ok {
		t.Fatal("catalog: encoding package not found")
	}
	storage.LoadEncoding(encPkg, [16]byte(decodeHex(t, ckeyHex)))

	rootCKey := [16]byte(decodeHex(t, config.GetRoot()))
	r := root.ReadRoot(storage.ReadByCKey(rootCKey))
	return storage, r
}

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
