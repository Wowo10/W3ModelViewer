package idxPackages

import (
	"encoding/binary"
	"fmt"
	"maps"
	"os"
	"path/filepath"
)

const (
	encoding = "encoding"
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

type IndexEntry struct {
	Archive uint32 // which data.### file
	Offset  uint32 // byte position inside that archive
	Size    uint32 // total bytes, including the 30-byte header
}

func ReadIdxPackages() (m map[[9]byte]IndexEntry) {
	m = make(map[[9]byte]IndexEntry)

	for _, filename := range filenames {
		path := filepath.Join(dataPath, filename)
		entries := readIdxPackage(path)

		if DEBUG {
			fmt.Printf("Read %d entries from %s\n", len(entries), filename)
		}

		maps.Copy(m, entries)
	}

	return m
}

func readIdxPackage(path string) (m map[[9]byte]IndexEntry) {
	m = make(map[[9]byte]IndexEntry)

	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	entriesSize := binary.LittleEndian.Uint32(data[0x20:0x24])
	count := int(entriesSize) / 18 // each entry is 18 bytes

	if DEBUG {
		fmt.Println("count:", count)
	}

	for i := range count {
		entryOffset := 0x28 + i*18

		var key [9]byte
		copy(key[:], data[entryOffset:entryOffset+9])

		size := binary.LittleEndian.Uint32(data[entryOffset+14 : entryOffset+18])

		raw := uint64(0)
		for _, b := range data[entryOffset+9 : entryOffset+14] {
			raw = raw<<8 | uint64(b)
		}

		archive := raw >> 30
		offset := raw & 0x3FFFFFFF

		entry := IndexEntry{
			Archive: uint32(archive),
			Offset:  uint32(offset),
			Size:    size,
		}

		var x byte
		for _, b := range key {
			x ^= b
		}
		bucket := (x & 0x0F) ^ (x >> 4)

		if DEBUG {
			fmt.Printf("IndexEntry %d, %x: %+v, B:%x\n", i, key[:], entry, bucket)
		}
		m[key] = entry

		//check
		checkPath := filepath.Join(dataPath, fmt.Sprintf("data.%03d", entry.Archive))
		f, err := os.Open(checkPath)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		header := make([]byte, 34) // 30-byte header + 4 bytes for "BLTE"
		if _, err := f.ReadAt(header, int64(entry.Offset)); err != nil {
			panic(err)
		}

		if DEBUG {
			fmt.Println("magic:", string(header[30:34])) // should print BLTE
		}

		if size > 30 && string(header[30:34]) != "BLTE" {
			panic(fmt.Sprintf("Invalid magic number: %s", string(header[30:34])))
		}

		var ekey [16]byte
		for j := 0; j < 16; j++ {
			ekey[j] = header[15-j]
		}

		if DEBUG {
			fmt.Printf("from header: %x\nfrom index:  %x\n", ekey[:9], key) // should match
		}

		if string(ekey[:9]) != string(key[:]) {
			panic(fmt.Sprintf("Key mismatch: from header: %x, from index: %x", ekey[:9], key))
		}

		if DEBUG {
			fmt.Println("header size:", binary.LittleEndian.Uint32(header[16:20]), "index size:", entry.Size)
		}

		if binary.LittleEndian.Uint32(header[16:20]) != entry.Size {
			panic(fmt.Sprintf("Size mismatch: from header: %d, from index: %d", binary.LittleEndian.Uint32(header[16:20]), entry.Size))
		}

	}

	return m
}
