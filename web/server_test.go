package web

import (
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"modelView/extract/buildConfig"
	"modelView/extract/buildinfo"
	"modelView/extract/casc"
	"modelView/extract/root"
)

// Footman's three editions, as listed in the task's roadmap notes.
const (
	classicFootman    = "War3.w3mod:Units/Human/Footman/Footman.mdx"
	reforgedFootman   = "War3.w3mod:_HD.w3mod:Units/Human/Footman/Footman.mdx"
	definitiveFootman = "War3.w3mod:_DE.w3mod:Units/Human/Footman/Footman.mdx"
)

var (
	ts  *httptest.Server
	srv *Server
)

// TestMain chdirs to the repo root (go test runs in the package dir) and
// skips the package when wardata isn't present, else opens storage with
// verify off (mirroring main.go's start-up) and shares one Server +
// httptest.Server across every test.
func TestMain(m *testing.M) {
	if err := os.Chdir(".."); err != nil {
		fmt.Println("web: chdir ..:", err)
		os.Exit(1)
	}

	if _, err := os.Stat("wardata/.build.info"); err != nil {
		fmt.Println("web: skipping, wardata/.build.info not found:", err)
		os.Exit(0)
	}

	storage, r := openTestStorage()
	defer storage.Close()

	srv = New(storage, r)
	ts = httptest.NewServer(srv.Handler())
	defer ts.Close()

	os.Exit(m.Run())
}

// openTestStorage mirrors main.go's start-up (readBuildInfo through
// readRoot) with verify off, for fast test runs against real wardata.
func openTestStorage() (*casc.Storage, root.Root) {
	biData, err := os.ReadFile("wardata/.build.info")
	if err != nil {
		panic(err)
	}
	bInfo := buildinfo.ReadBuildInfo(biData)

	key := bInfo.GetBuildKey()
	cfgData, err := os.ReadFile("wardata/Data/config/" + key[0:2] + "/" + key[2:4] + "/" + key)
	if err != nil {
		panic(err)
	}
	config := buildConfig.ReadBuildConfig(cfgData)
	ckeyHex, ekeyHex := config.GetEncoding()

	storage := casc.Open("wardata", false)

	ekey := decodeHex(ekeyHex)
	encPkg, ok := storage.LookupEKey(ekey)
	if !ok {
		panic("web: encoding package not found")
	}
	storage.LoadEncoding(encPkg, [16]byte(decodeHex(ckeyHex)))

	rootCKey := [16]byte(decodeHex(config.GetRoot()))
	r := root.ReadRoot(storage.ReadByCKey(rootCKey))
	return storage, r
}

func decodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getBody(t *testing.T, path string) []byte {
	t.Helper()
	resp := get(t, path)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// getFrom is getBody against a caller-supplied httptest.Server, for tests
// that need their own Server/cache rather than the shared one.
func getFrom(t *testing.T, ts *httptest.Server, path string) []byte {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
