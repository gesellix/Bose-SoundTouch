package health

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newOrionTestDS(t *testing.T, account, device string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "orion-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func writePresetsXML(t *testing.T, ds *datastore.DataStore, account, device, xml string) {
	t.Helper()

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml")
	if err := os.WriteFile(path, []byte(xml), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}
}

func TestOrionPaths_FlagsDeadCloudURLs(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
  <preset id="1" createdOn="2024-01-01" updatedOn="2024-01-01">
    <contentItem source="TUNEIN" type="stationurl" location="https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/v1/playback/station/s1234" sourceAccount="">
      <itemName>Dead preset</itemName>
    </contentItem>
  </preset>
  <preset id="2" createdOn="2026-05-01" updatedOn="2026-05-01">
    <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s5678" sourceAccount="">
      <itemName>Healthy preset</itemName>
    </contentItem>
  </preset>
</presets>`
	writePresetsXML(t, ds, account, device, xml)

	r := NewRegistry()
	RegisterOrionPathsCheck(r, ds)

	results := r.RunAll()
	if len(results) != 1 || results[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", results)
	}

	if len(results[0].Findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(results[0].Findings))
	}

	finding := results[0].Findings[0]
	if !strings.Contains(finding.Message, "1 preset") {
		t.Errorf("expected mention of 1 affected preset, got %q", finding.Message)
	}

	if len(finding.ManualCommands) != 1 {
		t.Fatalf("expected one manual command (sed snippet)")
	}

	if !strings.Contains(finding.ManualCommands[0].Command, "sed") || !strings.Contains(finding.ManualCommands[0].Command, account) {
		t.Errorf("manual command should reference sed + account dir, got %q", finding.ManualCommands[0].Command)
	}
}

func TestOrionPaths_NoFindingWhenClean(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
  <preset id="1" createdOn="2026-05-01" updatedOn="2026-05-01">
    <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s1234" sourceAccount="">
      <itemName>Healthy preset</itemName>
    </contentItem>
  </preset>
</presets>`
	writePresetsXML(t, ds, account, device, xml)

	r := NewRegistry()
	RegisterOrionPathsCheck(r, ds)

	results := r.RunAll()
	if results[0].Severity != SeverityOK {
		t.Errorf("expected OK, got %q", results[0].Severity)
	}

	if len(results[0].Findings) != 0 {
		t.Errorf("expected no findings, got %+v", results[0].Findings)
	}
}

func TestOrionPaths_NoFindingWhenNoPresetsFile(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)

	r := NewRegistry()
	RegisterOrionPathsCheck(r, ds)

	results := r.RunAll()
	if len(results[0].Findings) != 0 {
		t.Errorf("expected no findings for missing Presets.xml, got %+v", results[0].Findings)
	}
}

func TestFindOrionHits_LabelFallback(t *testing.T) {
	presets := []models.ServicePreset{
		{
			ID: "5",
			ServiceContentItem: models.ServiceContentItem{
				Location: "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/abc",
			},
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:     "Fallback Name",
				Location: "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/xyz",
			},
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Location: "/v1/playback/station/s1234",
			},
		},
	}

	got := findOrionHits(presets)
	if len(got) != 2 {
		t.Fatalf("expected 2 hits, got %v", got)
	}

	if got[0] != "5" || got[1] != "Fallback Name" {
		t.Errorf("unexpected labels: %v", got)
	}
}
