package datastore

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestSavePresets_Format(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-format-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:          "test-playlist",
				Source:        "SPOTIFY",
				Type:          "tracklisturl",
				Location:      "/playback/container/c3BvdGlmeTpwbGF5bGlzdDo1Mm5QaVJrbWVmSkZPeHh1M1ZTd1hh",
				SourceAccount: "test-user",
				IsPresetable:  "true",
			},
			ID:           "1",
			ButtonNumber: "1",
			ContainerArt: "https://i.scdn.co/image/ab67616d00001e025ff75c5d082fc50a3a74ad7b",
			CreatedOn:    "1719128436",
			UpdatedOn:    "1728740382",
		},
	}

	err = ds.SavePresets(account, device, presets)
	if err != nil {
		t.Fatalf("SavePresets failed: %v", err)
	}

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read Presets.xml: %v", err)
	}

	xmlContent := string(data)

	// Check for correct id attribute
	if !strings.Contains(xmlContent, `id="1"`) {
		t.Errorf("Presets.xml missing correct id attribute, got: %s", xmlContent)
	}

	// Check that contentItemType is NOT present (as requested in previous issues)
	if strings.Contains(xmlContent, "contentItemType") {
		t.Errorf("Presets.xml should not contain contentItemType tag, got: %s", xmlContent)
	}

	// Verify unmarshaling still works
	loadedPresets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets failed: %v", err)
	}

	if len(loadedPresets) != 1 {
		t.Fatalf("Expected 1 preset, got %d", len(loadedPresets))
	}

	if loadedPresets[0].ID != "1" {
		t.Errorf("Expected ID 1, got %s", loadedPresets[0].ID)
	}

	if loadedPresets[0].ContentItemType != "tracklisturl" {
		t.Errorf("Expected ContentItemType to be tracklisturl, got %s", loadedPresets[0].ContentItemType)
	}
}

func TestSavePresets_PreservesID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-id-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "test-acc"
	device := "test-dev"

	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name: "Preset 1",
			},
			ID:           "1",
			ButtonNumber: "1",
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Name: "Preset 2",
			},
			ID: "2",
			// ButtonNumber is empty, should fall back to ID
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Name: "Preset 3",
			},
			ButtonNumber: "3",
			// ID is empty, should use ButtonNumber
		},
	}

	err = ds.SavePresets(account, device, presets)
	if err != nil {
		t.Fatalf("SavePresets failed: %v", err)
	}

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read Presets.xml: %v", err)
	}

	xmlContent := string(data)

	if !strings.Contains(xmlContent, `id="1"`) {
		t.Errorf("Expected id=\"1\", got: %s", xmlContent)
	}
	if !strings.Contains(xmlContent, `id="2"`) {
		t.Errorf("Expected id=\"2\", got: %s", xmlContent)
	}
	if !strings.Contains(xmlContent, `id="3"`) {
		t.Errorf("Expected id=\"3\", got: %s", xmlContent)
	}

	// Now check if GetPresets loads them correctly
	loaded, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets failed: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("Expected 3 presets, got %d", len(loaded))
	}

	for i, p := range loaded {
		expectedID := strconv.Itoa(i + 1)
		if p.ID != expectedID {
			t.Errorf("At index %d, expected ID %s, got %s", i, expectedID, p.ID)
		}
		if p.ButtonNumber != expectedID {
			t.Errorf("At index %d, expected ButtonNumber %s, got %s", i, expectedID, p.ButtonNumber)
		}
	}
}

func TestPresetsXML_NoID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-noid-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)

	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name: "No ID Preset",
			},
		},
	}

	err = ds.SavePresets("acc", "dev", presets)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(ds.AccountDeviceDir("acc", "dev"), "Presets.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `id=""`) {
		t.Errorf("Expected empty id attribute, got: %s", string(data))
	}
}
