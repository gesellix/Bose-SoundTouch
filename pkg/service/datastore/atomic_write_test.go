package datastore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAtomicWriteFile_DurableRoundTrip guards the durable write path added for
// #458: content must round-trip, an overwrite must truncate cleanly, and no
// `.tmp` sidecar may be left behind. (The fsync durability itself isn't
// unit-testable without power-loss fault injection; this is the functional
// regression guard so the fsync rework doesn't break writes.)
func TestAtomicWriteFile_DurableRoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-atomic-test-*")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds := NewDataStore(tempDir)

	dir := ds.AccountDeviceDir("1234567", "001122334455")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "Sources.xml")

	if err := ds.atomicWriteFile(path, []byte("first")); err != nil {
		t.Fatalf("atomicWriteFile (create) failed: %v", err)
	}

	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Errorf("content after create = %q, want %q", got, "first")
	}

	// Overwrite must truncate the previous (longer) content, not leave a tail.
	if err := ds.atomicWriteFile(path, []byte("hi")); err != nil {
		t.Fatalf("atomicWriteFile (overwrite) failed: %v", err)
	}

	if got, _ := os.ReadFile(path); string(got) != "hi" {
		t.Errorf("content after overwrite = %q, want %q", got, "hi")
	}

	// No leftover temp sidecar.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected no %s.tmp leftover, stat err = %v", path, err)
	}
}
