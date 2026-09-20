package datastore

import (
	"encoding/json"
	"log"
	"path/filepath"
	"sync"

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

	for _, e := range entries {
		if c.Record(e, size) {
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
