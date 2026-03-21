package datastore

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestSaveRecents_Format(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore_recents_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)

	account := "test-account"
	device := "test-device"

	recents := []models.ServiceRecent{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:            "2567119953",
				Name:          "The National",
				Source:        "SPOTIFY",
				Type:          "tracklisturl",
				Location:      "/playback/container/c3BvdGlmeTp1c2VyOnRlc3QtdXNlcjpjb2xsZWN0aW9uOmFydGlzdDoyY0NVdEdLOXNEVTJFb0VsbmswR05C",
				SourceAccount: "test-user",
				IsPresetable:  "true",
			},
			DeviceID: "001122334455",
			UtcTime:  "1771666755",
		},
	}

	if err := ds.SaveRecents(account, device, recents); err != nil {
		t.Fatalf("SaveRecents failed: %v", err)
	}

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Recents.xml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read Recents.xml: %v", err)
	}

	expectedXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent deviceID="001122334455" utcTime="1771666755" id="2567119953">
        <contentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/c3BvdGlmeTp1c2VyOnRlc3QtdXNlcjpjb2xsZWN0aW9uOmFydGlzdDoyY0NVdEdLOXNEVTJFb0VsbmswR05C" sourceAccount="test-user" isPresetable="true">
            <itemName>The National</itemName>
        </contentItem>
    </recent>
</recents>`

	// Normalize whitespace for comparison by unmarshaling both
	var expected, actual struct {
		XMLName xml.Name `xml:"recents"`
		Recents []struct {
			DeviceID    string `xml:"deviceID,attr"`
			UtcTime     string `xml:"utcTime,attr"`
			ID          string `xml:"id,attr"`
			ContentItem struct {
				Source        string `xml:"source,attr"`
				Type          string `xml:"type,attr"`
				Location      string `xml:"location,attr"`
				SourceAccount string `xml:"sourceAccount,attr"`
				IsPresetable  string `xml:"isPresetable,attr"`
				ItemName      string `xml:"itemName"`
			} `xml:"contentItem"`
		} `xml:"recent"`
	}

	if err := xml.Unmarshal([]byte(expectedXML), &expected); err != nil {
		t.Fatalf("Failed to unmarshal expected XML: %v", err)
	}
	if err := xml.Unmarshal(content, &actual); err != nil {
		t.Fatalf("Failed to unmarshal actual XML: %v", err)
	}

	if len(actual.Recents) != 1 {
		t.Fatalf("Expected 1 recent, got %d", len(actual.Recents))
	}

	r := actual.Recents[0]
	if r.ID != "2567119953" || r.DeviceID != "001122334455" || r.UtcTime != "1771666755" {
		t.Errorf("Attributes mismatch: %+v", r)
	}
	if r.ContentItem.ItemName != "The National" || r.ContentItem.Source != "SPOTIFY" {
		t.Errorf("ContentItem mismatch: %+v", r.ContentItem)
	}
	if r.ContentItem.IsPresetable != "true" {
		t.Errorf("IsPresetable mismatch: got %s, expected true", r.ContentItem.IsPresetable)
	}

	// Now test Round-trip (GetRecents)
	loadedRecents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("GetRecents failed: %v", err)
	}

	if len(loadedRecents) != 1 {
		t.Fatalf("Expected 1 loaded recent, got %d", len(loadedRecents))
	}

	lr := loadedRecents[0]
	if lr.ID != "2567119953" || lr.Name != "The National" || lr.Source != "SPOTIFY" || lr.SourceAccount != "test-user" {
		t.Errorf("Loaded recent mismatch: %+v", lr)
	}
}
