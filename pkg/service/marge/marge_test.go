package marge

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestEscapeXML(t *testing.T) {
	input := "Antenne Chillout & Other"
	expected := "Antenne Chillout &amp; Other"
	actual := EscapeXML(input)
	if actual != expected {
		t.Errorf("Expected %s, got %s", expected, actual)
	}

	inputWithAll := "< > & ' \""
	expectedWithAll := "&lt; &gt; &amp; &#39; &#34;"
	actualWithAll := EscapeXML(inputWithAll)
	if actualWithAll != expectedWithAll {
		t.Errorf("Expected %s, got %s", expectedWithAll, actualWithAll)
	}
}

func TestRecentsXML_EmptyIDFix(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "test-acc"
	device := "test-dev"

	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)

	// Create a Recents.xml with empty ID
	recentsXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="" deviceID="test-dev" utcTime="1708896000">
        <contentItem source="SPOTIFY" type="tracklisturl" location="/test" sourceAccount="user" isPresetable="true">
            <itemName>Test Item</itemName>
        </contentItem>
    </recent>
</recents>`)
	_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), recentsXML, 0644)
	_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte("<sources/>"), 0644)

	// Fetching should fix the empty ID
	recents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("Failed to get recents: %v", err)
	}

	if len(recents) != 1 {
		t.Fatalf("Expected 1 recent, got %d", len(recents))
	}

	if recents[0].ID == "" {
		t.Errorf("Expected non-empty ID for recent")
	}

	if _, err := strconv.Atoi(recents[0].ID); err != nil {
		t.Errorf("Expected numeric ID, got %s", recents[0].ID)
	}

	// Verify the XML output also has the non-empty ID
	xmlData, err := RecentsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("RecentsToXML failed: %v", err)
	}

	if strings.Contains(string(xmlData), `recent id=""`) {
		t.Errorf("XML should not contain empty recent ID: %s", string(xmlData))
	}

	if !strings.Contains(string(xmlData), `recent id="1"`) {
		t.Errorf("XML should contain fixed numeric ID: %s", string(xmlData))
	}
}

func TestGetConfiguredSourceXML_Escaping(t *testing.T) {
	src := models.ConfiguredSource{
		ID:          "101&202",
		DisplayName: "Test & Source",
		Secret:      "key&value",
	}
	src.SourceKeyAccount = "user&name"

	xml := GetConfiguredSourceXML(src)
	if !strings.Contains(xml, "id=\"101&amp;202\"") {
		t.Errorf("ID not escaped in attribute: %s", xml)
	}
	if strings.Contains(xml, "<sourceid>101&amp;202</sourceid>") {
		t.Errorf("ID should not be escaped in sourceid tag inside source tag anymore: %s", xml)
	}
	if !strings.Contains(xml, "<sourcename>Test &amp; Source</sourcename>") {
		t.Errorf("DisplayName not escaped: %s", xml)
	}
	if !strings.Contains(xml, ">key&amp;value</credential>") {
		t.Errorf("Secret not escaped: %s", xml)
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

	// Verify that sourceid is present in recent response and is a sibling to source tag
	if !strings.Contains(string(respXML), "<sourceid>101</sourceid>") {
		t.Errorf("Expected sourceid in recent response: %s", string(respXML))
	}
	if strings.Contains(string(respXML), "<source id=\"101\" type=\"Audio\"><createdOn>2012-09-19T12:43:00.000+00:00</createdOn><credential type=\"token\">key&amp;value</credential><name>test-user</name><sourceid>101</sourceid>") {
		t.Errorf("sourceid should not be inside source tag: %s", string(respXML))
	}
}
