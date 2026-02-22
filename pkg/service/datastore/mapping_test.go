package datastore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

func TestDataStore_MacAddressMapping(t *testing.T) {
	if err := os.MkdirAll("testdata/mapping", 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll("testdata/mapping")

	accountID := "12345"
	serialNumber := "SERIAL123"
	macAddress := "AABBCCDDEEFF"

	// Create directory structure
	deviceDir := filepath.Join("testdata/mapping", "accounts", accountID, "devices", serialNumber)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("failed to create device dir: %v", err)
	}

	// Create DeviceInfo.xml with MAC address
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="` + serialNumber + `">
    <name>Test Device</name>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>1.0</softwareVersion>
            <serialNumber>` + serialNumber + `</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>` + macAddress + `</macAddress>
        <ipAddress>192.168.1.10</ipAddress>
    </networkInfo>
</info>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.DeviceInfoFile), []byte(deviceInfoXML), 0644); err != nil {
		t.Fatalf("failed to write DeviceInfo.xml: %v", err)
	}

	// Create Presets.xml so we can verify access
	presetsXML := `<presets><preset id="1">test</preset></presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.PresetsFile), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("failed to write Presets.xml: %v", err)
	}

	ds := NewDataStore("testdata/mapping")
	if err := ds.Initialize(); err != nil {
		t.Fatalf("failed to initialize datastore: %v", err)
	}

	// Test mapping resolution in AccountDeviceDir
	resolvedDir := ds.AccountDeviceDir(accountID, macAddress)
	expectedDir := filepath.Join("testdata/mapping", "accounts", accountID, "devices", serialNumber)
	if resolvedDir != expectedDir {
		t.Errorf("expected dir %s, got %s", expectedDir, resolvedDir)
	}

	// Test that we can still use the serial number directly
	resolvedDirSerial := ds.AccountDeviceDir(accountID, serialNumber)
	if resolvedDirSerial != expectedDir {
		t.Errorf("expected dir %s when using serial, got %s", expectedDir, resolvedDirSerial)
	}

	// Test GetPresets using MAC address
	presets, err := ds.GetPresets(accountID, macAddress)
	if err != nil {
		t.Errorf("GetPresets failed with MAC address: %v", err)
	}
	if len(presets) == 0 {
		t.Error("expected presets to be loaded")
	}
}
