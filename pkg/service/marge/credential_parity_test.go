package marge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestCredentialParity_LegacyAndNewFormat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "credential-parity-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "12345"
	device := "DEV123"
	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)

	// 1. Setup Sources.xml with BOTH legacy attribute and new element
	// This simulates what the datastore now produces.
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="1001" type="Audio" secret="legacy-token" secretType="token">
        <credential type="token">new-token</credential>
        <sourceKey type="SPOTIFY" account="user1" />
    </source>
    <source id="1002" type="Audio" secret="only-legacy" secretType="token">
        <sourceKey type="TUNEIN" account="user2" />
    </source>
    <source id="1003" type="Audio">
        <credential type="token_version_3">only-new</credential>
        <sourceKey type="SPOTIFY" account="user3" />
    </source>
</sources>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644)

	// 2. Verify GetConfiguredSources prioritizes new element
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	if len(sources) != 3 {
		t.Fatalf("Expected 3 sources, got %d", len(sources))
	}

	// Source 1001: should have "new-token"
	if sources[0].Secret != "new-token" {
		t.Errorf("Source 1001: expected secret 'new-token', got '%s'", sources[0].Secret)
	}

	// Source 1002: should have "only-legacy"
	if sources[1].Secret != "only-legacy" {
		t.Errorf("Source 1002: expected secret 'only-legacy', got '%s'", sources[1].Secret)
	}

	// Source 1003: should have "only-new" and "token_version_3"
	if sources[2].Secret != "only-new" {
		t.Errorf("Source 1003: expected secret 'only-new', got '%s'", sources[2].Secret)
	}
	if sources[2].SecretType != "token_version_3" {
		t.Errorf("Source 1003: expected secretType 'token_version_3', got '%s'", sources[2].SecretType)
	}

	// 3. Verify AccountFullToXML (API response) contains correct credential elements
	fullXML, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountFullToXML failed: %v", err)
	}

	xmlStr := string(fullXML)

	// Check 1001: should have new-token (Spotify with 'token' upgraded to 'token_version_3' in mapping)
	if !strings.Contains(xmlStr, `<source id="1001" type="Audio">`) {
		t.Errorf("Missing source 1001 in XML")
	}
	// Spotify with 'token' is upgraded to 'token_version_3' in mapToFullResponseSource
	if !strings.Contains(xmlStr, `<credential type="token_version_3">new-token</credential>`) {
		t.Errorf("Source 1001: missing expected credential. XML: %s", xmlStr)
	}

	// Check 1002: should have only-legacy
	if !strings.Contains(xmlStr, `<source id="1002" type="Audio">`) {
		t.Errorf("Missing source 1002 in XML")
	}
	if !strings.Contains(xmlStr, `<credential type="token">only-legacy</credential>`) {
		t.Errorf("Source 1002: missing expected credential. XML: %s", xmlStr)
	}

	// Check 1003: should have only-new
	if !strings.Contains(xmlStr, `<source id="1003" type="Audio">`) {
		t.Errorf("Missing source 1003 in XML")
	}
	if !strings.Contains(xmlStr, `<credential type="token_version_3">only-new</credential>`) {
		t.Errorf("Source 1003: missing expected credential. XML: %s", xmlStr)
	}
}

func TestAccountFullToXML_RecentsCredentialConsistency(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "recents-consistency-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "12345"
	device := "DEV123"
	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)

	// 1. Setup Sources.xml
	// 9330201 comes first and matches type "Audio" but has NO token.
	// 14774275 comes later and matches the sourceid exactly and HAS token.
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="9330201" type="Audio">
        <credential type="token"></credential>
        <sourceKey type="Audio" account=""></sourceKey>
    </source>
    <source id="14774275" secret="token-value" secretType="token" type="Audio">
        <credential type="token">token-value</credential>
        <sourceKey type="Audio" account=""></sourceKey>
    </source>
</sources>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644)

	// 2. Setup Recents.xml
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="2270445222">
        <contentItem source="Audio" type="" location="/v1/playback/episodes/t104218136" sourceAccount="" isPresetable="">
            <itemName>Atemlos durch die Charts</itemName>
        </contentItem>
        <createdOn>2019-07-29T15:29:59.000+00:00</createdOn>
        <updatedOn>2019-07-29T15:29:59.000+00:00</updatedOn>
        <lastplayedat>2019-07-29T11:29:54.000+00:00</lastplayedat>
        <sourceid>14774275</sourceid>
        <username>Atemlos durch die Charts</username>
    </recent>
</recents>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644)

	// Setup DeviceInfo.xml so CreateAccountDevice works
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEV123">
	<name>Test Device</name>
	<type>SoundTouch 10</type>
</info>`
	_ = os.WriteFile(filepath.Join(deviceDir, "DeviceInfo.xml"), []byte(deviceInfoXML), 0644)

	// 3. Verify RecentsToXML (used by /recents)
	recentsBytes, err := RecentsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("RecentsToXML failed: %v", err)
	}
	recentsStr := string(recentsBytes)
	// t.Logf("Recents XML: %s", recentsStr)
	if !strings.Contains(recentsStr, `<sourceid>14774275</sourceid>`) {
		t.Errorf("/recents response should have sourceid 14774275. XML: %s", recentsStr)
	}
	if !strings.Contains(recentsStr, `<credential type="token">token-value</credential>`) {
		t.Errorf("/recents response missing credential. XML: %s", recentsStr)
	}

	// 4. Verify AccountFullToXML (used by /full)
	fullBytes, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountFullToXML failed: %v", err)
	}
	fullStr := string(fullBytes)
	// t.Logf("Full XML: %s", fullStr)
	// In AccountFullToXML, recents are grouped under devices
	if !strings.Contains(fullStr, `<recent id="2270445222">`) {
		t.Errorf("/full response missing recent item. XML: %s", fullStr)
	}
	if !strings.Contains(fullStr, `<sourceid>14774275</sourceid>`) {
		t.Errorf("/full response should have sourceid 14774275. XML: %s", fullStr)
	}
	if !strings.Contains(fullStr, `<credential type="token">token-value</credential>`) {
		t.Errorf("/full response missing credential. XML: %s", fullStr)
	}
}

func TestAccountSourcesToXML_CredentialParity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sources-parity-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "123"
	device := "ABC"

	src := models.ConfiguredSource{
		ID:         "2001",
		Secret:     "secret-val",
		SecretType: "token_version_3",
	}
	src.SourceKey.Type = "SPOTIFY"
	src.SourceKey.Account = "user1"

	_ = ds.SaveConfiguredSources(account, device, []models.ConfiguredSource{src})

	xmlData, err := AccountSourcesToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountSourcesToXML failed: %v", err)
	}

	xmlStr := string(xmlData)
	if !strings.Contains(xmlStr, `<credential type="token_version_3">secret-val</credential>`) {
		t.Errorf("AccountSourcesToXML missing expected credential element. Got: %s", xmlStr)
	}
}

func TestAccountFullToXML_RecentsCredentialParity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "recents-parity-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "123"
	device := "ABC"
	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)

	// 0. Setup DeviceInfo.xml (required for CreateAccountDevice)
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="ABC">
    <name>Test Device</name>
    <type>SoundTouch 10</type>
    <components>
        <component componentCategory="SCM">
            <softwareVersion>1.2.3</softwareVersion>
            <serialNumber>ABC123</serialNumber>
        </component>
    </components>
</info>`
	_ = os.WriteFile(filepath.Join(deviceDir, "DeviceInfo.xml"), []byte(deviceInfoXML), 0644)

	// 1. Setup Sources.xml
	src := models.ConfiguredSource{
		ID:         "3001",
		Secret:     "recent-token",
		SecretType: "token_version_3",
	}
	src.SourceKey.Type = "SPOTIFY"
	src.SourceKey.Account = "user-recent"
	_ = ds.SaveConfiguredSources(account, device, []models.ConfiguredSource{src})

	// 2. Setup Recents.xml
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="1" deviceID="ABC" utcTime="123456789">
        <contentItem source="SPOTIFY" type="track" location="spotify:track:123" sourceAccount="user-recent" isPresetable="true" itemName="Recent Track" />
    </recent>
</recents>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644)

	// 3. Generate Account Full XML
	fullXML, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountFullToXML failed: %v", err)
	}

	xmlStr := string(fullXML)

	// 4. Verify that the recent item has the credential populated via its source
	// The recent item's source should be mapped from the configured source with ID 3001 or matching source/account.
	if !strings.Contains(xmlStr, `<recent id="1">`) {
		t.Errorf("Missing recent 1 in XML. Got: %s", xmlStr)
	}

	// This is what is currently missing according to the issue.
	// We need to check if the <recent> element's nested <source> has the <credential>.
	// Simple way to check: is there at least TWO occurrences of the credential?
	// One in <sources><source> and one in <recents><recent><source>.
	count := strings.Count(xmlStr, `<credential type="token_version_3">recent-token</credential>`)
	if count < 2 {
		t.Errorf("Recent item likely missing expected credential element. Count: %d, XML: %s", count, xmlStr)
	}
}

func TestAccountFullToXML_RecentsSourceAccountMatching(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "recents-matching-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "123"
	device := "ABC"
	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)

	// 0. Setup DeviceInfo.xml
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="ABC">
    <name>Test Device</name>
    <components><component componentCategory="SCM"><serialNumber>ABC123</serialNumber></component></components>
</info>`
	_ = os.WriteFile(filepath.Join(deviceDir, "DeviceInfo.xml"), []byte(deviceInfoXML), 0644)

	// 1. Setup TWO Spotify sources with different accounts
	src1 := models.ConfiguredSource{
		ID:         "101",
		Secret:     "token-1",
		SecretType: "token_version_3",
	}
	src1.SourceKey.Type = "SPOTIFY"
	src1.SourceKey.Account = "user-1"

	src2 := models.ConfiguredSource{
		ID:         "202",
		Secret:     "token-2",
		SecretType: "token_version_3",
	}
	src2.SourceKey.Type = "SPOTIFY"
	src2.SourceKey.Account = "user-2"

	_ = ds.SaveConfiguredSources(account, device, []models.ConfiguredSource{src1, src2})

	// 2. Setup Recents.xml with a Spotify recent for user-2
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="1" deviceID="ABC" utcTime="123456789">
        <contentItem source="SPOTIFY" type="track" location="spotify:track:123" sourceAccount="user-2" isPresetable="true" itemName="User 2 Track" />
    </recent>
</recents>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644)

	// 3. Generate Account Full XML
	fullXML, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountFullToXML failed: %v", err)
	}

	xmlStr := string(fullXML)

	// 4. Verify that the recent item matches source 202 (user-2) and HAS token-2
	if !strings.Contains(xmlStr, `<recent id="1">`) {
		t.Fatalf("Missing recent 1")
	}

	// It should have token-2. If it picked src1 by mistake, it would have token-1.
	if !strings.Contains(xmlStr, `<credential type="token_version_3">token-2</credential>`) {
		t.Errorf("Recent item missing expected credential element (token-2). XML: %s", xmlStr)
	}

	// Total count: token-1 (once in sources), token-2 (once in sources, once in recents)
	if strings.Count(xmlStr, `token-2`) < 2 {
		t.Errorf("token-2 should appear twice (source list and recent). XML: %s", xmlStr)
	}
}

func TestAccountFullToXML_ContentItemTypeParity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "content-item-type-parity-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "123"
	device := "ABC"
	deviceDir := ds.AccountDeviceDir(account, device)
	_ = os.MkdirAll(deviceDir, 0755)

	// 0. Setup DeviceInfo.xml
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="ABC">
    <name>Test Device</name>
    <type>SoundTouch 10</type>
    <components><component componentCategory="SCM"><serialNumber>ABC123</serialNumber></component></components>
</info>`
	_ = os.WriteFile(filepath.Join(deviceDir, "DeviceInfo.xml"), []byte(deviceInfoXML), 0644)

	// 1. Setup Sources.xml
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="100" type="TUNEIN">
        <credential type="token"></credential>
        <sourceKey type="TUNEIN" account=""></sourceKey>
    </source>
</sources>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644)

	// 2. Setup Presets.xml and Recents.xml with contentItem elements
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1" createdOn="123456789" updatedOn="123456789">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/stations/s166521" sourceAccount="" isPresetable="true">
            <itemName>Station Name</itemName>
        </contentItem>
    </preset>
</presets>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(presetsXML), 0644)

	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="1" deviceID="ABC" utcTime="123456789">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/stations/s166521" sourceAccount="" isPresetable="true">
            <itemName>Station Name</itemName>
        </contentItem>
    </recent>
</recents>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644)

	// 3. Generate Account Full XML
	fullXML, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountFullToXML failed: %v", err)
	}

	xmlStr := string(fullXML)

	// 4. Verify that contentItemType is present and matches the contentItem's type
	if !strings.Contains(xmlStr, `<contentItemType>stationurl</contentItemType>`) {
		t.Errorf("Missing expected <contentItemType>stationurl</contentItemType> in XML. XML: %s", xmlStr)
	}

	// It should appear twice: once in preset, once in recent
	count := strings.Count(xmlStr, `<contentItemType>stationurl</contentItemType>`)
	if count != 2 {
		t.Errorf("Expected <contentItemType>stationurl</contentItemType> to appear twice, got %d. XML: %s", count, xmlStr)
	}

	// Verify itemName is present
	if !strings.Contains(xmlStr, `<name>Station Name</name>`) {
		t.Errorf("Missing expected <name>Station Name</name> in XML. XML: %s", xmlStr)
	}

	// Verify location is present
	if !strings.Contains(xmlStr, `<location>/v1/playback/stations/s166521</location>`) {
		t.Errorf("Missing expected <location>/v1/playback/stations/s166521</location> in XML. XML: %s", xmlStr)
	}
}
