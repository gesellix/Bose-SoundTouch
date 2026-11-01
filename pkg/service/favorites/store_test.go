package favorites

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func station(location string) models.ContentItem {
	return models.ContentItem{
		Source: "TUNEIN", SourceAccount: "TUNEIN", Type: "stationurl",
		Location: location, ItemName: "Radio", ContainerArt: "https://example.test/art.png",
		IsPresetable: true,
	}
}

func mustAdd(t *testing.T, store *FileStore, item models.ContentItem) Entry {
	t.Helper()
	entry, err := store.Add(item)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func mustList(t *testing.T, store *FileStore) []Entry {
	t.Helper()
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestPersistenceOrderAndIndependentResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "favorites.json")
	store := NewFileStore(path)
	if got := mustList(t, store); got == nil || len(got) != 0 {
		t.Fatalf("missing file = %#v, want []", got)
	}
	var saved []Entry
	for i := range 12 {
		item := station(fmt.Sprintf("s%d", i))
		item.IsPresetable = i%2 == 0
		saved = append(saved, mustAdd(t, store, item))
	}
	got := mustList(t, NewFileStore(path))
	if len(got) != len(saved) {
		t.Fatalf("got %d entries, want %d", len(got), len(saved))
	}
	for i := range got {
		if !reflect.DeepEqual(got[i], saved[len(saved)-1-i]) {
			t.Fatalf("entry %d changed after reopening: %+v", i, got[i])
		}
	}
	got[0].ContentItem.ItemName = "caller mutation"
	if next := mustList(t, store); next[0].ContentItem.ItemName == "caller mutation" {
		t.Fatal("List exposed mutable store state")
	}
	// Windows synthesizes POSIX mode bits from the read-only attribute; these
	// assertions describe Unix permissions, not Windows access-control lists.
	if runtime.GOOS != "windows" {
		for target, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
			info, err := os.Stat(target)
			if err != nil || info.Mode().Perm() != mode {
				t.Fatalf("permissions for %s: info=%v, err=%v", target, info, err)
			}
		}
	}
	if err := store.Remove(saved[5].ID); err != nil {
		t.Fatal(err)
	}
	if len(mustList(t, NewFileStore(path))) != 11 {
		t.Fatal("removal did not persist")
	}
	if err := store.Remove(saved[5].ID); err != nil {
		t.Fatalf("repeated removal: %v", err)
	}
}

func TestNormalizedDuplicatesPreserveOriginalEntry(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "favorites.json"))
	item := station("http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=one")
	item.Source = "LOCAL_INTERNET_RADIO"
	item.SourceAccount = item.Source
	first := mustAdd(t, store, item)
	other := mustAdd(t, store, station("another"))
	duplicate := item
	duplicate.Location = "/station?data=one"
	duplicate.SourceAccount = ""
	duplicate.ItemName = "changed name"
	duplicate.ContainerArt = ""
	duplicate.IsPresetable = false
	if got := mustAdd(t, store, duplicate); !reflect.DeepEqual(got, first) {
		t.Fatalf("duplicate overwrote metadata: got %+v, want %+v", got, first)
	}
	if got := mustList(t, store); !reflect.DeepEqual(got, []Entry{other, first}) {
		t.Fatalf("duplicate changed insertion order: %+v", got)
	}
	for _, variant := range []models.ContentItem{
		{Source: item.Source, SourceAccount: "real-account", Location: duplicate.Location},
		{Source: item.Source, Location: "/station?data=two"},
		{Source: "RADIO_BROWSER", Location: duplicate.Location},
	} {
		if entry := mustAdd(t, store, variant); entry.ID == first.ID {
			t.Fatalf("distinct content collapsed into %s", first.ID)
		}
	}
	if len(mustList(t, store)) != 5 {
		t.Fatal("distinct accounts, locations or sources collapsed")
	}
}

func TestIdentityV1AndAssignedIDSurvivesRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	store := NewFileStore(path)
	first := mustAdd(t, store, station("s1"))
	sum := sha256.Sum256([]byte(`["TUNEIN","","s1"]`))
	if first.ID != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected v1 ID: %s", first.ID)
	}
	// A future identity implementation must not invalidate IDs already assigned.
	first.ID = strings.Repeat("a", 64)
	writeDocument(t, path, document{Version: 1, Entries: []Entry{first}})
	if got := mustAdd(t, store, first.ContentItem); !reflect.DeepEqual(got, first) {
		t.Fatalf("assigned ID changed: got %+v, want %+v", got, first)
	}
	if err := store.Remove(first.ID); err != nil || len(mustList(t, store)) != 0 {
		t.Fatalf("assigned ID cannot be removed: %v", err)
	}
}

func TestConcurrentAddsAndRemoves(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "favorites.json"))
	var initial []Entry
	for i := range 20 {
		initial = append(initial, mustAdd(t, store, station(fmt.Sprintf("initial-%d", i))))
	}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Add(station(fmt.Sprintf("added-%d", i))); err != nil {
				t.Error(err)
			}
			if i < 10 {
				if err := store.Remove(initial[i].ID); err != nil {
					t.Error(err)
				}
			}
			if _, err := store.Add(station("shared-duplicate")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got := mustList(t, store)
	if len(got) != 51 {
		t.Fatalf("lost concurrent update: got %d, want 51", len(got))
	}
	seen := make(map[string]bool)
	for _, entry := range got {
		seen[entry.ContentItem.Location] = true
	}
	for i := range 40 {
		if !seen[fmt.Sprintf("added-%d", i)] {
			t.Fatalf("missing concurrent addition %d", i)
		}
	}
	for i := range 20 {
		if seen[fmt.Sprintf("initial-%d", i)] != (i >= 10) {
			t.Fatalf("incorrect concurrent removal %d", i)
		}
	}
}

func writeDocument(t *testing.T, path string, doc document) {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidFilesNeverBecomeEmptyReplacements(t *testing.T) {
	entry := Entry{ID: itemID(station("s1")), ContentItem: station("s1"), AddedAt: time.Now().UTC()}
	other := entry
	other.ContentItem.Location = "s2"
	badID := entry
	badID.ID = "invalid"
	zeroTime := entry
	zeroTime.AddedAt = time.Time{}
	for name, doc := range map[string]document{
		"unknown version":   {Version: 2, Entries: []Entry{}},
		"missing entries":   {Version: 1},
		"duplicate ID":      {Version: 1, Entries: []Entry{entry, other}},
		"duplicate content": {Version: 1, Entries: []Entry{entry, {ID: strings.Repeat("b", 64), ContentItem: entry.ContentItem, AddedAt: entry.AddedAt}}},
		"invalid ID":        {Version: 1, Entries: []Entry{badID}},
		"missing timestamp": {Version: 1, Entries: []Entry{zeroTime}},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "favorites.json")
			writeDocument(t, path, doc)
			assertInvalidFilePreserved(t, path)
		})
	}
	for name, data := range map[string][]byte{
		"corrupt":   []byte(`{"version":1,"entries":[`),
		"empty":     {},
		"oversized": bytes.Repeat([]byte(" "), maxFileBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "favorites.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			assertInvalidFilePreserved(t, path)
		})
	}
	t.Run("unreadable directory", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		if _, err := store.List(); err == nil {
			t.Fatal("directory read succeeded")
		}
		if _, err := store.Add(station("s1")); err == nil {
			t.Fatal("directory was replaced")
		}
	})
}

func assertInvalidFilePreserved(t *testing.T, path string) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewFileStore(path)
	if _, err := store.List(); err == nil {
		t.Fatal("invalid file read succeeded")
	}
	if _, err := store.Add(station("new")); err == nil {
		t.Fatal("Add overwrote invalid file")
	}
	if err := store.Remove("absent"); err == nil {
		t.Fatal("Remove ignored invalid file")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("invalid file changed: %v", err)
	}
}

func TestValidateReplayAndBounds(t *testing.T) {
	for _, source := range []string{"TUNEIN", "RADIO_BROWSER", "LOCAL_INTERNET_RADIO", "SPOTIFY", "STORED_MUSIC", "LOCAL_MUSIC", "future-provider"} {
		if err := Validate(models.ContentItem{Source: source, Location: "provider:opaque/item?variant=2"}); err != nil {
			t.Errorf("legitimate provider %s rejected: %v", source, err)
		}
	}
	for _, item := range []models.ContentItem{
		{Source: "", Location: "s1"}, {Source: "TUNEIN", Location: " \t"},
		{Source: " standby ", Location: "x"}, {Source: "AUX", Location: "x"},
		{Source: "AirPlay", Location: "x"}, {Source: "BLUETOOTH", Location: "x"},
		{Source: "TUNEIN", Location: "x", ItemName: strings.Repeat("n", maxItemBytes)},
		{Source: "TUNEIN", Location: string([]byte{0xff})},
	} {
		if err := Validate(item); err == nil {
			t.Errorf("invalid item accepted: source=%q", item.Source)
		}
	}
}

func TestFailedWritePreservesPreviousFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "favorites.json")
	store := NewFileStore(path)
	first := mustAdd(t, store, station("s1"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	// A privileged test runner may bypass directory permissions.
	if probe, err := os.CreateTemp(dir, "permission-probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("test runner bypasses directory write permissions")
	}
	if _, err := store.Add(station("s2")); err == nil {
		t.Fatal("write to unwritable directory succeeded")
	}
	if err := store.Remove(first.ID); err == nil {
		t.Fatal("removal from unwritable store succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed write changed previous file: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "favorites.json" {
		t.Fatalf("failed write left temporary files: %v, %v", files, err)
	}
}

func TestFailedRenameCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "favorites.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewFileStore(path)
	if err := store.write([]Entry{}); err == nil {
		t.Fatal("rename over directory succeeded")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 || files[0].Name() != "favorites.json" {
		t.Fatalf("failed rename left temporary files: %v, %v", files, err)
	}
}

func TestFullStoreRejectsAddWithoutEviction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	item := station("large-000")
	item.ItemName = strings.Repeat("n", 64000)
	entry := Entry{ID: itemID(item), ContentItem: item, AddedAt: time.Now().UTC()}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	// Equal-width locations make every entry the same size. Fill the file so
	// one more valid item exceeds the file bound, while the existing file fits.
	count := (maxFileBytes - 100) / (len(encoded) + 1)
	entries := make([]Entry, count)
	for i := range entries {
		entries[i] = entry
		entries[i].ContentItem.Location = fmt.Sprintf("large-%03d", i)
		entries[i].ID = itemID(entries[i].ContentItem)
	}
	writeDocument(t, path, document{Version: 1, Entries: entries})
	before, err := os.ReadFile(path)
	if err != nil || len(before) > maxFileBytes {
		t.Fatalf("invalid test setup: %d bytes, err=%v", len(before), err)
	}
	store := NewFileStore(path)
	item.Location = "large-new"
	if _, err := store.Add(item); err == nil {
		t.Fatal("full store accepted addition")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("full store evicted or changed entries: %v", err)
	}
}
