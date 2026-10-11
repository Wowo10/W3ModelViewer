package catalog

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"modelView/extract/casc"
	"modelView/extract/root"
	"modelView/model/mdx"
)

// cacheFormat is bumped whenever the cache file's layout changes, so an
// older file is rescanned rather than misread.
const cacheFormat = 1

// Catalog holds a background scan's result (Info per content key) and its
// progress, safe for concurrent use.
type Catalog struct {
	mu       sync.RWMutex
	infos    map[[16]byte]Info
	unstored map[[16]byte]bool // content keys found not to be stored locally

	done, total int64 // atomic
	finished    int32 // atomic bool

	fromCache bool
	elapsed   time.Duration
}

// cacheFile is the on-disk cache, JSON-encoded: content keys as 32 lower-
// case hex chars. Unstored lists keys that were found not to be stored
// locally at scan time (so they have no entry in Models, by design), letting
// load tell "not scanned" apart from "known not to be stored" when checking
// it has every wanted key.
type cacheFile struct {
	Format   int             `json:"format"`
	Build    string          `json:"build"`
	Models   map[string]Info `json:"models"`
	Unstored []string        `json:"unstored,omitempty"`
}

// CacheDir is where Start's cache file lives: os.UserCacheDir()/modelViewer.
// It returns "" (after printing one line to stderr) when os.UserCacheDir
// fails, meaning the caller should scan uncached rather than panic a
// background goroutine over an optional file.
func CacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog: no cache directory:", err)
		return ""
	}
	return filepath.Join(dir, "modelViewer")
}

// Start collects the distinct content keys of every root entry whose Name
// ends in ".mdx" (case-insensitively, as newModelPicker does), sets the
// returned Catalog's total, and scans them in a background goroutine:
// Lookup, Progress, FromCache and Elapsed are safe to call right away.
// dir's cache file (named by buildKey) is tried first; it is used only when
// it parses, its format and build match, and it has an entry for every key
// to scan, else a full scan runs and, once complete, is written to dir
// (skipped entirely when dir == "").
func Start(storage *casc.Storage, r root.Root, buildKey, dir string) *Catalog {
	keys := distinctMDXKeys(r)

	c := &Catalog{
		infos:    make(map[[16]byte]Info, len(keys)),
		unstored: make(map[[16]byte]bool),
	}
	atomic.StoreInt64(&c.total, int64(len(keys)))

	go c.run(storage, keys, buildKey, dir)

	return c
}

// distinctMDXKeys returns the distinct content keys of every root entry
// whose Name ends in ".mdx", case-insensitively.
func distinctMDXKeys(r root.Root) [][16]byte {
	seen := map[[16]byte]bool{}
	var keys [][16]byte
	for _, entry := range r.Entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".mdx") {
			continue
		}
		if !seen[entry.CKey] {
			seen[entry.CKey] = true
			keys = append(keys, entry.CKey)
		}
	}
	return keys
}

// Lookup returns ckey's Info, if it has been scanned (or loaded from cache)
// yet.
func (c *Catalog) Lookup(ckey [16]byte) (Info, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	info, ok := c.infos[ckey]
	return info, ok
}

// Progress reports how many of the distinct content keys have been
// scanned, and whether the scan (or cache load) has finished.
func (c *Catalog) Progress() (done, total int, finished bool) {
	return int(atomic.LoadInt64(&c.done)), int(atomic.LoadInt64(&c.total)), atomic.LoadInt32(&c.finished) != 0
}

// FromCache reports whether the result came from a cache file rather than a
// fresh scan. Meaningless until Progress reports finished.
func (c *Catalog) FromCache() bool {
	return c.fromCache
}

// Elapsed is how long the scan, or the cache load, took. Meaningless until
// Progress reports finished.
func (c *Catalog) Elapsed() time.Duration {
	return c.elapsed
}

// run tries dir's cache file, else scans keys and then writes it. It sets
// elapsed and finished exactly once, whichever path it takes.
func (c *Catalog) run(storage *casc.Storage, keys [][16]byte, buildKey, dir string) {
	start := time.Now()
	path := ""
	if dir != "" {
		path = filepath.Join(dir, "catalog-"+buildKey+".json")
	}

	if path != "" {
		if infos, ok := load(path, buildKey, keys); ok {
			c.mu.Lock()
			c.infos = infos
			c.mu.Unlock()
			atomic.StoreInt64(&c.done, int64(len(keys)))
			c.fromCache = true
			c.elapsed = time.Since(start)
			atomic.StoreInt32(&c.finished, 1)
			return
		}
	}

	c.scan(storage, keys)
	c.elapsed = time.Since(start)
	atomic.StoreInt32(&c.finished, 1)

	if path != "" {
		c.save(path, buildKey)
	}
}

// scan reads and inspects every key through storage, max(1, NumCPU()-1)
// workers at a time (leaving a core free for the GL thread), recording each
// result and bumping done as it completes.
func (c *Catalog) scan(storage *casc.Storage, keys [][16]byte) {
	workers := max(1, runtime.NumCPU()-1)

	ch := make(chan [16]byte)
	go func() {
		defer close(ch)
		for _, k := range keys {
			ch <- k
		}
	}()

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for ckey := range ch {
				c.scanOne(storage, ckey)
			}
		}()
	}
	wg.Wait()
}

// scanOne inspects one content key, if it is stored locally, and records
// its Info. A content key not stored locally is recorded as unstored
// (for the cache) and stays unknown to Lookup, so the scan never panics on a
// model the list could show but not open.
func (c *Catalog) scanOne(storage *casc.Storage, ckey [16]byte) {
	defer atomic.AddInt64(&c.done, 1)

	if !stored(storage, ckey) {
		c.mu.Lock()
		c.unstored[ckey] = true
		c.mu.Unlock()
		return
	}

	info := Inspect(mdx.Read(storage.ReadByCKey(ckey)))

	c.mu.Lock()
	c.infos[ckey] = info
	c.mu.Unlock()
}

// stored reports whether ckey's content is stored locally, without reading
// it (mirrors web.Server.stored).
func stored(storage *casc.Storage, ckey [16]byte) bool {
	encEntry, exists := storage.LookupCKey(ckey)
	if !exists {
		return false
	}
	ekey := encEntry.EKeys[0]
	_, exists = storage.LookupEKey(ekey[:])
	return exists
}

// load reads path, returning its models (decoded to [16]byte keys) and true
// only when the file parses as JSON, its format and build match, and every
// key in want is either in Models or listed in Unstored (so a key that was
// found not to be stored locally at scan time, and so was never recorded in
// Models, doesn't force a rescan). Any other case, including a missing or
// corrupt file, reports false and leaves the caller to scan.
func load(path, buildKey string, want [][16]byte) (map[[16]byte]Info, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, false
	}
	if cf.Format != cacheFormat || cf.Build != buildKey {
		return nil, false
	}

	unstored := make(map[string]bool, len(cf.Unstored))
	for _, h := range cf.Unstored {
		unstored[h] = true
	}

	infos := make(map[[16]byte]Info, len(want))
	for _, ckey := range want {
		hexKey := hex.EncodeToString(ckey[:])
		if info, ok := cf.Models[hexKey]; ok {
			infos[ckey] = info
			continue
		}
		if !unstored[hexKey] {
			return nil, false
		}
	}
	return infos, true
}

// save writes the catalog's current result to path, through a temp file in
// the same directory then os.Rename, so an interrupted write never leaves a
// half-written file there. Any failure (making the directory, writing,
// renaming) is reported on stderr and otherwise ignored: an optional cache
// file isn't worth panicking a background goroutine over.
func (c *Catalog) save(path, buildKey string) {
	c.mu.RLock()
	models := make(map[string]Info, len(c.infos))
	for ckey, info := range c.infos {
		models[hex.EncodeToString(ckey[:])] = info
	}
	unstored := make([]string, 0, len(c.unstored))
	for ckey := range c.unstored {
		unstored = append(unstored, hex.EncodeToString(ckey[:]))
	}
	c.mu.RUnlock()

	data, err := json.Marshal(cacheFile{Format: cacheFormat, Build: buildKey, Models: models, Unstored: unstored})
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog: marshal cache:", err)
		return
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "catalog: cache directory:", err)
		return
	}

	tmp, err := os.CreateTemp(dir, "catalog-*.tmp")
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog: cache temp file:", err)
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		fmt.Fprintln(os.Stderr, "catalog: write cache:", err)
		return
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "catalog: close cache:", err)
		return
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		fmt.Fprintln(os.Stderr, "catalog: rename cache:", err)
	}
}
