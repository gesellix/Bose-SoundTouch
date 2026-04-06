package datastore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestSaveSources_Format(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-sources-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	sources := []models.ConfiguredSource{
		{
			DisplayName: "AUX IN",
			SourceKey: struct {
				Type    string `xml:"type,attr"`
				Account string `xml:"account,attr"`
			}{Type: "AUX", Account: "AUX"},
		},
		{
			SourceKey: struct {
				Type    string `xml:"type,attr"`
				Account string `xml:"account,attr"`
			}{Type: "INTERNET_RADIO", Account: ""},
		},
		{
			DisplayName: "user@example.com",
			Secret:      "dummy-token-spotify",
			SecretType:  "token_version_3",
			SourceKey: struct {
				Type    string `xml:"type,attr"`
				Account string `xml:"account,attr"`
			}{Type: "SPOTIFY", Account: "test-user"},
		},
	}

	err = ds.SaveConfiguredSources(account, device, sources)
	if err != nil {
		t.Fatalf("SaveConfiguredSources failed: %v", err)
	}

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Sources.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read Sources.xml: %v", err)
	}

	xmlContent := string(data)

	// Check for correct attributes in first source
	if !strings.Contains(xmlContent, `<source displayName="AUX IN" secret="" secretType="">`) {
		t.Errorf("First source missing expected attributes. Got: %s", xmlContent)
	}
	if !strings.Contains(xmlContent, `<sourceKey type="AUX" account="AUX" />`) &&
		!strings.Contains(xmlContent, `<sourceKey type="AUX" account="AUX"></sourceKey>`) {
		t.Errorf("First sourceKey incorrect. Got: %s", xmlContent)
	}

	// Check for credential element (new format)
	if !strings.Contains(xmlContent, `<credential type="token_version_3">dummy-token-spotify</credential>`) {
		t.Errorf("Spotify source missing <credential> element. Got: %s", xmlContent)
	}

	// Check for third source (Spotify)
	if !strings.Contains(xmlContent, `displayName="user@example.com"`) {
		t.Errorf("Spotify source missing displayName. Got: %s", xmlContent)
	}
	if !strings.Contains(xmlContent, `secret="dummy-token-spotify" secretType="token_version_3">`) {
		t.Errorf("Spotify source missing secret. Got: %s", xmlContent)
	}
	if !strings.Contains(xmlContent, `<sourceKey type="SPOTIFY" account="test-user" />`) &&
		!strings.Contains(xmlContent, `<sourceKey type="SPOTIFY" account="test-user"></sourceKey>`) {
		t.Errorf("Spotify sourceKey incorrect. Got: %s", xmlContent)
	}

	// Negative checks for extra tags
	if strings.Contains(xmlContent, "<sourcename>") {
		t.Errorf("Sources.xml should not contain <sourcename> tag")
	}
	if strings.Contains(xmlContent, "<username>") {
		t.Errorf("Sources.xml should not contain <username> tag")
	}
	if strings.Contains(xmlContent, "<name>") {
		t.Errorf("Sources.xml should not contain <name> tag")
	}
	if strings.Contains(xmlContent, "<sourceSettings>") {
		t.Errorf("Sources.xml should not contain <sourceSettings> tag")
	}
}
