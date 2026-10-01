package encoding

import (
	"encoding/binary"
	"fmt"
)

var DEBUG = false

const (
	magic      = "EN"
	headerSize = 22
	keySize    = 16
	md5Size    = 16
	kilobyte   = 1024
)

// Entry lists the encoding keys a content key is stored under.
type Entry struct {
	Size  uint64 // decoded file size
	EKeys [][keySize]byte
}

type Encoding map[[keySize]byte]Entry

type header struct {
	cePageSize  int // bytes per content-key page
	cePageCount int
	especSize   int
}

// ReadEncoding parses a decoded encoding file into a content key → encoding keys map.
// The encoding-key pages are skipped; they only hold per-file encoding specs.
func ReadEncoding(data []byte) Encoding {
	if string(data[:len(magic)]) != magic {
		panic(fmt.Sprintf("Bad encoding magic: %q", data[:len(magic)]))
	}

	h := readHeader(data)

	if DEBUG {
		fmt.Printf("header: %+v\n", h)
	}

	// Skip the espec strings and the page index (first key + MD5 per page).
	pagesStart := headerSize + h.especSize + h.cePageCount*(keySize+md5Size)

	e := make(Encoding)
	for i := range h.cePageCount {
		pageStart := pagesStart + i*h.cePageSize
		readPage(data[pageStart:pageStart+h.cePageSize], e)
	}

	return e
}

func readHeader(data []byte) header {
	return header{
		cePageSize:  int(binary.BigEndian.Uint16(data[5:7])) * kilobyte,
		cePageCount: int(binary.BigEndian.Uint32(data[9:13])),
		especSize:   int(binary.BigEndian.Uint32(data[18:22])),
	}
}

// readPage adds every entry in a content-key page to e. Pages are zero-padded,
// so a key count of 0 marks the end.
func readPage(page []byte, e Encoding) {
	offset := 0
	for offset < len(page) && page[offset] != 0 {
		keyCount := int(page[offset])

		// 5 bytes big-endian file size.
		size := uint64(0)
		for _, b := range page[offset+1 : offset+6] {
			size = size<<8 | uint64(b)
		}

		var ckey [keySize]byte
		copy(ckey[:], page[offset+6:offset+6+keySize])

		entry := Entry{Size: size, EKeys: make([][keySize]byte, keyCount)}
		ekeysStart := offset + 6 + keySize
		for i := range keyCount {
			copy(entry.EKeys[i][:], page[ekeysStart+i*keySize:ekeysStart+(i+1)*keySize])
		}

		e[ckey] = entry
		offset = ekeysStart + keyCount*keySize
	}
}

func (e Encoding) Lookup(ckey [keySize]byte) (Entry, bool) {
	entry, ok := e[ckey]
	return entry, ok
}
