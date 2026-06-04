package datastore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// These tests cover #458: an unclean power-cut on the speaker's NAND can leave
// a datastore file present but 0-byte (truncated, not-yet-flushed write). The
// read paths must treat empty / 0-byte / unparseable files the same as
// "missing" — serve defaults for sources, return empty lists for presets/recents
// — instead of advertising nothing on /full (which wipes the speaker) or
// returning HTTP 500 on the device-level endpoints.

func newTestStore(t *testing.T) (*DataStore, string, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "st-empty-test-*")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	dir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	return ds, account, device
}

func writeDeviceFile(t *testing.T, ds *DataStore, account, device, name string, content []byte) {
	t.Helper()

	path := filepath.Join(ds.AccountDeviceDir(account, device), name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetConfiguredSources_EmptyFile_ServesDefaults(t *testing.T) {
	ds, account, device := newTestStore(t)
	writeDeviceFile(t, ds, account, device, constants.SourcesFile, []byte{})

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources returned error for 0-byte file: %v", err)
	}

	if len(sources) == 0 {
		t.Fatal("expected default sources for a 0-byte Sources.xml, got none")
	}

	types := map[string]bool{}
	for i := range sources {
		types[sources[i].SourceKeyType] = true
	}

	for _, want := range []string{constants.ProviderTunein, constants.ProviderLocalInternetRadio} {
		if !types[want] {
			t.Errorf("expected default sources to include %q (ding/radio need it); got %v", want, types)
		}
	}
}

func TestGetConfiguredSources_MalformedFile_ServesDefaults(t *testing.T) {
	ds, account, device := newTestStore(t)
	writeDeviceFile(t, ds, account, device, constants.SourcesFile, []byte("<sources><not-closed"))

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources returned error for malformed file: %v", err)
	}

	if len(sources) == 0 {
		t.Fatal("expected default sources for a malformed Sources.xml, got none")
	}
}

func TestGetPresets_EmptyFile_NoError(t *testing.T) {
	ds, account, device := newTestStore(t)
	writeDeviceFile(t, ds, account, device, constants.PresetsFile, []byte{})

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets returned error for 0-byte file (would surface as HTTP 500): %v", err)
	}

	if len(presets) != 0 {
		t.Errorf("expected no presets for a 0-byte Presets.xml, got %d", len(presets))
	}
}

func TestGetRecents_EmptyFile_NoError(t *testing.T) {
	ds, account, device := newTestStore(t)
	writeDeviceFile(t, ds, account, device, constants.RecentsFile, []byte{})

	recents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("GetRecents returned error for 0-byte file (would surface as HTTP 500): %v", err)
	}

	if len(recents) != 0 {
		t.Errorf("expected no recents for a 0-byte Recents.xml, got %d", len(recents))
	}
}

func TestHasConfiguredSources_EmptyFile_False(t *testing.T) {
	ds, account, device := newTestStore(t)

	// 0-byte file present must NOT count as "has sources" — otherwise the
	// sources_xml_present health check stays green and hides the
	// create_default_sources quick fix.
	writeDeviceFile(t, ds, account, device, constants.SourcesFile, []byte{})

	if ds.HasConfiguredSources(account, device) {
		t.Error("HasConfiguredSources returned true for a 0-byte Sources.xml; want false")
	}

	// A populated file must still count as present.
	writeDeviceFile(t, ds, account, device, constants.SourcesFile,
		[]byte(`<?xml version="1.0"?><sources><source><sourceKey type="TUNEIN" account=""/></source></sources>`))

	if !ds.HasConfiguredSources(account, device) {
		t.Error("HasConfiguredSources returned false for a populated Sources.xml; want true")
	}
}
