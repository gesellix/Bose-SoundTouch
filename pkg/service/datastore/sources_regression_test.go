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

// TestGetConfiguredSources_MinimalAuxEntryNormalized covers the migration case from
// issue #195: the device's on-disk Sources.xml carries only displayName + sourceKey
// for AUX (no id, no type). When read back, the AUX entry must surface as the
// canonical id="10001" type="Audio" sourceproviderid="9", not synthesized values.
func TestGetConfiguredSources_MinimalAuxEntryNormalized(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-sources-min-aux-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	deviceDir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatal(err)
	}

	minimalSourcesXML := `<sources>
    <source displayName="AUX IN" secret="">
        <sourceKey type="AUX" account="AUX" />
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(minimalSourcesXML), 0644); err != nil {
		t.Fatal(err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	s := sources[0]
	if s.ID != "10001" {
		t.Errorf("expected canonical AUX id 10001, got %q", s.ID)
	}
	if s.Type != "Audio" {
		t.Errorf("expected canonical AUX type 'Audio', got %q", s.Type)
	}
	if s.SourceKey.Type != "AUX" || s.SourceKey.Account != "AUX" {
		t.Errorf("expected sourceKey type/account AUX/AUX, got %q/%q", s.SourceKey.Type, s.SourceKey.Account)
	}
}

// TestGetConfiguredSources_DuplicateProviderUniqueIDs ensures that when a file
// contains multiple entries for the same SourceKey.Type (e.g. two AUX entries),
// only one gets the canonical ID; the rest fall back to synthesized IDs so they
// don't collide.
func TestGetConfiguredSources_DuplicateProviderUniqueIDs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-sources-dup-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	deviceDir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatal(err)
	}

	dupXML := `<sources>
    <source displayName="AUX IN" secret="">
        <sourceKey type="AUX" account="AUX" />
    </source>
    <source displayName="AUX 2" secret="">
        <sourceKey type="AUX" account="AUX" />
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(dupXML), 0644); err != nil {
		t.Fatal(err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}

	if sources[0].ID == sources[1].ID {
		t.Errorf("duplicate AUX entries must not share an ID, got %q for both", sources[0].ID)
	}

	// Both should still have Type repaired to the canonical "Audio".
	for i, s := range sources {
		if s.Type != "Audio" {
			t.Errorf("source %d: expected Type 'Audio', got %q", i, s.Type)
		}
	}
}

// TestGetConfiguredSources_PoisonedAuxEntryRepaired covers the case where a previous
// version of the datastore already persisted bad synthesized values (type="AUX",
// id="2000001"). On read, those values must be repaired to the canonical defaults.
func TestGetConfiguredSources_PoisonedAuxEntryRepaired(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-sources-poisoned-aux-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	deviceDir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatal(err)
	}

	poisonedXML := `<sources>
    <source displayName="AUX IN" id="2000001" secret="" secretType="" type="AUX">
        <credential type=""></credential>
        <sourceKey type="AUX" account="AUX"></sourceKey>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(poisonedXML), 0644); err != nil {
		t.Fatal(err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	s := sources[0]
	if s.Type != "Audio" {
		t.Errorf("expected Type to be repaired to 'Audio', got %q", s.Type)
	}
	// ID repair is intentionally not aggressive — only empty IDs are filled
	// from canonical defaults to avoid breaking references in recents/presets.
	if s.ID != "2000001" {
		t.Errorf("expected ID preserved as 2000001, got %q", s.ID)
	}
}
