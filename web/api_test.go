package web

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"modelView/model/mdx"
)

// footmanCases lists one test case per Footman edition: the root path and
// the -mdxinfo counts recorded in this task's roadmap notes.
var footmanCases = []struct {
	name          string
	path          string
	geosets       int
	vertices      int
	triangles     int
	objects       int
	sequences     int
	defaultSeq    string
	meshSize      int
	lod0Vertices  int // 0 = same as vertices (single-LOD models)
	lod0Triangles int
}{
	{
		name: "classic", path: classicFootman,
		geosets: 5, vertices: 582, triangles: 372, objects: 57,
		sequences: 13, defaultSeq: "Stand - 1", meshSize: 58108,
	},
	{
		name: "reforged", path: reforgedFootman,
		geosets: 36, vertices: 40142, triangles: 53211, objects: 134,
		sequences: 27, defaultSeq: "Stand 1", meshSize: 1456532,
		lod0Vertices: 13400, lod0Triangles: 19015,
	},
	{
		name: "definitive", path: definitiveFootman,
		geosets: 8, vertices: 7488, triangles: 11336, objects: 134,
		sequences: 27, defaultSeq: "Stand 1", meshSize: 786864,
	},
}

func TestModels(t *testing.T) {
	var list modelListResponse
	if err := json.Unmarshal(getBody(t, "/api/models"), &list); err != nil {
		t.Fatal(err)
	}

	if len(list.Models) != 14984 {
		t.Errorf("models: got %d, want 14984", len(list.Models))
	}
	for i := 1; i < len(list.Models); i++ {
		if list.Models[i].Path < list.Models[i-1].Path {
			t.Fatalf("models not sorted by path at %d: %q < %q", i, list.Models[i].Path, list.Models[i-1].Path)
		}
	}
	if len(list.Team) != 25 {
		t.Errorf("team: got %d entries, want 25", len(list.Team))
	}

	want := map[string]bool{classicFootman: false, reforgedFootman: false, definitiveFootman: false}
	for _, m := range list.Models {
		if _, ok := want[m.Path]; ok {
			want[m.Path] = true
			if m.Overlays == nil {
				t.Errorf("%s: overlays is nil, want []", m.Path)
			}
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("models: missing %s", path)
		}
	}
}

func TestModel(t *testing.T) {
	for _, c := range footmanCases {
		t.Run(c.name, func(t *testing.T) {
			var m modelResponse
			if err := json.Unmarshal(getBody(t, "/api/model?path="+url.QueryEscape(c.path)), &m); err != nil {
				t.Fatal(err)
			}

			if m.Version != 1800 {
				t.Errorf("version: got %d, want 1800", m.Version)
			}
			if len(m.Geosets) != c.geosets {
				t.Errorf("geosets: got %d, want %d", len(m.Geosets), c.geosets)
			}
			if m.Vertices != c.vertices {
				t.Errorf("vertices: got %d, want %d", m.Vertices, c.vertices)
			}
			if m.Triangles != c.triangles {
				t.Errorf("triangles: got %d, want %d", m.Triangles, c.triangles)
			}
			if m.Objects != c.objects {
				t.Errorf("objects: got %d, want %d", m.Objects, c.objects)
			}
			if len(m.Sequences) != c.sequences {
				t.Errorf("sequences: got %d, want %d", len(m.Sequences), c.sequences)
			}
			if m.DefaultSequence < 0 || int(m.DefaultSequence) >= len(m.Sequences) {
				t.Fatalf("defaultSequence: got %d, out of range for %d sequences", m.DefaultSequence, len(m.Sequences))
			}
			if got := m.Sequences[m.DefaultSequence].Name; got != c.defaultSeq {
				t.Errorf("default sequence: got %q, want %q", got, c.defaultSeq)
			}

			if len(m.Editions) != 4 {
				t.Errorf("editions: got %d, want 4", len(m.Editions))
			}

			if m.Mesh.Size != c.meshSize {
				t.Errorf("mesh size: got %d, want %d", m.Mesh.Size, c.meshSize)
			}
			mesh := getBody(t, "/api/mesh?path="+url.QueryEscape(c.path))
			if len(mesh) != m.Mesh.Size {
				t.Errorf("mesh body length %d != mesh.size %d", len(mesh), m.Mesh.Size)
			}

			lodVertices, lodTriangles := 0, 0
			for i, g := range m.Geosets {
				if g.Mesh == nil {
					continue
				}
				lodVertices += g.Vertices
				lodTriangles += g.Triangles

				checkOffset(t, i, "positions", g.Mesh.Positions, 12*g.Vertices, len(mesh))
				checkOffset(t, i, "normals", g.Mesh.Normals, 12*g.Vertices, len(mesh))
				for u, off := range g.Mesh.UVs {
					checkOffset(t, i, "uv"+string(rune('0'+u)), off, 8*g.Vertices, len(mesh))
				}
				checkOffset(t, i, "weights", g.Mesh.Weights, 4*mdx.MaxInfluences*g.Vertices, len(mesh))
				checkOffset(t, i, "nodes", g.Mesh.Nodes, 4*mdx.MaxInfluences*g.Vertices, len(mesh))
			}

			wantLodV, wantLodT := c.lod0Vertices, c.lod0Triangles
			if wantLodV == 0 {
				wantLodV, wantLodT = c.vertices, c.triangles
			}
			if lodVertices != wantLodV {
				t.Errorf("lod vertices: got %d, want %d", lodVertices, wantLodV)
			}
			if lodTriangles != wantLodT {
				t.Errorf("lod triangles: got %d, want %d", lodTriangles, wantLodT)
			}
		})
	}
}

func checkOffset(t *testing.T, geoset int, field string, offset, size, bufLen int) {
	t.Helper()
	if offset < 0 || offset+size > bufLen {
		t.Errorf("geoset %d %s: offset %d size %d out of bounds (buffer %d)", geoset, field, offset, size, bufLen)
	}
}

// TestMeshVerticesMatchDirect decodes a few vertices of reforged Footman's
// first lod-0 geoset from /api/mesh and compares them against Geosets()
// and Influences() read directly through storage.
func TestMeshVerticesMatchDirect(t *testing.T) {
	entry, ok := srv.root.Lookup(reforgedFootman)
	if !ok {
		t.Fatal("reforged Footman not found")
	}
	data := srv.storage.ReadByCKey(entry.CKey)
	model := mdx.Read(data)
	geosets := model.Geosets()
	lod := mdx.LowestLOD(geosets)

	var g mdx.Geoset
	var gi int
	for i, gs := range geosets {
		if gs.LOD == lod {
			g, gi = gs, i
			break
		}
	}

	var m modelResponse
	if err := json.Unmarshal(getBody(t, "/api/model?path="+url.QueryEscape(reforgedFootman)), &m); err != nil {
		t.Fatal(err)
	}
	mesh := getBody(t, "/api/mesh?path="+url.QueryEscape(reforgedFootman))

	off := m.Geosets[gi].Mesh
	if off == nil {
		t.Fatal("expected geoset to have mesh offsets")
	}

	influences := g.Influences()
	for v := 0; v < 3 && v < g.VertexCount(); v++ {
		for k := 0; k < 3; k++ {
			got := readFloat32(mesh, off.Positions+12*v+4*k)
			want := g.Positions[3*v+k]
			if got != want {
				t.Errorf("vertex %d position[%d]: got %v, want %v", v, k, got, want)
			}
		}
		for k := 0; k < mdx.MaxInfluences; k++ {
			gotNode := binary.LittleEndian.Uint32(mesh[off.Nodes+4*mdx.MaxInfluences*v+4*k:])
			wantNode := influences.Nodes[mdx.MaxInfluences*v+k]
			if gotNode != wantNode {
				t.Errorf("vertex %d node[%d]: got %d, want %d", v, k, gotNode, wantNode)
			}
			gotWeight := readFloat32(mesh, off.Weights+4*mdx.MaxInfluences*v+4*k)
			wantWeight := influences.Weights[mdx.MaxInfluences*v+k]
			if gotWeight != wantWeight {
				t.Errorf("vertex %d weight[%d]: got %v, want %v", v, k, gotWeight, wantWeight)
			}
		}
	}
}

func readFloat32(buf []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(buf[offset:]))
}

func TestTexture(t *testing.T) {
	for _, c := range footmanCases {
		var m modelResponse
		if err := json.Unmarshal(getBody(t, "/api/model?path="+url.QueryEscape(c.path)), &m); err != nil {
			t.Fatal(err)
		}

		seen := map[string]bool{}
		for _, g := range m.Geosets {
			for _, l := range g.Layers {
				if l.Texture != nil {
					checkDDS(t, l.Texture.CKey)
					seen[l.Texture.CKey] = true
				}
				if l.Mask != nil {
					checkDDS(t, l.Mask.CKey)
					seen[l.Mask.CKey] = true
				}
			}
		}
		t.Logf("%s Footman: %d distinct resolved textures", c.name, len(seen))
	}
}

func checkDDS(t *testing.T, ckeyHex string) {
	t.Helper()
	body := getBody(t, "/api/texture?ckey="+ckeyHex)
	if !bytes.HasPrefix(body, []byte("DDS ")) {
		t.Errorf("texture %s: doesn't start with \"DDS \"", ckeyHex)
	}
}

func TestPose(t *testing.T) {
	for _, c := range footmanCases {
		t.Run(c.name, func(t *testing.T) {
			var m modelResponse
			if err := json.Unmarshal(getBody(t, "/api/model?path="+url.QueryEscape(c.path)), &m); err != nil {
				t.Fatal(err)
			}
			seq := m.Sequences[m.DefaultSequence]

			entry, ok := srv.root.Lookup(c.path)
			if !ok {
				t.Fatal("entry not found")
			}
			data := srv.storage.ReadByCKey(entry.CKey)
			model := mdx.Read(data)
			skeleton := mdx.NewSkeleton(model)
			at := mdx.AnimTime{Sequence: mdx.Sequence{Name: seq.Name, Start: seq.Start, End: seq.End}, Frame: seq.Start, Global: 0}
			wantMatrices := skeleton.WorldMatrices(at)
			wantAlphas := mdx.GeosetAlphasAtTime(len(model.Geosets()), model.GeosetAnims(), model.GlobalSequences(), at, true)

			q := url.Values{
				"path":   {c.path},
				"seq":    {strconv.Itoa(int(m.DefaultSequence))},
				"frame":  {strconv.Itoa(int(seq.Start))},
				"global": {"0"},
			}
			body := getBody(t, "/api/pose?"+q.Encode())

			objCount := binary.LittleEndian.Uint32(body[0:])
			geosetCount := binary.LittleEndian.Uint32(body[4:])
			if int(objCount) != len(wantMatrices) {
				t.Fatalf("object count: got %d, want %d", objCount, len(wantMatrices))
			}
			if int(geosetCount) != len(m.Geosets) {
				t.Fatalf("geoset count: got %d, want %d", geosetCount, len(m.Geosets))
			}

			pos := 8
			for i, want := range wantMatrices {
				for k := 0; k < 16; k++ {
					got := readFloat32(body, pos)
					if got != want[k] {
						t.Errorf("object %d matrix[%d]: got %v, want %v", i, k, got, want[k])
					}
					pos += 4
				}
			}
			for i, want := range wantAlphas {
				got := readFloat32(body, pos)
				if got != want {
					t.Errorf("geoset %d alpha: got %v, want %v", i, got, want)
				}
				if got != m.Geosets[i].Alpha {
					t.Errorf("geoset %d alpha %v != /api/model's %v", i, got, m.Geosets[i].Alpha)
				}
				pos += 4
			}
		})
	}
}

func TestPoseNoSequence(t *testing.T) {
	q := url.Values{"path": {classicFootman}, "seq": {"-1"}, "frame": {"0"}, "global": {"0"}}
	body := getBody(t, "/api/pose?"+q.Encode())

	objCount := binary.LittleEndian.Uint32(body[0:])
	geosetCount := binary.LittleEndian.Uint32(body[4:])

	pos := 8
	for i := 0; i < int(objCount); i++ {
		for k := 0; k < 16; k++ {
			got := readFloat32(body, pos)
			want := float32(0)
			if k%5 == 0 { // diagonal of the 4x4 identity: indices 0, 5, 10, 15
				want = 1
			}
			if got != want {
				t.Errorf("object %d matrix[%d]: got %v, want identity", i, k, got)
			}
			pos += 4
		}
	}
	for i := 0; i < int(geosetCount); i++ {
		if got := readFloat32(body, pos); got != 1 {
			t.Errorf("geoset %d alpha: got %v, want 1", i, got)
		}
		pos += 4
	}
}

func TestCache(t *testing.T) {
	path := definitiveFootman
	getBody(t, "/api/model?path="+url.QueryEscape(path))

	var m modelResponse
	if err := json.Unmarshal(getBody(t, "/api/model?path="+url.QueryEscape(path)), &m); err != nil {
		t.Fatal(err)
	}
	seq := m.Sequences[m.DefaultSequence]
	q := url.Values{
		"path":   {path},
		"seq":    {strconv.Itoa(int(m.DefaultSequence))},
		"frame":  {strconv.Itoa(int(seq.Start))},
		"global": {"0"},
	}

	srv.readMu.Lock()
	before := srv.reads
	srv.readMu.Unlock()

	for i := 0; i < 50; i++ {
		getBody(t, "/api/pose?"+q.Encode())
	}

	srv.readMu.Lock()
	after := srv.reads
	srv.readMu.Unlock()

	if after != before {
		t.Errorf("reads: got %d, want %d (poses should hit the cache)", after, before)
	}
}

// TestCacheEviction uses its own Server (over the same storage and root)
// with a 1-byte cache bound, since the shared srv's cache is already
// warm with every Footman by the time this test runs.
func TestCacheEviction(t *testing.T) {
	s := New(srv.storage, srv.root)
	s.CacheBound = 1
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	getFrom(t, ts, "/api/model?path="+url.QueryEscape(classicFootman))
	classicEntry, _ := s.root.Lookup(classicFootman)
	if _, ok := s.cache.get(classicEntry.CKey); !ok {
		t.Fatal("classic Footman should be cached right after loading it")
	}

	getFrom(t, ts, "/api/model?path="+url.QueryEscape(reforgedFootman))

	if _, ok := s.cache.get(classicEntry.CKey); ok {
		t.Error("classic Footman should have been evicted by a 1-byte cache bound")
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name string
		path string
		want int
	}{
		{"unknown path", "/api/model?path=" + url.QueryEscape("War3.w3mod:nope.mdx"), 404},
		{"non-mdx path", "/api/model?path=" + url.QueryEscape("War3.w3mod:ReplaceableTextures/TeamColor/TeamColor00.dds"), 404},
		{"ckey of non-dds entry", "/api/texture?ckey=" + hexFootmanCKey(t), 404},
		{"all-zero ckey", "/api/texture?ckey=" + strings.Repeat("0", 32), 404},
		{"missing path", "/api/model", 400},
		{"seq not a number", "/api/pose?path=" + url.QueryEscape(classicFootman) + "&seq=x&frame=0&global=0", 400},
		{"seq -2", "/api/pose?path=" + url.QueryEscape(classicFootman) + "&seq=-2&frame=0&global=0", 400},
		{"seq too large", "/api/pose?path=" + url.QueryEscape(classicFootman) + "&seq=999&frame=0&global=0", 400},
		{"negative frame", "/api/pose?path=" + url.QueryEscape(classicFootman) + "&seq=0&frame=-1&global=0", 400},
		{"short ckey", "/api/texture?ckey=zz", 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := get(t, c.path)
			defer resp.Body.Close()
			if resp.StatusCode != c.want {
				t.Errorf("status: got %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
}

// hexFootmanCKey returns classic Footman's content key as hex: a valid
// 32-hex-char key that names a non-.dds entry.
func hexFootmanCKey(t *testing.T) string {
	t.Helper()
	entry, ok := srv.root.Lookup(classicFootman)
	if !ok {
		t.Fatal("classic Footman not found")
	}
	return hex.EncodeToString(entry.CKey[:])
}

func TestHostCheck(t *testing.T) {
	req, err := http.NewRequest("GET", ts.URL+"/api/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "example.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", resp.StatusCode)
	}
}

func TestCheckAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:8080", "[::1]:80"} {
		t.Run("ok "+addr, func(t *testing.T) {
			CheckAddr(addr) // must not panic
		})
	}

	for _, addr := range []string{":8080", "0.0.0.0:8080", "192.168.1.2:8080"} {
		t.Run("panics "+addr, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("CheckAddr(%q) did not panic", addr)
				}
			}()
			CheckAddr(addr)
		})
	}
}
