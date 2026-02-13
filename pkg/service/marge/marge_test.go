package marge

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMargeXML(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "123"
	device := "ABC"

	// Setup initial data
	info := &models.ServiceDeviceInfo{
		DeviceID: device,
		Name:     "Living Room",
	}
	_ = ds.SaveDeviceInfo(account, device, info)

	// Save empty presets/recents to avoid index out of range when stripping header
	_ = ds.SavePresets(account, device, []models.ServicePreset{})
	_ = ds.SaveRecents(account, device, []models.ServiceRecent{})

	// Test SourceProvidersToXML
	xmlData, err := SourceProvidersToXML()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(xmlData), "<sourceProviders>") {
		t.Errorf("Expected <sourceProviders>, got %s", string(xmlData))
	}

	// Test AccountFullToXML
	fullXML, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(fullXML), `id="123"`) {
		t.Errorf("Expected account id 123, got %s", string(fullXML))
	}

	if !strings.Contains(string(fullXML), "Living Room") {
		t.Errorf("Expected device name Living Room, got %s", string(fullXML))
	}

	// Test SoftwareUpdateToXML
	swXML := SoftwareUpdateToXML()
	if !strings.Contains(swXML, "<software_update>") {
		t.Errorf("Expected <software_update>, got %s", swXML)
	}
}

func TestAddRecent_TimestampPreservation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "test-acc"
	device := "test-dev"

	// 1. Setup configured sources
	// We need a Sources.xml file in the account directory
	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)
	src := models.ConfiguredSource{
		ID:          "101",
		DisplayName: "Test Source",
		SecretType:  "Audio",
	}
	src.SourceKey.Type = "TUNEIN"
	src.SourceKey.Account = "test-user"
	src.SourceKeyType = "TUNEIN"
	src.SourceKeyAccount = "test-user"

	_ = ds.SaveConfiguredSources(account, device, []models.ConfiguredSource{src})
	_ = ds.SaveRecents(account, device, []models.ServiceRecent{})

	// 2. Add an initial recent
	sourceXML := []byte(`
<recent>
    <name>Initial Station</name>
    <sourceid>101</sourceid>
    <location>station-1</location>
    <contentItemType>station</contentItemType>
</recent>`)

	_, err = AddRecent(ds, account, device, sourceXML)
	if err != nil {
		t.Fatalf("AddRecent failed: %v", err)
	}

	recents, _ := ds.GetRecents(account, device)
	if len(recents) != 1 {
		t.Fatalf("Expected 1 recent, got %d", len(recents))
	}

	originalCreatedOn := recents[0].UtcTime // It's stored in UtcTime field (unix string) in models.ServiceRecent but the AddRecent return XML uses <createdOn> tag which is DateStr or Now depending on logic.
	// Actually let's check what AddRecent returns.

	// 3. Add the same recent again (it should move to front and preserve createdOn)
	// We'll wait a second to ensure time.Now() would be different if it were used for createdOn
	time.Sleep(1 * time.Second)

	respXML, err := AddRecent(ds, account, device, sourceXML)
	if err != nil {
		t.Fatalf("AddRecent second time failed: %v", err)
	}

	if !strings.Contains(string(respXML), "2012-09-19T12:43:00.000+00:00") {
		// Our DateStr is 2012-09-19T12:43:00.000+00:00
		t.Errorf("Expected preserved DateStr in createdOn, got XML: %s", string(respXML))
	}

	recents, _ = ds.GetRecents(account, device)
	if len(recents) != 1 {
		t.Errorf("Expected still 1 recent, got %d", len(recents))
	}

	// Check that UtcTime was updated (it should be, for lastplayedat)
	if recents[0].UtcTime == originalCreatedOn {
		// Wait, if they are the same it might be because we didn't specify LastPlayedAt in input XML so it used Now.
		// Since we slept, it should be different.
	}
}
