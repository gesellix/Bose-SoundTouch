package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMigration_FilePreservation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration-file-preservation-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	manager := NewManager(ds, Config{Enabled: true, DryRun: false})

	accountID := "test-account"
	oldDeviceID := "I6332527703739342000020" // Legacy serial number
	newDeviceID := "A81B6A536A98"            // MAC address

	// 1. Create old device directory with multiple files
	oldDir := ds.AccountDeviceDir(accountID, oldDeviceID)
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create DeviceInfo.xml with serial number as deviceID (legacy format)
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="I6332527703739342000020">
    <name>Sound Speaker Legacy</name>
    <type>SoundTouch 10</type>
    <moduleType>sm2</moduleType>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>3.4.6.2356</softwareVersion>
            <serialNumber>I6332527703739342000020</serialNumber>
        </component>
        <component>
            <componentCategory>PackagedProduct</componentCategory>
            <serialNumber>069231P63364828AE</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>A81B6A536A98</macAddress>
        <ipAddress>192.168.1.100</ipAddress>
    </networkInfo>
</info>`

	if err := os.WriteFile(filepath.Join(oldDir, "DeviceInfo.xml"), []byte(deviceInfoXML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Presets.xml
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1">
        <ContentItem source="SPOTIFY" location="spotify://track/123">
            <itemName>Test Song</itemName>
        </ContentItem>
    </preset>
</presets>`

	if err := os.WriteFile(filepath.Join(oldDir, "Presets.xml"), []byte(presetsXML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Sources.xml
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="123" displayName="SPOTIFY" secret="token123" secretType="Audio">
        <accountDisplayName>user@example.com</accountDisplayName>
    </source>
</sources>`

	if err := os.WriteFile(filepath.Join(oldDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Recents.xml
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent>
        <ContentItem source="TUNEIN" location="tunein://station/s12345">
            <itemName>BBC Radio 1</itemName>
        </ContentItem>
    </recent>
</recents>`

	if err := os.WriteFile(filepath.Join(oldDir, "Recents.xml"), []byte(recentsXML), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Verify old directory has all files
	oldEntries, err := os.ReadDir(oldDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldEntries) != 4 {
		t.Fatalf("Expected 4 files in old directory, got %d", len(oldEntries))
	}

	t.Logf("Before migration - Old directory (%s) contains %d files:", oldDeviceID, len(oldEntries))
	for _, entry := range oldEntries {
		t.Logf("  - %s", entry.Name())
	}

	// 3. Create device info for migration
	existingDevice := models.ServiceDeviceInfo{
		DeviceID:  oldDeviceID,
		AccountID: accountID,
		Name:      "Sound Speaker Legacy",
		IPAddress: "192.168.1.100",
	}

	// 4. Perform migration
	migrated := manager.MigrateDevicesIfNeeded([]models.ServiceDeviceInfo{existingDevice}, newDeviceID)
	if !migrated {
		t.Fatal("Migration should have occurred")
	}

	// 5. Verify old directory no longer exists
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("Old directory should not exist after migration")
	}

	// 6. Verify new directory exists with all files preserved
	newDir := ds.AccountDeviceDir(accountID, newDeviceID)
	newEntries, err := os.ReadDir(newDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(newEntries) != 4 {
		t.Fatalf("Expected 4 files in new directory after migration, got %d", len(newEntries))
	}

	t.Logf("After migration - New directory (%s) contains %d files:", newDeviceID, len(newEntries))
	for _, entry := range newEntries {
		t.Logf("  - %s", entry.Name())
	}

	// 7. Verify each file exists and has content
	expectedFiles := []string{"DeviceInfo.xml", "Presets.xml", "Sources.xml", "Recents.xml"}
	for _, filename := range expectedFiles {
		filePath := filepath.Join(newDir, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("File %s should exist after migration: %v", filename, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("File %s should not be empty after migration", filename)
		}
		t.Logf("  ✓ %s preserved (%d bytes)", filename, len(data))
	}

	// 8. Verify specific content preservation
	// Presets should still contain the test song
	presetsData, _ := os.ReadFile(filepath.Join(newDir, "Presets.xml"))
	if !containsString(string(presetsData), "Test Song") {
		t.Error("Presets.xml should preserve original content")
	}

	// Sources should still contain the Spotify account
	sourcesData, _ := os.ReadFile(filepath.Join(newDir, "Sources.xml"))
	if !containsString(string(sourcesData), "user@example.com") {
		t.Error("Sources.xml should preserve original content")
	}

	// Recents should still contain the radio station
	recentsData, _ := os.ReadFile(filepath.Join(newDir, "Recents.xml"))
	if !containsString(string(recentsData), "BBC Radio 1") {
		t.Error("Recents.xml should preserve original content")
	}

	t.Log("✅ Migration successfully preserved all files with their original content")
}

func TestMigration_DryRun(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration-dry-run-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	manager := NewManager(ds, Config{Enabled: true, DryRun: true}) // DRY RUN MODE

	accountID := "test-account"
	oldDeviceID := "I6332527703739342000020"
	newDeviceID := "A81B6A536A98"

	// Create old directory with files
	oldDir := ds.AccountDeviceDir(accountID, oldDeviceID)
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}

	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="I6332527703739342000020">
    <name>Test Device</name>
</info>`

	if err := os.WriteFile(filepath.Join(oldDir, "DeviceInfo.xml"), []byte(deviceInfoXML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create device info for migration
	existingDevice := models.ServiceDeviceInfo{
		DeviceID:  oldDeviceID,
		AccountID: accountID,
	}

	// Perform dry-run migration
	migrated := manager.MigrateDevicesIfNeeded([]models.ServiceDeviceInfo{existingDevice}, newDeviceID)
	if migrated {
		t.Error("Dry run should not report actual migration")
	}

	// Verify old directory still exists (no actual migration)
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		t.Error("Old directory should still exist after dry run")
	}

	// Verify new directory does not exist
	newDir := ds.AccountDeviceDir(accountID, newDeviceID)
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Error("New directory should not exist after dry run")
	}

	t.Log("✅ Dry run mode correctly simulated migration without making changes")
}

func TestMigration_Disabled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration-disabled-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	manager := NewManager(ds, Config{Enabled: false, DryRun: false}) // MIGRATION DISABLED

	accountID := "test-account"
	oldDeviceID := "I6332527703739342000020"
	newDeviceID := "A81B6A536A98"

	// Create device info for migration
	existingDevice := models.ServiceDeviceInfo{
		DeviceID:  oldDeviceID,
		AccountID: accountID,
	}

	// Attempt migration
	migrated := manager.MigrateDevicesIfNeeded([]models.ServiceDeviceInfo{existingDevice}, newDeviceID)
	if migrated {
		t.Error("Migration should not occur when disabled")
	}

	t.Log("✅ Migration correctly disabled")
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMigration_CompleteFlowWithDeviceInfoUpdate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration-complete-flow-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	manager := NewManager(ds, Config{Enabled: true, DryRun: false})

	accountID := "test-account"
	oldDeviceID := "I6332527703739342000020" // Legacy serial number
	newDeviceID := "A81B6A536A98"            // MAC address

	// 1. Create old device directory with legacy DeviceInfo.xml (deviceID=serial)
	oldDir := ds.AccountDeviceDir(accountID, oldDeviceID)
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacyDeviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="I6332527703739342000020">
    <name>Sound Speaker Legacy</name>
    <type>SoundTouch 10</type>
    <moduleType>sm2</moduleType>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>3.4.6.2356</softwareVersion>
            <serialNumber>I6332527703739342000020</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>A81B6A536A98</macAddress>
        <ipAddress>192.168.1.100</ipAddress>
    </networkInfo>
</info>`

	if err := os.WriteFile(filepath.Join(oldDir, "DeviceInfo.xml"), []byte(legacyDeviceInfoXML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Presets.xml to verify preservation
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1">
        <ContentItem source="SPOTIFY" location="spotify://track/123">
            <itemName>My Favorite Song</itemName>
        </ContentItem>
    </preset>
</presets>`

	if err := os.WriteFile(filepath.Join(oldDir, "Presets.xml"), []byte(presetsXML), 0644); err != nil {
		t.Fatal(err)
	}

	t.Log("Step 1: Legacy directory created with deviceID=serial in DeviceInfo.xml")

	// 2. Perform migration
	existingDevice := models.ServiceDeviceInfo{
		DeviceID:  oldDeviceID,
		AccountID: accountID,
		Name:      "Sound Speaker Legacy",
		IPAddress: "192.168.1.100",
	}

	migrated := manager.MigrateDevicesIfNeeded([]models.ServiceDeviceInfo{existingDevice}, newDeviceID)
	if !migrated {
		t.Fatal("Migration should have occurred")
	}

	t.Log("Step 2: Migration completed - directory renamed, all files preserved")

	// 3. Verify migration moved files but preserved content
	newDir := ds.AccountDeviceDir(accountID, newDeviceID)

	// Check that Presets.xml was preserved
	preservedPresetsData, err := os.ReadFile(filepath.Join(newDir, "Presets.xml"))
	if err != nil {
		t.Fatalf("Presets.xml should be preserved after migration: %v", err)
	}
	if !containsString(string(preservedPresetsData), "My Favorite Song") {
		t.Error("Presets.xml content should be preserved")
	}

	t.Log("Step 3: Verified Presets.xml preserved during migration")

	// 4. Simulate SaveDeviceInfo with fresh /info data (like real discovery)
	// This should overwrite DeviceInfo.xml with correct MAC-based deviceID
	freshDeviceInfo := &models.ServiceDeviceInfo{
		DeviceID:            newDeviceID, // MAC address as deviceID
		AccountID:           accountID,
		Name:                "Sound Machinechen", // Fresh name from /info
		IPAddress:           "192.168.1.100",     // Fresh IP
		MacAddress:          newDeviceID,
		DeviceSerialNumber:  oldDeviceID,         // Serial goes in component
		ProductCode:         "SoundTouch 10 sm2", // Fresh product info
		FirmwareVersion:     "27.0.6.46330.5043500",
		ProductSerialNumber: "069231P63364828AE",
		DiscoveryMethod:     "Test Discovery",
	}

	if err := ds.SaveDeviceInfo(accountID, newDeviceID, freshDeviceInfo); err != nil {
		t.Fatalf("Failed to save fresh device info: %v", err)
	}

	t.Log("Step 4: SaveDeviceInfo called with fresh /info data")

	// 5. Verify DeviceInfo.xml now has correct MAC-based deviceID
	updatedDeviceInfoData, err := os.ReadFile(filepath.Join(newDir, "DeviceInfo.xml"))
	if err != nil {
		t.Fatalf("DeviceInfo.xml should exist after SaveDeviceInfo: %v", err)
	}

	updatedXML := string(updatedDeviceInfoData)

	// Should contain deviceID="A81B6A536A98" (MAC address)
	if !containsString(updatedXML, `deviceID="A81B6A536A98"`) {
		t.Errorf("DeviceInfo.xml should have deviceID set to MAC address, content:\n%s", updatedXML)
	}

	// Should contain fresh device name from /info
	if !containsString(updatedXML, "Sound Machinechen") {
		t.Errorf("DeviceInfo.xml should have fresh device name from /info")
	}

	// Should contain serial number in component (not as deviceID)
	if !containsString(updatedXML, "I6332527703739342000020") {
		t.Errorf("DeviceInfo.xml should still contain serial number in component")
	}

	// Should contain product serial in component
	if !containsString(updatedXML, "069231P63364828AE") {
		t.Errorf("DeviceInfo.xml should contain product serial in component")
	}

	t.Log("Step 5: Verified DeviceInfo.xml has correct MAC-based deviceID attribute")

	// 6. Verify Presets.xml still exists and wasn't overwritten
	finalPresetsData, err := os.ReadFile(filepath.Join(newDir, "Presets.xml"))
	if err != nil {
		t.Fatalf("Presets.xml should still exist after SaveDeviceInfo: %v", err)
	}
	if !containsString(string(finalPresetsData), "My Favorite Song") {
		t.Error("Presets.xml should not be overwritten by SaveDeviceInfo")
	}

	t.Log("Step 6: Verified Presets.xml was not overwritten by SaveDeviceInfo")

	// 7. Verify data is accessible via MAC address
	retrievedInfo, err := ds.GetDeviceInfo(accountID, newDeviceID)
	if err != nil {
		t.Fatalf("Should be able to retrieve device info by MAC address: %v", err)
	}

	if retrievedInfo.DeviceID != newDeviceID {
		t.Errorf("Retrieved device info should have MAC-based deviceID, got %s", retrievedInfo.DeviceID)
	}

	if retrievedInfo.Name != "Sound Machinechen" {
		t.Errorf("Retrieved device info should have fresh name, got %s", retrievedInfo.Name)
	}

	t.Log("Step 7: Verified device info retrieval works with MAC address")

	t.Log("✅ Complete migration flow verified:")
	t.Log("   1. Migration preserves all files (Presets.xml, Sources.xml, etc.)")
	t.Log("   2. SaveDeviceInfo updates DeviceInfo.xml with correct deviceID=MAC")
	t.Log("   3. Serial number preserved in component, not as deviceID")
	t.Log("   4. Fresh /info data properly integrated")
	t.Log("   5. User data (presets, etc.) completely preserved")
}
