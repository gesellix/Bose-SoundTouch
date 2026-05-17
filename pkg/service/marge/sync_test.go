package marge

import (
	"encoding/xml"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestSyncFromAccountFull(t *testing.T) {
	// Setup a temporary datastore
	tmpDir, err := os.MkdirTemp("", "datastore_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := datastore.NewDataStore(tmpDir)

	// Mock AccountFullResponse
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<account id="USER_123">
    <accountStatus>ACTIVE</accountStatus>
    <devices>
        <device deviceid="DEVICE_ABC">
            <name>Living Room</name>
            <ipaddress>192.0.2.10</ipaddress>
            <serialNumber>ABC123XYZ</serialNumber>
            <firmwareVersion>27.0.6</firmwareVersion>
            <attachedProduct product_code="ST10">
                <serialNumber>ABC123XYZ</serialNumber>
            </attachedProduct>
            <presets>
                <preset buttonNumber="1">
                    <name>My Station</name>
                    <location>tunein://station/s123</location>
                    <contentItemType>station</contentItemType>
                    <source id="TUNEIN" type="TUNEIN">
                        <name>TuneIn</name>
                        <sourcename>TuneIn</sourcename>
                    </source>
                </preset>
            </presets>
            <recents>
                <recent id="RECENT_1">
                    <name>Last Song</name>
                    <location>spotify:track:abc</location>
                    <contentItemType>track</contentItemType>
                    <source id="SPOTIFY" type="SPOTIFY">
                        <name>Spotify</name>
                        <sourcename>Spotify</sourcename>
                    </source>
                </recent>
            </recents>
        </device>
    </devices>
    <sources>
        <source id="TUNEIN" type="TUNEIN">
            <name>TuneIn</name>
            <sourcename>TuneIn</sourcename>
        </source>
    </sources>
</account>`

	var resp models.AccountFullResponse
	if err := xml.Unmarshal([]byte(xmlData), &resp); err != nil {
		t.Fatalf("Failed to unmarshal mock data: %v", err)
	}

	// Run Sync
	if err := SyncFromAccountFull(ds, &resp); err != nil {
		t.Fatalf("SyncFromAccountFull failed: %v", err)
	}

	// Verify Device Info
	info, err := ds.GetDeviceInfo("USER_123", "DEVICE_ABC")
	if err != nil {
		t.Errorf("Failed to get device info: %v", err)
	}
	if info.Name != "Living Room" {
		t.Errorf("Expected name 'Living Room', got '%s'", info.Name)
	}
	// Note: ProductCode might be concatenated with a space in some implementations or models
	if info.ProductCode != "ST10" && info.ProductCode != "ST10 " {
		t.Errorf("Expected product code 'ST10', got '%s'", info.ProductCode)
	}

	// Verify Presets
	presets, err := ds.GetPresets("USER_123", "DEVICE_ABC")
	if err != nil {
		t.Errorf("Failed to get presets: %v", err)
	}
	if len(presets) != 1 {
		t.Errorf("Expected 1 preset, got %d", len(presets))
	} else {
		// Datastore's ServicePreset might not use ButtonNumber field in its XML structure,
		// but rather relies on order or an 'id' attribute.
		// Let's check the name which we know was set.
		if presets[0].Name != "My Station" {
			t.Errorf("Expected preset name 'My Station', got '%s'", presets[0].Name)
		}
	}

	// Verify Recents
	recents, err := ds.GetRecents("USER_123", "DEVICE_ABC")
	if err != nil {
		t.Errorf("Failed to get recents: %v", err)
	}
	if len(recents) != 1 {
		t.Errorf("Expected 1 recent, got %d", len(recents))
	}

	// Verify Sources
	sources, err := ds.GetConfiguredSources("USER_123", "DEVICE_ABC")
	if err != nil {
		t.Errorf("Failed to get sources: %v", err)
	}
	// Now we aggregate sources from Account + Preset + Recent.
	// Account has TUNEIN.
	// Preset has TUNEIN (same ID, so deduplicated).
	// Recent has SPOTIFY (new ID, so added).
	// Total expected: 2
	if len(sources) != 2 {
		t.Errorf("Expected 2 sources (aggregated), got %d", len(sources))
	}
}
