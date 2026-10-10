// Package web serves a local HTTP API with everything the desktop viewer
// takes from model/mdx and model/texture, plus the embedded static page, so
// a browser page never parses MDX or CASC itself.
//
// All JSON is camelCase; content keys are 32 lower-case hex chars;
// "overlays" is always [] rather than null. Binary bodies are
// little-endian, every section 4-byte aligned so the page can view them
// with typed arrays.
//
//   - GET /api/models -> {"models": [{"path","name","edition","overlays"}...],
//     "team": [{"color": TextureRef, "glow": TextureRef}...]}; models sorted
//     by path like the desktop picker (every root entry whose Name ends in
//     ".mdx", case-insensitively); team[i] is player i, 0-24.
//
//   - GET /api/model?path= -> {"path","name","edition","overlays","ckey",
//     "version","chunks","objects","lod","vertices","triangles",
//     "geosets":[Geoset...],"sequences":[{"name","key","start","end"}...],
//     "defaultSequence","globalSequences":[...],"editions":[{"path",
//     "edition","overlays"}...],"mesh":{"url","size"}}.
//     objects = len(Pivots()), the pose's matrix count; lod = the lowest
//     LOD (the one in the mesh); vertices/triangles = totals over every
//     geoset. Geoset: {"material","lod","vertices","triangles","priority",
//     "alpha","layers":[Layer...],"mesh": MeshOffsets|null} - alpha at the
//     default sequence's start frame, global clock 0 (1 without
//     sequences); mesh is null for geosets not at lod. MeshOffsets: byte
//     offsets into the mesh buffer, {"positions","normals",
//     "uvs":[...one per UV set],"weights","nodes","indices"}; lengths
//     follow from vertices/triangles. Layer: {"filter" (mdx.FilterMode
//     number),"shading","alpha","uvSet","wrapWidth","wrapHeight","source"
//     ("texture"|"teamColor"|"teamGlow"|"grey"),"label","texture":
//     TextureRef|null,"mask": TextureRef|null}; mask is the ORM when slot 4
//     is replaceable 1 and slot 2 resolves - the page applies it only when
//     the DDS is DXT5. A geoset with no layers draws as one opaque grey
//     layer, left to the page. TextureRef: {"path","ckey","url"} (url =
//     /api/texture?ckey=). sequences[i].key = mdx.SequenceKey(name);
//     defaultSequence is an index, -1 when there are none; editions is
//     root.Variants(name) sorted by root.SortVariants, including this
//     model.
//
//   - GET /api/mesh?path= -> application/octet-stream, per lowest-LOD
//     geoset in geoset order: positions float32[3v], normals float32[3v]
//     (zeros when the geoset has none), each UV set float32[2v], weights
//     float32[8v], node object ids uint32[8v] (Influences(),
//     mdx.MaxInfluences = 8 per vertex), indices uint16[3t] padded to 4
//     bytes.
//
//   - GET /api/texture?ckey= -> the decoded .dds file bytes, with
//     Cache-Control: max-age=31536000, immutable (content-addressed). Only
//     content keys of root entries whose Name ends in ".dds".
//
//   - GET /api/pose?path=&seq=&frame=&global= -> application/octet-stream:
//     uint32 object count, uint32 geoset count, then object count x 16
//     float32 world matrices (column-major mdx.Mat4, indexed by object
//     id), then geoset count x float32 alphas. seq is an index into
//     sequences or -1 (bind pose: identity matrices, alpha 1); frame
//     (absolute, clamped by the sampler) and global are uint32. All three
//     are required.
//
// Errors: missing/unparseable parameter, seq outside [-1, len(sequences)),
// a ckey that isn't 32 hex chars -> 400; path that isn't a listed .mdx
// entry, ckey that isn't a .dds entry's, or content not stored locally ->
// 404 (plain-text body). Wrong method -> 405 (ServeMux). Model parse
// panics stay panics: net/http recovers them, logs, and drops the
// connection.
//
// Requests whose Host isn't localhost or a loopback IP get 403 (a cheap
// guard against DNS rebinding, since the data must never leave the
// machine).
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/texture"
)

//go:embed static
var staticFS embed.FS

// defaultCacheBound is the cache's default byte bound (sum of mesh buffer
// lengths), about 10x the largest model seen in wardata (Cinematics'
// ArthasArrives, ~23 MB).
const defaultCacheBound = 256 << 20

// Server serves the model list and model data from an opened CASC storage
// and root over HTTP.
type Server struct {
	storage  *casc.Storage
	root     root.Root
	resolver *texture.Resolver

	models []root.Entry      // every .mdx root entry, sorted by path
	byPath map[string]int    // models[byPath[path]].Path == path
	ddsSet map[[16]byte]bool // content keys of root entries whose Name ends in .dds
	team   []teamTextureRefs // team[i] is player i, 0-24

	readMu sync.Mutex // serializes storage.ReadByCKey, which races on concurrent reads
	reads  int64      // bumped under readMu, for tests and the requests/s notes

	cache      *modelCache
	CacheBound int64 // byte bound for cache, a Server field so tests can shrink it
}

// teamTextureRefs is one player's team colour and glow texture references.
type teamTextureRefs struct {
	Color textureRef `json:"color"`
	Glow  textureRef `json:"glow"`
}

// New builds a Server over an opened storage and root: every .mdx root
// entry sorted by path, the set of .dds content keys, and the 25 standard
// players' team colour/glow texture references (War3.w3mod's
// ReplaceableTextures/TeamColor|TeamGlow/<name>NN.dds, players 00-24).
// New panics (like the desktop viewer's loadTeamTexture) if any of those 50
// team textures is missing from the root.
func New(storage *casc.Storage, r root.Root) *Server {
	s := &Server{
		storage:    storage,
		root:       r,
		resolver:   texture.NewResolver(r),
		ddsSet:     map[[16]byte]bool{},
		cache:      newModelCache(),
		CacheBound: defaultCacheBound,
	}

	for _, entry := range r.Entries {
		if strings.HasSuffix(strings.ToLower(entry.Name), ".mdx") {
			s.models = append(s.models, entry)
		}
		if strings.HasSuffix(strings.ToLower(entry.Name), ".dds") {
			s.ddsSet[entry.CKey] = true
		}
	}
	sort.Slice(s.models, func(i, j int) bool { return s.models[i].Path < s.models[j].Path })

	s.byPath = make(map[string]int, len(s.models))
	for i, m := range s.models {
		s.byPath[m.Path] = i
	}

	s.team = make([]teamTextureRefs, 25)
	for i := range s.team {
		s.team[i] = teamTextureRefs{
			Color: s.teamTextureRef("TeamColor", i),
			Glow:  s.teamTextureRef("TeamGlow", i),
		}
	}

	return s
}

// teamTextureRef looks up War3.w3mod:ReplaceableTextures/<name>/<name>NN.dds
// for the given player (00-24), panicking if it's missing.
func (s *Server) teamTextureRef(name string, player int) textureRef {
	path := fmt.Sprintf("War3.w3mod:ReplaceableTextures/%s/%s%02d.dds", name, name, player)
	entry, ok := s.root.Lookup(path)
	if !ok {
		panic(fmt.Sprintf("web: team texture not found: %s", path))
	}
	return s.textureRefFor(entry)
}

// read decodes the file with the given content key, serialized across
// goroutines since idxPackages.Archives.get races on concurrent lookups.
func (s *Server) read(ckey [16]byte) []byte {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	s.reads++
	return s.storage.ReadByCKey(ckey)
}

// stored reports whether ckey's content is stored locally, without reading
// it (mirrors main.readFile's check).
func (s *Server) stored(ckey [16]byte) bool {
	encEntry, exists := s.storage.LookupCKey(ckey)
	if !exists {
		return false
	}
	ekey := encEntry.EKeys[0]
	_, exists = s.storage.LookupEKey(ekey[:])
	return exists
}

// Handler builds the server's http.Handler: the JSON/binary API under
// /api/, and the embedded static files at /, both behind the loopback Host
// check.
func (s *Server) Handler() http.Handler {
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	registerAPI(mux, "/api/models", s.handleModels)
	registerAPI(mux, "/api/model", s.handleModel)
	registerAPI(mux, "/api/mesh", s.handleMesh)
	registerAPI(mux, "/api/texture", s.handleTexture)
	registerAPI(mux, "/api/pose", s.handlePose)
	mux.Handle("/", http.FileServerFS(static))

	return loopbackOnly(mux)
}

// registerAPI registers handler for GET path, and a 405 for every other
// method on path (the bare pattern, less specific than "GET "+path, only
// matches when the method doesn't).
func registerAPI(mux *http.ServeMux, path string, handler http.HandlerFunc) {
	mux.HandleFunc("GET "+path, handler)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
}

// loopbackOnly rejects requests whose Host isn't localhost or a loopback
// IP, a cheap guard against DNS rebinding since the server never checks
// auth and the data must never leave the machine.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		} else {
			host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		}
		if host != "localhost" && !net.ParseIP(host).IsLoopback() {
			http.Error(w, fmt.Sprintf("host %q is not localhost or a loopback address", r.Host), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Serve listens on addr (see CheckAddr) and serves Handler until an error
// occurs, printing the URL to serve on to stdout first. It panics on any
// error from net.Listen or http.Serve. A panic from a handler (e.g. an MDX
// model with a known parsing bug) is recovered by net/http per request,
// logged, and the connection dropped.
func (s *Server) Serve(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Serving the web viewer on http://%s/\n", ln.Addr())

	if err := http.Serve(ln, s.Handler()); err != nil {
		panic(err)
	}
}

// CheckAddr panics unless addr's host is "localhost" or a loopback IP. An
// empty host (e.g. ":8080", which net.Listen treats as "listen on every
// interface") and "0.0.0.0" are refused, since the server has no auth and
// its data must never leave the machine.
func CheckAddr(addr string) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	if host == "localhost" {
		return
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		panic(fmt.Sprintf("web: -addr host %q is not localhost or a loopback address", host))
	}
}
