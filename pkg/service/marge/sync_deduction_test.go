package marge

import (
	"encoding/xml"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestSyncFromAccountFull_DeduceIDs(t *testing.T) {
	// Setup a temporary datastore
	tmpDir, err := os.MkdirTemp("", "sync_deduce_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := datastore.NewDataStore(tmpDir)
	accountID := "USER_123"
	deviceID := "DEVICE_ABC"

	// Mock AccountFullResponse with generic source IDs (e.g., from a fresh sync or default mapping)
	// and specific source IDs in presets/recents that we want to "deduce" and use.
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<account id="USER_123">
    <devices>
        <device deviceid="DEVICE_ABC">
            <presets>
                <preset buttonNumber="1">
                    <contentItem itemName="Lounge FM Digital" location="52349" source="INTERNET_RADIO" type="station" />
                    <source id="9330201" type="Audio">
                        <sourceproviderid>2</sourceproviderid>
                    </source>
                    <sourceid>9330201</sourceid>
                </preset>
            </presets>
            <recents>
                <recent id="RECENT_1">
                    <contentItem itemName="TuneIn Station" source="TUNEIN" type="station" />
                    <source id="DEDUCED_TUNEIN" type="Audio">
                        <sourceproviderid>25</sourceproviderid>
                    </source>
                    <sourceid>DEDUCED_TUNEIN</sourceid>
                </recent>
            </recents>
        </device>
    </devices>
    <sources>
        <source id="GENERIC_2" type="Audio" sourceproviderid="2" />
        <source id="GENERIC_25" type="Audio" sourceproviderid="25" />
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

	// Verify Sources
	sources, err := ds.GetConfiguredSources(accountID, deviceID)
	if err != nil {
		t.Errorf("Failed to get sources: %v", err)
	}

	found2 := false
	found25 := false
	for _, s := range sources {
		if s.SourceProviderID == "2" {
			if s.ID == "9330201" {
				found2 = true
			} else {
				t.Errorf("Expected source ID 9330201 for provider 2, got %s", s.ID)
			}
		}
		if s.SourceProviderID == "25" {
			if s.ID == "DEDUCED_TUNEIN" {
				found25 = true
			} else {
				t.Errorf("Expected source ID DEDUCED_TUNEIN for provider 25, got %s", s.ID)
			}
		}
	}

	if !found2 {
		t.Errorf("Did not find source with provider ID 2 and deducted ID 9330201")
	}
	if !found25 {
		t.Errorf("Did not find source with provider ID 25 and deducted ID DEDUCED_TUNEIN")
	}
}
