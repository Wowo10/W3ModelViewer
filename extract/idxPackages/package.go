package idxPackages

import (
	"encoding/binary"
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

var dataPath = "wardata/Data/data/"

var filenames = []string{
	"0a0000001c.idx",
	"0b0000001b.idx",
	"0c0000001b.idx",
	"0d0000001b.idx",
	"0e0000001c.idx",
	"0f0000001b.idx",
	"000000001b.idx",
	"010000001b.idx",
	"020000001c.idx",
	"030000001b.idx",
	"040000001b.idx",
	"050000001a.idx",
	"060000001b.idx",
	"070000001b.idx",
	"080000001b.idx",
	"090000001c.idx",
}

var DEBUG = false

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
func ReadIdxPackages(archives Archives, verify bool) (m Index) {
	m = make(Index)

	for _, filename := range filenames {
		path := filepath.Join(dataPath, filename)
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
type Archives map[uint32]*os.File

func (a Archives) get(archive uint32) *os.File {
	if f, ok := a[archive]; ok {
		return f
	}

	f, err := os.Open(filepath.Join(dataPath, fmt.Sprintf("data.%03d", archive)))
	if err != nil {
		panic(err)
	}
	a[archive] = f
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
	for _, f := range a {
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
