// Package favorites persists the household's explicitly saved content,
// independently of the learned catalog and the speakers' six preset slots.
// One process must own each file and share one FileStore instance for that path.
// The mutex protects operations in that instance; it is not a cross-process lock.
package favorites

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// These are safety bounds rather than eviction policies: ordinary entries are
// hundreds of bytes, so 64 KiB leaves ample room for provider replay metadata,
// and 16 MiB accommodates thousands of favorites. A full store returns an error.
const (
	maxItemBytes  = 64 << 10
	maxFileBytes  = 16 << 20
	schemaVersion = 1
)

// Entry is one explicitly saved, replayable content item.
type Entry struct {
	ID          string             `json:"id"`
	ContentItem models.ContentItem `json:"content_item"`
	AddedAt     time.Time          `json:"added_at"`
}

type document struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// FileStore stores favorites in a versioned JSON file. It rereads the file on
// every operation so read errors never turn into an empty replacement store.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore constructs a store; it does not read or create the file.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Validate checks that an item can be saved for replay. Provider locations are
// opaque: they need not be URLs, and IsPresetable does not govern favorites.
// Validation never normalizes the content that will be persisted.
func Validate(item models.ContentItem) error {
	if strings.TrimSpace(item.Source) == "" || strings.TrimSpace(item.Location) == "" {
		return fmt.Errorf("favorites require a source and location")
	}

	switch strings.ToUpper(strings.TrimSpace(item.Source)) {
	case "STANDBY", "AUX", "AIRPLAY", "BLUETOOTH":
		return fmt.Errorf("source %q cannot be saved for replay", item.Source)
	}

	for _, value := range []string{item.Source, item.Location, item.SourceAccount, item.Type, item.ItemName, item.ContainerArt} {
		if !utf8.ValidString(value) {
			return fmt.Errorf("favorite content must be valid UTF-8")
		}
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode favorite content: %w", err)
	}

	if len(data) > maxItemBytes {
		return fmt.Errorf("favorite content exceeds %d bytes", maxItemBytes)
	}

	return nil
}

// identity defines favorites identity v1: a JSON string tuple of source,
// account (the exact source-name placeholder becomes empty), and canonical
// content location. Names, artwork and replay metadata do not affect identity.
// IDs are initially the full SHA256 hex of this tuple, but assigned IDs remain
// authoritative on load: future normalization changes must not invalidate them.
func identity(item models.ContentItem) string {
	account := item.SourceAccount
	if account == item.Source {
		account = ""
	}

	parts := [3]string{item.Source, account, models.CanonicalContentLocation(item.Source, item.Location)}
	data, _ := json.Marshal(parts) //nolint:errchkjson // A fixed tuple of strings is always JSON encodable.

	return string(data)
}

func itemID(item models.ContentItem) string {
	sum := sha256.Sum256([]byte(identity(item)))
	return hex.EncodeToString(sum[:])
}

// List returns favorites in insertion order, newest first. A missing file
// returns an empty slice. Invalid or unreadable persisted state returns an error.
func (s *FileStore) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.read()
}

// Add saves an item once. An existing identity returns its original metadata
// and timestamp without changing its position or writing the file.
func (s *FileStore) Add(item models.ContentItem) (Entry, error) {
	if err := Validate(item); err != nil {
		return Entry{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.read()
	if err != nil {
		return Entry{}, err
	}

	key := identity(item)
	for i := range entries {
		if identity(entries[i].ContentItem) == key {
			return entries[i], nil
		}
	}

	entry := Entry{ID: itemID(item), ContentItem: item, AddedAt: time.Now().UTC()}
	for i := range entries {
		if entries[i].ID == entry.ID {
			return Entry{}, fmt.Errorf("favorite ID already assigned to different content")
		}
	}

	entries = append([]Entry{entry}, entries...)

	if err := s.write(entries); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

// Remove deletes a favorite by ID. Removing an absent ID is a no-op, including
// when the store does not yet exist. Read errors are always reported.
func (s *FileStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.read()
	if err != nil {
		return err
	}

	for i := range entries {
		if entries[i].ID == id {
			return s.write(append(entries[:i], entries[i+1:]...))
		}
	}

	return nil
}

func (s *FileStore) read() ([]Entry, error) {
	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("open favorites: %w", err)
	}

	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read favorites: %w", err)
	}

	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("favorites file exceeds %d bytes", maxFileBytes)
	}

	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode favorites: %w", err)
	}

	if doc.Version != schemaVersion || doc.Entries == nil {
		return nil, fmt.Errorf("invalid favorites schema (version %d)", doc.Version)
	}

	if err := validateEntries(doc.Entries); err != nil {
		return nil, err
	}

	return doc.Entries, nil
}

func validateEntries(entries []Entry) error {
	seen := make(map[string]bool, len(entries))
	identities := make(map[string]bool, len(entries))

	for i := range entries {
		entry := &entries[i]
		if err := Validate(entry.ContentItem); err != nil {
			return fmt.Errorf("invalid favorite %d: %w", i, err)
		}

		_, err := hex.DecodeString(entry.ID)
		if len(entry.ID) != sha256.Size*2 || err != nil || entry.AddedAt.IsZero() {
			return fmt.Errorf("invalid favorite identity or timestamp at entry %d", i)
		}

		if seen[entry.ID] || identities[identity(entry.ContentItem)] {
			return fmt.Errorf("duplicate favorite ID at entry %d", i)
		}

		seen[entry.ID] = true
		identities[identity(entry.ContentItem)] = true
	}

	return nil
}

func (s *FileStore) write(entries []Entry) error {
	data, err := json.Marshal(document{Version: schemaVersion, Entries: entries})
	if err != nil {
		return fmt.Errorf("encode favorites: %w", err)
	}

	if len(data) > maxFileBytes {
		return fmt.Errorf("favorites file exceeds %d bytes", maxFileBytes)
	}

	dir := filepath.Dir(s.path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create favorites directory: %w", err)
	}

	file, err := os.CreateTemp(dir, ".favorites-*")
	if err != nil {
		return fmt.Errorf("create favorites temporary file: %w", err)
	}

	defer func() { _ = os.Remove(file.Name()) }() // Best effort after a failed write or rename.

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write favorites: %w", err)
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync favorites: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close favorites: %w", err)
	}

	if err := os.Rename(file.Name(), s.path); err != nil {
		return fmt.Errorf("replace favorites: %w", err)
	}

	// The rename has committed. Directory sync is best effort because some
	// filesystems do not support it; do not report a committed write as failed.
	if parent, err := os.Open(dir); err == nil {
		_ = parent.Sync()
		_ = parent.Close()
	}

	return nil
}
