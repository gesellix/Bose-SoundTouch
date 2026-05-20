package datastore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestListAllDevices_RealAccountWinsOverDefault reproduces the operator's
// observation: the same physical device exists on disk under both
// accounts/default/devices/<id>/ (leftover pre-pair state) and
// accounts/<real>/devices/<id>/ (active pairing). The dedupe loop used to
// let "default" replace the real-account entry whenever the default
// directory's DeviceInfo.xml had a non-empty <name>, which made
// consistency checks report the device under "default" while the speaker
// was happily POST/PUT'ing to the real account.
//
// "default" must be treated as a fallback placeholder, never as
// authoritative when a real account also has the device.
func TestListAllDevices_RealAccountWinsOverDefault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-default-orphan-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	deviceID := "AABBCCDDEEFF"

	// Default-account entry: stale pre-pair state with a name.
	defaultDir := filepath.Join(tempDir, "accounts", "default", "devices", deviceID)
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		t.Fatalf("mkdir default: %v", err)
	}

	if err := os.WriteFile(filepath.Join(defaultDir, "DeviceInfo.xml"), []byte(`<?xml version="1.0"?>
<info deviceID="`+deviceID+`"><name>Discovered Device</name></info>`), 0644); err != nil {
		t.Fatalf("write default info: %v", err)
	}

	// Real-account entry: the speaker is paired here.
	realDir := filepath.Join(tempDir, "accounts", "1111111", "devices", deviceID)
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}

	if err := os.WriteFile(filepath.Join(realDir, "DeviceInfo.xml"), []byte(`<?xml version="1.0"?>
<info deviceID="`+deviceID+`"><name>Living Room SoundTouch</name></info>`), 0644); err != nil {
		t.Fatalf("write real info: %v", err)
	}

	ds := NewDataStore(tempDir)

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected exactly 1 deduped device, got %d: %+v", len(devices), devices)
	}

	if devices[0].AccountID != "1111111" {
		t.Errorf("expected real account 1111111 to win dedup, got AccountID=%q (default leaked through)", devices[0].AccountID)
	}

	if devices[0].Name != "Living Room SoundTouch" {
		t.Errorf("expected real-account Name to survive dedup, got %q", devices[0].Name)
	}
}

// TestListAllDevices_DefaultOnlyDeviceStillSeen confirms the dedup
// preference is one-way: a device that *only* exists under "default"
// (e.g. fresh discovery, never paired) is still returned. We just
// refuse to let "default" override a real account.
func TestListAllDevices_DefaultOnlyDeviceStillSeen(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-default-only-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	deviceID := "112233445566"
	defaultDir := filepath.Join(tempDir, "accounts", "default", "devices", deviceID)
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(defaultDir, "DeviceInfo.xml"), []byte(`<?xml version="1.0"?>
<info deviceID="`+deviceID+`"><name>Newly Discovered</name></info>`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ds := NewDataStore(tempDir)

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	if devices[0].AccountID != "default" {
		t.Errorf("expected default-only device to be returned with AccountID=default, got %q", devices[0].AccountID)
	}
}
