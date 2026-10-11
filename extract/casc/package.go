package casc

import (
	"crypto/md5"
	"fmt"
	"modelView/extract/blte"
	"modelView/extract/encoding"
	"modelView/extract/idxPackages"
	"path/filepath"
)

// Storage reads files from the local archives by encoding or content key.
// After Open and LoadEncoding, ReadByCKey, LookupCKey and LookupEKey are safe
// for concurrent use.
type Storage struct {
	index    idxPackages.Index
	archives idxPackages.Archives
	encoding encoding.Encoding
	verify   bool
}

// Open reads the local idx files under installDir/Data/data and opens every
// archive they reference, so the archive map is never written again. With
// verify set, idx entries are checked against their archive headers and
// every file read is checked against its MD5.
func Open(installDir string, verify bool) *Storage {
	dataDir := filepath.Join(installDir, "Data", "data")
	archives := idxPackages.NewArchives(dataDir)
	index := idxPackages.ReadIdxPackages(dataDir, archives, verify)
	archives.OpenAll(index)

	return &Storage{
		index:    index,
		archives: archives,
		verify:   verify,
	}
}

func (s *Storage) Close() {
	s.archives.Close()
}

// LoadEncoding reads the encoding file stored at entry, whose content key is ckey.
// It must be loaded before looking up or reading by content key.
func (s *Storage) LoadEncoding(entry idxPackages.IndexEntry, ckey [16]byte) encoding.Encoding {
	s.encoding = encoding.ReadEncoding(s.read(entry, ckey))
	return s.encoding
}

// LookupEKey finds where the file with the given encoding key is stored.
func (s *Storage) LookupEKey(ekey []byte) (idxPackages.IndexEntry, bool) {
	return s.index.Lookup(ekey)
}

// LookupCKey finds the encoding keys a content key is stored under.
func (s *Storage) LookupCKey(ckey [16]byte) (encoding.Entry, bool) {
	return s.encoding.Lookup(ckey)
}

// ReadByCKey reads and decodes the file with the given content key.
func (s *Storage) ReadByCKey(ckey [16]byte) []byte {
	encEntry, exists := s.LookupCKey(ckey)
	if !exists {
		panic(fmt.Sprintf("Content key %x not found in encoding file", ckey))
	}

	ekey := encEntry.EKeys[0]
	entry, exists := s.LookupEKey(ekey[:])
	if !exists {
		panic(fmt.Sprintf("Encoding key %x not found in index packages", ekey))
	}

	return s.read(entry, ckey)
}

// read decodes the file stored at entry. With verify set, its MD5 must match ckey.
func (s *Storage) read(entry idxPackages.IndexEntry, ckey [16]byte) []byte {
	data := blte.Decode(s.archives.ReadEntry(entry), s.verify)

	if s.verify {
		if sum := md5.Sum(data); sum != ckey {
			panic(fmt.Sprintf("MD5 mismatch: got %x, want %x", sum, ckey))
		}
	}

	return data
}
