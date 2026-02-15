package datastore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestDataStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "test-account"
	device := "test-device"

	// Test Save/Get DeviceInfo
	info := &models.ServiceDeviceInfo{
		DeviceID:  device,
		Name:      "Test Speaker",
		AccountID: account,
	}

	err = ds.SaveDeviceInfo(account, device, info)
	if err != nil {
		t.Errorf("SaveDeviceInfo failed: %v", err)
	}

	loadedInfo, err := ds.GetDeviceInfo(account, device)
	if err != nil {
		t.Errorf("GetDeviceInfo failed: %v", err)
	}

	if loadedInfo.Name != info.Name {
		t.Errorf("Expected name %s, got %s", info.Name, loadedInfo.Name)
	}

	// Test Presets
	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name: "Preset 1",
			},
		},
	}

	err = ds.SavePresets(account, device, presets)
	if err != nil {
		t.Errorf("SavePresets failed: %v", err)
	}

	loadedPresets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Errorf("GetPresets failed: %v", err)
	}

	if len(loadedPresets) != 1 || loadedPresets[0].Name != "Preset 1" {
		t.Errorf("Unexpected presets: %+v", loadedPresets)
	}

	// Test Recents
	recents := []models.ServiceRecent{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name: "Recent 1",
			},
		},
	}

	err = ds.SaveRecents(account, device, recents)
	if err != nil {
		t.Errorf("SaveRecents failed: %v", err)
	}

	loadedRecents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Errorf("GetRecents failed: %v", err)
	}

	if len(loadedRecents) != 1 || loadedRecents[0].Name != "Recent 1" {
		t.Errorf("Unexpected recents: %+v", loadedRecents)
	}

	// Test path helpers
	expectedAccountDir := filepath.Join(tempDir, "accounts", account)
	if ds.AccountDir(account) != expectedAccountDir {
		t.Errorf("Expected account dir %s, got %s", expectedAccountDir, ds.AccountDir(account))
	}
}

func TestListAllDevices_Empty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-empty-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)

	// Case 1: DataDir does not exist
	_ = os.RemoveAll(tempDir)

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Errorf("ListAllDevices should not return error when DataDir does not exist, got %v", err)
	}

	if devices == nil || len(devices) != 0 {
		t.Errorf("Expected empty slice when DataDir does not exist, got %+v", devices)
	}

	// Case 2: DataDir is empty
	_ = os.MkdirAll(tempDir, 0755)

	devices, err = ds.ListAllDevices()
	if err != nil {
		t.Errorf("ListAllDevices failed on empty dir: %v", err)
	}

	if devices == nil {
		t.Errorf("Expected empty slice (not nil) when no devices exist")
	}

	if len(devices) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(devices))
	}
}

func TestListAllDevices(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-list-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "default"
	deviceID := "BO5EBO5E-F00D-F00D-FEED-08DF1F0BA325"

	info := &models.ServiceDeviceInfo{
		DeviceID:           deviceID,
		Name:               "Test Speaker",
		IPAddress:          "192.168.178.28",
		DeviceSerialNumber: deviceID,
		ProductCode:        "SoundTouch 10",
		FirmwareVersion:    "1.2.3",
		AccountID:          account,
	}

	err = ds.SaveDeviceInfo(account, deviceID, info)
	if err != nil {
		t.Fatalf("SaveDeviceInfo failed: %v", err)
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices failed: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("Expected 1 device, got %d", len(devices))
	}

	if devices[0].DeviceID != deviceID {
		t.Errorf("Expected DeviceID %s, got %s", deviceID, devices[0].DeviceID)
	}
}

func TestListAllDevices_EmptyDeviceID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-empty-id-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "default"
	deviceID := ""

	info := &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		Name:      "Empty ID Speaker",
		AccountID: account,
	}

	// Use IP as fallback for device ID if it is empty
	key := deviceID
	if key == "" {
		key = "127.0.0.1"
	}

	err = ds.SaveDeviceInfo(account, key, info)
	if err != nil {
		t.Fatalf("SaveDeviceInfo failed: %v", err)
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices failed: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("Expected 1 device, got %d", len(devices))
	}

	if devices[0].Name != "Empty ID Speaker" {
		t.Errorf("Expected Name 'Empty ID Speaker', got %s", devices[0].Name)
	}
}

func TestListAllDevices_MultipleEmptyIDs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-multi-empty-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "default"

	// Save two devices with empty ID but different IPs
	info1 := &models.ServiceDeviceInfo{
		DeviceID:  "",
		Name:      "Speaker 1",
		IPAddress: "192.168.1.1",
		AccountID: account,
	}
	info2 := &models.ServiceDeviceInfo{
		DeviceID:  "",
		Name:      "Speaker 2",
		IPAddress: "192.168.1.2",
		AccountID: account,
	}

	// We use the same logic as in main.go: use IP as fallback for directory name
	err = ds.SaveDeviceInfo(account, info1.IPAddress, info1)
	if err != nil {
		t.Fatalf("SaveDeviceInfo 1 failed: %v", err)
	}

	err = ds.SaveDeviceInfo(account, info2.IPAddress, info2)
	if err != nil {
		t.Fatalf("SaveDeviceInfo 2 failed: %v", err)
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices failed: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("Expected 2 devices, got %d", len(devices))
	}
}

func TestListAllDevices_MalformedXML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-malformed-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "default"
	deviceID := "malformed-device"

	dir := ds.AccountDeviceDir(account, deviceID)
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, "DeviceInfo.xml"), []byte("<info>not even closed"), 0644)

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices should not return error on malformed XML: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("Expected 0 devices, got %d", len(devices))
	}
}

func TestConfiguredSources(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "datastore-sources-*")

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "test-acc"

	sources := []models.ConfiguredSource{
		{
			DisplayName: "Source 1",
			ID:          "101",
			Secret:      "secret1",
			SecretType:  "type1",
			SourceKey: struct {
				Type    string `xml:"type,attr"`
				Account string `xml:"account,attr"`
			}{Type: "TUNEIN", Account: "user1"},
			SourceKeyType:    "TUNEIN",
			SourceKeyAccount: "user1",
		},
		{
			DisplayName: "Source 2",
			ID:          "102",
			Secret:      "secret2",
			SecretType:  "type2",
			SourceKey: struct {
				Type    string `xml:"type,attr"`
				Account string `xml:"account,attr"`
			}{Type: "PANDORA", Account: "user2"},
			SourceKeyType:    "PANDORA",
			SourceKeyAccount: "user2",
		},
	}

	err := ds.SaveConfiguredSources(account, "any", sources)
	if err != nil {
		t.Fatalf("SaveConfiguredSources failed: %v", err)
	}

	loadedSources, err := ds.GetConfiguredSources(account, "any")
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	if len(loadedSources) != len(sources) {
		t.Fatalf("Expected %d sources, got %d", len(sources), len(loadedSources))
	}

	for i, s := range sources {
		ls := loadedSources[i]
		if ls.DisplayName != s.DisplayName || ls.ID != s.ID || ls.Secret != s.Secret ||
			ls.SecretType != s.SecretType || ls.SourceKeyType != s.SourceKeyType ||
			ls.SourceKeyAccount != s.SourceKeyAccount {
			t.Errorf("Source %d mismatch. Expected %+v, got %+v", i, s, ls)
		}
	}

	// Test with missing ID (GetConfiguredSources should auto-assign)
	sources2 := []models.ConfiguredSource{
		{
			DisplayName:      "Source No ID",
			SourceKeyType:    "LOCAL",
			SourceKeyAccount: "user3",
		},
	}

	err = ds.SaveConfiguredSources(account, "any", sources2)
	if err != nil {
		t.Fatal(err)
	}

	loadedSources2, err := ds.GetConfiguredSources(account, "any")
	if err != nil {
		t.Fatal(err)
	}

	if loadedSources2[0].ID == "" {
		t.Error("Expected auto-assigned ID for source with empty ID")
	}
}

func TestSettingsPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "settings-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)

	settings := Settings{
		ServerURL:         "http://myserver:8000",
		SoundcorkURL:      "http://myproxy:8001",
		LogBodies:         true,
		DiscoveryInterval: "10m",
		DiscoveryEnabled:  true,
	}

	err = ds.SaveSettings(settings)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	loaded, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}

	if loaded.ServerURL != settings.ServerURL {
		t.Errorf("Expected ServerURL %s, got %s", settings.ServerURL, loaded.ServerURL)
	}
	if loaded.LogBodies != settings.LogBodies {
		t.Errorf("Expected LogBodies %v, got %v", settings.LogBodies, loaded.LogBodies)
	}
	if loaded.DiscoveryInterval != settings.DiscoveryInterval {
		t.Errorf("Expected DiscoveryInterval %s, got %s", settings.DiscoveryInterval, loaded.DiscoveryInterval)
	}
	if loaded.DiscoveryEnabled != settings.DiscoveryEnabled {
		t.Errorf("Expected DiscoveryEnabled %v, got %v", settings.DiscoveryEnabled, loaded.DiscoveryEnabled)
	}
}
