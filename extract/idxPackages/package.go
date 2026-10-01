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

const DEBUG = false

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

func ReadIdxPackages() (m map[[9]byte]IndexEntry) {
	m = make(map[[9]byte]IndexEntry)

	archives := make(archiveFiles)
	defer archives.closeAll()

	for _, filename := range filenames {
		path := filepath.Join(dataPath, filename)
		entries := readIdxPackage(path, archives)

		if DEBUG {
			fmt.Printf("Read %d entries from %s\n", len(entries), filename)
		}

		maps.Copy(m, entries)
	}

	return m
}

func readIdxPackage(path string, archives archiveFiles) (m map[[9]byte]IndexEntry) {
	m = make(map[[9]byte]IndexEntry)

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
		m[key] = entry

		verifyEntry(key, entry, archives)
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
func verifyEntry(key [9]byte, entry IndexEntry, archives archiveFiles) {
	header := readArchiveHeader(archives.get(entry.Archive), entry)

	checkMagic(header, entry)
	checkKey(header, key)
	checkSize(header, entry)
}

// archiveFiles keeps data.### files open so each archive is opened only once.
type archiveFiles map[uint32]*os.File

func (a archiveFiles) get(archive uint32) *os.File {
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

func (a archiveFiles) closeAll() {
	for _, f := range a {
		f.Close()
	}
}

func readArchiveHeader(f *os.File, entry IndexEntry) []byte {
	header := make([]byte, archiveHeaderSize+len(blteMagic))
	if _, err := f.ReadAt(header, int64(entry.Offset)); err != nil {
		panic(err)
	}

	return header
}

func checkMagic(header []byte, entry IndexEntry) {
	magic := string(header[archiveHeaderSize : archiveHeaderSize+len(blteMagic)])

	if DEBUG {
		fmt.Println("magic:", magic) // should print BLTE
	}

	if entry.Size > archiveHeaderSize && magic != blteMagic {
		panic(fmt.Sprintf("Invalid magic number: %s", magic))
	}
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
