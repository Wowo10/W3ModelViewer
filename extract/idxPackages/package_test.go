package idxPackages

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFilenamesHighestVersion(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"0a0000001c.idx", "0a0000001e.idx", "0a0000001d.idx", // stale versions left by an update
		"shmem", "data.000", "zz0000001b.idx",
	}
	for _, b := range "0123456789bcdef" {
		names = append(names, "0"+string(b)+"0000001b.idx")
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Filenames(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != bucketCount {
		t.Fatalf("Filenames: %d names, want %d", len(got), bucketCount)
	}
	if got[0x0a] != "0a0000001e.idx" {
		t.Errorf("bucket 0a: %s, want 0a0000001e.idx", got[0x0a])
	}
	if !slices.IsSorted(got) {
		t.Errorf("Filenames not in bucket order: %v", got)
	}
}

func TestFilenamesMissingBucket(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000000001b.idx"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Filenames(dir); err == nil {
		t.Error("Filenames with 15 buckets missing: want error, got nil")
	}
}
