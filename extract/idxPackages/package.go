package idxPackages

import (
	"encoding/binary"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var DEBUG = false

const bucketCount = 16

// Filenames returns the idx files ReadIdxPackages reads under dataDir, one
// per bucket in bucket order. An idx file is named after its bucket (2 hex
// digits) and version (8 hex digits); an update writes new versions and can
// leave the old ones behind, pointing at stale data, so each bucket's
// highest version is taken. It fails if a bucket has no idx file.
func Filenames(dataDir string) ([]string, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}

	var names [bucketCount]string
	var versions [bucketCount]uint64
	for _, e := range entries {
		bucket, version, ok := parseIdxName(e.Name())
		if !ok || bucket >= bucketCount {
			continue
		}
		if names[bucket] == "" || version > versions[bucket] {
			names[bucket], versions[bucket] = e.Name(), version
		}
	}

	for bucket, name := range names {
		if name == "" {
			return nil, fmt.Errorf("no idx file for bucket %02x", bucket)
		}
	}

	return names[:], nil
}

// parseIdxName splits an idx file name such as "0a0000001c.idx" into its
// bucket and version.
func parseIdxName(name string) (bucket, version uint64, ok bool) {
	stem, found := strings.CutSuffix(name, ".idx")
	if !found || len(stem) != 10 {
		return 0, 0, false
	}

	bucket, err := strconv.ParseUint(stem[:2], 16, 8)
	if err != nil {
		return 0, 0, false
	}
	version, err = strconv.ParseUint(stem[2:], 16, 32)
	if err != nil {
		return 0, 0, false
	}

	return bucket, version, true
}

const (
	entriesSizeOffset = 0x20
	entriesOffset     = 0x28
	entrySize         = 18 // 9-byte key + 5-byte archive/offset + 4-byte size
	keySize           = 9

	archiveHeaderSize = 30
	blteMagic         = "BLTE"
)

type IndexEntry struct {
	Archive uint32 // which data.### file
	Offset  uint32 // byte position inside that archive
	Size    uint32 // total bytes, including the 30-byte header
}

// Index maps the first 9 bytes of an encoding key to where its file is stored.
type Index map[[keySize]byte]IndexEntry

// Lookup finds an encoding key; only its first 9 bytes are stored in the index.
func (i Index) Lookup(ekey []byte) (IndexEntry, bool) {
	entry, ok := i[[keySize]byte(ekey[:keySize])]
	return entry, ok
}

// ReadIdxPackages reads all local idx files. With verify set, every entry is
// cross-checked against the header stored in its archive.
func ReadIdxPackages(dataDir string, archives Archives, verify bool) (m Index) {
	m = make(Index)

	filenames, err := Filenames(dataDir)
	if err != nil {
		panic(err)
	}

	for _, filename := range filenames {
		path := filepath.Join(dataDir, filename)
		entries := readIdxPackage(path, archives, verify)

		if DEBUG {
			fmt.Printf("Read %d entries from %s\n", len(entries), filename)
		}

		maps.Copy(m, entries)
	}

	return m
}

func readIdxPackage(path string, archives Archives, verify bool) (m Index) {
	m = make(Index)

	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	entriesSize := binary.LittleEndian.Uint32(data[entriesSizeOffset : entriesSizeOffset+4])
	count := int(entriesSize) / entrySize

	if DEBUG {
		fmt.Println("count:", count)
	}

	for i := range count {
		entryOffset := entriesOffset + i*entrySize
		key, entry := parseEntry(data[entryOffset : entryOffset+entrySize])

		if DEBUG {
			fmt.Printf("IndexEntry %d, %x: %+v, B:%x\n", i, key[:], entry, bucket(key))
		}
		if verify && !verifyEntry(key, entry, archives) {
			continue
		}

		m[key] = entry
	}

	return m
}

// parseEntry decodes one 18-byte index record.
func parseEntry(raw []byte) (key [9]byte, entry IndexEntry) {
	copy(key[:], raw[:keySize])

	// 5 bytes big-endian: top 10 bits are the archive number, low 30 the offset.
	packed := uint64(0)
	for _, b := range raw[keySize : keySize+5] {
		packed = packed<<8 | uint64(b)
	}

	entry = IndexEntry{
		Archive: uint32(packed >> 30),
		Offset:  uint32(packed & 0x3FFFFFFF),
		Size:    binary.LittleEndian.Uint32(raw[keySize+5 : keySize+9]),
	}

	return key, entry
}

func bucket(key [9]byte) byte {
	var x byte
	for _, b := range key {
		x ^= b
	}
	return (x & 0x0F) ^ (x >> 4)
}

// verifyEntry cross-checks an index entry against the header stored in its archive.
// It returns false for entries without a BLTE payload, which should be skipped.
func verifyEntry(key [9]byte, entry IndexEntry, archives Archives) bool {
	header := readArchiveHeader(archives.get(entry.Archive), entry)

	if !checkMagic(header, entry) {
		return false
	}
	checkKey(header, key)
	checkSize(header, entry)

	return true
}

// Archives keeps data.### files open so each archive is opened only once.
// The caller closes them with Close.
type Archives struct {
	dir   string
	files map[uint32]*os.File
}

// NewArchives creates an Archives that opens data.### files under dataDir.
func NewArchives(dataDir string) Archives {
	return Archives{dir: dataDir, files: make(map[uint32]*os.File)}
}

func (a Archives) get(archive uint32) *os.File {
	if f, ok := a.files[archive]; ok {
		return f
	}

	f, err := os.Open(filepath.Join(a.dir, fmt.Sprintf("data.%03d", archive)))
	if err != nil {
		panic(err)
	}
	a.files[archive] = f
	return f
}

// OpenAll opens the archive of every index entry, so Archives.get never
// writes its map afterwards and is then safe to read from several goroutines.
func (a Archives) OpenAll(index Index) {
	for _, entry := range index {
		a.get(entry.Archive)
	}
}

func (a Archives) Close() {
	for _, f := range a.files {
		f.Close()
	}
}

// ReadEntry reads an entry's payload from its archive, without the 30-byte header.
func (a Archives) ReadEntry(entry IndexEntry) []byte {
	data := make([]byte, entry.Size-archiveHeaderSize)
	if _, err := a.get(entry.Archive).ReadAt(data, int64(entry.Offset)+archiveHeaderSize); err != nil {
		panic(err)
	}

	return data
}

func readArchiveHeader(f *os.File, entry IndexEntry) []byte {
	header := make([]byte, archiveHeaderSize+len(blteMagic))
	if _, err := f.ReadAt(header, int64(entry.Offset)); err != nil {
		panic(err)
	}

	return header
}

// checkMagic reports whether the payload is BLTE-encoded. Some entries, such as
// the patch manifest ("PA"), are stored raw and carry nothing we need.
func checkMagic(header []byte, entry IndexEntry) bool {
	magic := string(header[archiveHeaderSize : archiveHeaderSize+len(blteMagic)])

	if DEBUG {
		fmt.Println("magic:", magic) // should print BLTE
	}

	return entry.Size <= archiveHeaderSize || magic == blteMagic
}

func checkKey(header []byte, key [9]byte) {
	ekey := headerEKey(header)

	if DEBUG {
		fmt.Printf("from header: %x\nfrom index:  %x\n", ekey[:9], key) // should match
	}

	if string(ekey[:9]) != string(key[:]) {
		panic(fmt.Sprintf("Key mismatch: from header: %x, from index: %x", ekey[:9], key))
	}
}

// headerEKey returns the encoding key, which the archive header stores byte-reversed.
func headerEKey(header []byte) (ekey [16]byte) {
	for j := range 16 {
		ekey[j] = header[15-j]
	}
	return ekey
}

func checkSize(header []byte, entry IndexEntry) {
	headerSize := binary.LittleEndian.Uint32(header[16:20])

	if DEBUG {
		fmt.Println("header size:", headerSize, "index size:", entry.Size)
	}

	if headerSize != entry.Size {
		panic(fmt.Sprintf("Size mismatch: from header: %d, from index: %d", headerSize, entry.Size))
	}
}
