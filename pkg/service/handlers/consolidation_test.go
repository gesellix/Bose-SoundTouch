package handlers

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestDeviceMigration_DirectoryRename(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	srv := NewServer(ds, nil, "http://localhost", false, false, false, true, false)

	accountID := "test-account"
	macAddress := "A81B6A536A98"
	serialNumber := "I6332527703739342000020"

	// Create serial-based device entry with full data (simulates legacy directory)
	serialDeviceInfo := &models.ServiceDeviceInfo{
		DeviceID:           serialNumber,
		AccountID:          accountID,
		Name:               "Living Room Speaker",
		MacAddress:         macAddress,
		DeviceSerialNumber: serialNumber,
		ProductCode:        "SoundTouch 30",
	}

	if err := ds.SaveDeviceInfo(accountID, serialNumber, serialDeviceInfo); err != nil {
		t.Fatalf("Failed to save serial-based device: %v", err)
	}

	// Create some preset data in the serial-based directory
	serialPresets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:     "1",
				Name:   "My Preset",
				Source: "SPOTIFY",
			},
		},
	}
	if err := ds.SavePresets(accountID, serialNumber, serialPresets); err != nil {
		t.Fatalf("Failed to save presets: %v", err)
	}

	// Verify initial state - serial directory exists
	serialDir := ds.AccountDeviceDir(accountID, serialNumber)
	if _, err := os.Stat(serialDir); os.IsNotExist(err) {
		t.Fatalf("Serial directory should exist before migration: %s", serialDir)
	}

	// Perform migration using migration manager
	existingDevices := []models.ServiceDeviceInfo{*serialDeviceInfo}
	srv.migrationManager.MigrateDevicesIfNeeded(existingDevices, macAddress)

	// Verify migration results
	t.Run("VerifyMigration", func(t *testing.T) {
		// 1. MAC directory should exist with files
		macDir := ds.AccountDeviceDir(accountID, macAddress)
		serialDir := ds.AccountDeviceDir(accountID, serialNumber)

		if _, err := os.Stat(macDir); os.IsNotExist(err) {
			t.Errorf("MAC directory should exist after migration: %s", macDir)
		}

		// 2. Serial directory should be gone
		if _, err := os.Stat(serialDir); !os.IsNotExist(err) {
			t.Errorf("Serial directory should not exist after migration: %s", serialDir)
		}

		// 3. Simulate SaveDeviceInfo with fresh data (like real discovery flow)
		// This overwrites DeviceInfo.xml with correct MAC-based deviceID
		freshDeviceInfo := &models.ServiceDeviceInfo{
			DeviceID:            macAddress,
			AccountID:           accountID,
			Name:                "Sound Speaker Fresh",
			IPAddress:           "192.168.1.100",
			MacAddress:          macAddress,
			DeviceSerialNumber:  serialNumber,
			ProductCode:         "SoundTouch 10 sm2",
			FirmwareVersion:     "3.4.6.2356",
			ProductSerialNumber: "069231P63364828AE",
			DiscoveryMethod:     "Migration Test",
		}
		if err := ds.SaveDeviceInfo(accountID, macAddress, freshDeviceInfo); err != nil {
			t.Errorf("Failed to save fresh device info: %v", err)
		}

		// 4. Device info should now have correct MAC-based deviceID
		macInfo, err := ds.GetDeviceInfo(accountID, macAddress)
		if err != nil {
			t.Errorf("Should be able to get device info with MAC ID: %v", err)
		} else if macInfo.DeviceID != macAddress {
			t.Errorf("DeviceID should be updated to MAC address, got %s", macInfo.DeviceID)
		}

		// 5. All data should be accessible through MAC address
		presets, err := ds.GetPresets(accountID, macAddress)
		if err != nil {
			t.Errorf("Should be able to get presets through MAC address: %v", err)
		} else if len(presets) != 1 || presets[0].Name != "My Preset" {
			t.Errorf("Presets should be preserved during migration")
		}

		t.Logf("✓ Device directory migration working correctly")
	})
}

func TestDeviceMigration_NoExistingTarget(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "no-target-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	srv := NewServer(ds, nil, "http://localhost", false, false, false, true, false)

	accountID := "test-account"
	macAddress := "A81B6A536A98"
	serialNumber := "I6332527703739342000020"

	// Create only serial-based device entry (no existing MAC directory)
	serialDeviceInfo := &models.ServiceDeviceInfo{
		DeviceID:           serialNumber,
		AccountID:          accountID,
		Name:               "Test Speaker",
		MacAddress:         macAddress,
		DeviceSerialNumber: serialNumber,
		ProductCode:        "SoundTouch 30",
	}

	if err := ds.SaveDeviceInfo(accountID, serialNumber, serialDeviceInfo); err != nil {
		t.Fatalf("Failed to save serial device: %v", err)
	}

	// Add some data files
	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:     "1",
				Name:   "Test Preset",
				Source: "SPOTIFY",
			},
		},
	}
	if err := ds.SavePresets(accountID, serialNumber, presets); err != nil {
		t.Fatalf("Failed to save presets: %v", err)
	}

	// Verify MAC directory doesn't exist initially
	macDir := ds.AccountDeviceDir(accountID, macAddress)
	if _, err := os.Stat(macDir); !os.IsNotExist(err) {
		t.Fatalf("MAC directory should not exist initially: %s", macDir)
	}

	// Migrate directory using migration manager
	existingDevices := []models.ServiceDeviceInfo{*serialDeviceInfo}
	srv.migrationManager.MigrateDevicesIfNeeded(existingDevices, macAddress)

	// Verify migration
	if _, err := os.Stat(macDir); os.IsNotExist(err) {
		t.Errorf("MAC directory should exist after migration: %s", macDir)
	}

	// Verify data is accessible
	migratedPresets, err := ds.GetPresets(accountID, macAddress)
	if err != nil {
		t.Errorf("Should be able to access presets after migration: %v", err)
	} else if len(migratedPresets) != 1 || migratedPresets[0].Name != "Test Preset" {
		t.Errorf("Presets should be preserved in migration")
	}

	t.Log("✓ Simple directory migration working correctly")
}

func TestDeviceMigration_ExistingTargetRemoved(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "existing-target-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	srv := NewServer(ds, nil, "http://localhost", false, false, false, true, false)

	accountID := "test-account"
	macAddress := "A81B6A536A98"
	serialNumber := "I6332527703739342000020"

	// Create both directories (serial has rich data, MAC has minimal data)
	serialInfo := &models.ServiceDeviceInfo{
		DeviceID:           serialNumber,
		AccountID:          accountID,
		Name:               "Rich Data Device",
		MacAddress:         macAddress,
		DeviceSerialNumber: serialNumber,
	}
	macInfo := &models.ServiceDeviceInfo{
		DeviceID:   macAddress,
		AccountID:  accountID,
		Name:       "Minimal Data Device",
		MacAddress: macAddress,
	}

	if err := ds.SaveDeviceInfo(accountID, serialNumber, serialInfo); err != nil {
		t.Fatalf("Failed to save serial device: %v", err)
	}
	if err := ds.SaveDeviceInfo(accountID, macAddress, macInfo); err != nil {
		t.Fatalf("Failed to save MAC device: %v", err)
	}

	// Add rich data to serial directory
	richPresets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:     "1",
				Name:   "Rich Preset",
				Source: "SPOTIFY",
			},
		},
	}
	if err := ds.SavePresets(accountID, serialNumber, richPresets); err != nil {
		t.Fatalf("Failed to save rich presets: %v", err)
	}

	// Migrate - should replace MAC directory with serial directory content
	existingDevices := []models.ServiceDeviceInfo{*serialInfo}
	srv.migrationManager.MigrateDevicesIfNeeded(existingDevices, macAddress)

	// Verify the rich data is now accessible via MAC address
	finalInfo, err := ds.GetDeviceInfo(accountID, macAddress)
	if err != nil {
		t.Errorf("Should be able to get device info after migration: %v", err)
	} else if finalInfo.Name != "Rich Data Device" {
		t.Errorf("Should have rich device data, got name: %s", finalInfo.Name)
	}

	finalPresets, err := ds.GetPresets(accountID, macAddress)
	if err != nil {
		t.Errorf("Should be able to get rich presets after migration: %v", err)
	} else if len(finalPresets) != 1 || finalPresets[0].Name != "Rich Preset" {
		t.Errorf("Should have rich presets after migration")
	}

	t.Log("✓ Migration correctly replaces existing target with richer source")
}
