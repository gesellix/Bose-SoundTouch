package datastore

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/catalog"
)

// CatalogFile is the service-wide catalog of preset and source entries we have
// seen (issue 754). It sits next to settings.json rather than under an account
// or device directory: the catalog's scope is the service, so that several
// speakers -- and a speaker that has since been re-paired to another account --
// contribute to one pick list.
const CatalogFile = "catalog.json"

// catalogMu guards catalog.json. It is deliberately NOT ds.fileMutex: the
// preset and recents write paths record into the catalog while already holding
// fileMutex, so anything on this path that reached back for fileMutex would
// deadlock. Nothing reachable from here takes it (rootReadFile,
// atomicWriteFile and GetSettings are all lock-free).
var catalogMu sync.Mutex

// catalogSize returns the configured entry cap: unset means the default, and
// zero means the operator turned the catalog off.
func (ds *DataStore) catalogSize() int {
	settings, err := ds.GetSettings()
	if err != nil || settings.CatalogSize == nil {
		return catalog.DefaultSize
	}

	return *settings.CatalogSize
}

func (ds *DataStore) catalogPath() string {
	return filepath.Join(ds.DataDir, CatalogFile)
}

// readCatalogNoLock loads catalog.json. A missing or unreadable file yields an
// empty catalog rather than an error: the catalog is derived state that
// rebuilds itself from the next write, and no preset or recents write may fail
// because of it.
func (ds *DataStore) readCatalogNoLock() catalog.Catalog {
	var c catalog.Catalog

	if ds == nil || ds.DataDir == "" {
		return c
	}

	path := ds.catalogPath()
	if !ds.rootExists(path) {
		return c
	}

	data, err := ds.rootReadFile(path)
	if err != nil {
		return c
	}

	if err := json.Unmarshal(data, &c); err != nil {
		return catalog.Catalog{}
	}

	return c
}

func (ds *DataStore) writeCatalogNoLock(c catalog.Catalog) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return ds.atomicWriteFile(ds.catalogPath(), data)
}

// GetCatalog returns the catalog entries, newest sighting first.
func (ds *DataStore) GetCatalog() []catalog.Entry {
	catalogMu.Lock()
	defer catalogMu.Unlock()

	c := ds.readCatalogNoLock()

	return c.List()
}

// RecordCatalogEntries files the given sightings and persists the result if
// anything changed. It returns nothing: the catalog is best-effort by
// construction, and a caller recording a preset write must not be able to fail
// because the catalog could not be written.
func (ds *DataStore) RecordCatalogEntries(entries []catalog.Entry) {
	if ds == nil || ds.DataDir == "" || len(entries) == 0 {
		return
	}

	size := ds.catalogSize()

	catalogMu.Lock()
	defer catalogMu.Unlock()

	c := ds.readCatalogNoLock()

	// Apply the cap first, so lowering it in settings.json takes effect on the
	// next write rather than only once the catalog overflows again.
	changed := c.Trim(size)

	for i := range entries {
		if c.Record(entries[i], size) {
			changed = true
		}
	}

	if !changed {
		return
	}

	if err := ds.writeCatalogNoLock(c); err != nil {
		log.Printf("[Datastore] RecordCatalogEntries: could not persist the catalog: %v", err)
	}
}

// sightingTime reads the first of the given timestamps that parses, so a
// sighting can carry the speaker's own idea of when the content was stored or
// played rather than the moment we happened to write the file. It matters most
// for the backfill: six presets filed in one pass would otherwise all share one
// timestamp, and the pick list would be in arbitrary order.
//
// Two formats appear in the datastore's XML: epoch seconds (what a speaker
// writes into createdOn/updatedOn) and ISO 8601 (what the service writes in
// other records). Anything else yields the zero time, which callers replace
// with now.
func sightingTime(values ...string) time.Time {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		if epoch, err := strconv.ParseInt(v, 10, 64); err == nil && epoch > 0 {
			return time.Unix(epoch, 0).UTC()
		}

		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000-07:00"} {
			if parsed, err := time.Parse(layout, v); err == nil {
				return parsed.UTC()
			}
		}
	}

	return time.Time{}
}

// orNow replaces a timestamp we could not read with the current time.
func orNow(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}

	return t
}

// catalogEntriesFromPresets turns a preset list into catalog sightings. A
// preset is the strongest kind of sighting: content someone deliberately kept.
func catalogEntriesFromPresets(device string, presets []models.ServicePreset) []catalog.Entry {
	now := time.Now().UTC()
	entries := make([]catalog.Entry, 0, len(presets))

	for i := range presets {
		p := &presets[i]
		created := sightingTime(p.CreatedOn)
		entries = append(entries, catalog.Entry{
			Source:        p.Source,
			SourceAccount: p.SourceAccount,
			Location:      p.Location,
			Type:          p.Type,
			Name:          p.Name,
			ContainerArt:  p.ContainerArt,
			IsPresetable:  p.IsPresetable,
			SourceID:      p.SourceID,
			Origin:        catalog.OriginPreset,
			DeviceID:      device,
			FirstSeen:     orNow(created, now),
			LastSeen:      orNow(sightingTime(p.UpdatedOn, p.CreatedOn), now),
		})
	}

	return entries
}

// catalogEntriesFromRecents turns a recents list into catalog sightings. They
// carry no artwork -- the speaker never puts any there -- which is exactly why
// a recents sighting is merged into, rather than over, what a preset sighting
// of the same content contributed.
func catalogEntriesFromRecents(device string, recents []models.ServiceRecent) []catalog.Entry {
	now := time.Now().UTC()
	entries := make([]catalog.Entry, 0, len(recents))

	for i := range recents {
		r := &recents[i]

		deviceID := r.DeviceID
		if deviceID == "" {
			deviceID = device
		}

		created := sightingTime(r.CreatedOn)
		played := sightingTime(r.LastPlayedAt, r.UtcTime, r.UpdatedOn, r.CreatedOn)

		entries = append(entries, catalog.Entry{
			Source:        r.Source,
			SourceAccount: r.SourceAccount,
			Location:      r.Location,
			Type:          r.Type,
			Name:          r.Name,
			ContainerArt:  r.ContainerArt,
			IsPresetable:  r.IsPresetable,
			SourceID:      r.SourceID,
			Origin:        catalog.OriginRecent,
			DeviceID:      deviceID,
			FirstSeen:     orNow(created, now),
			LastSeen:      orNow(played, now),
		})
	}

	return entries
}

// BackfillCatalog files what the datastore already holds.
//
// Without it the catalog only ever learns from writes, so an install that has
// been running for months shows an empty pick list next to six stored presets
// until something happens to rewrite them -- exactly the entries an owner
// would reach for first. It runs at startup, and is idempotent: entries merge
// on content identity, and sightings carry the speaker's own timestamps, so a
// second pass changes nothing and writes nothing.
func (ds *DataStore) BackfillCatalog() {
	if ds == nil || ds.DataDir == "" {
		return
	}

	accounts, err := ds.ListAccounts()
	if err != nil {
		log.Printf("[Datastore] BackfillCatalog: could not list accounts: %v", err)

		return
	}

	// Collect first, record once: every read below takes ds.fileMutex, and
	// gathering under it before touching catalogMu keeps the two locks in the
	// same order the write paths take them.
	var entries []catalog.Entry

	for _, account := range accounts {
		devices, dirErr := ds.ReadDirUnderBase(ds.AccountDevicesDir(account))
		if dirErr != nil {
			continue
		}

		for _, device := range devices {
			if !device.IsDir() {
				continue
			}

			if presets, presetErr := ds.GetPresetsReadOnly(account, device.Name()); presetErr == nil {
				entries = append(entries, catalogEntriesFromPresets(device.Name(), presets)...)
			}

			if recents, recentErr := ds.GetRecents(account, device.Name()); recentErr == nil {
				entries = append(entries, catalogEntriesFromRecents(device.Name(), recents)...)
			}
		}
	}

	ds.RecordCatalogEntries(entries)
}
