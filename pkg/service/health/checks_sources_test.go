package health

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newTestDatastoreWithDevice(t *testing.T, account, device string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "health-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		Name:      "TestSpeaker",
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func TestSourcesXMLPresent_FlagsMissingFile(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newTestDatastoreWithDevice(t, account, device)

	if ds.HasConfiguredSources(account, device) {
		t.Fatalf("precondition: device should not have Sources.xml yet")
	}

	r := NewRegistry()
	RegisterSourcesXMLPresent(r, ds)

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 check result, got %d", len(results))
	}

	check := results[0]
	if check.ID != CheckIDSourcesXMLPresent {
		t.Errorf("unexpected check id %q", check.ID)
	}

	if check.Severity != SeverityWarning {
		t.Errorf("expected warning severity, got %q", check.Severity)
	}

	if len(check.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(check.Findings))
	}

	finding := check.Findings[0]
	if finding.Target.Account != account || finding.Target.Device != device {
		t.Errorf("finding target %+v doesn't match device", finding.Target)
	}

	if len(finding.QuickFixes) != 1 || finding.QuickFixes[0].ID != FixIDCreateDefaultSources {
		t.Errorf("expected create_default_sources quick fix, got %+v", finding.QuickFixes)
	}
}

func TestSourcesXMLPresent_QuickFix_MaterialisesDefaults(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newTestDatastoreWithDevice(t, account, device)

	r := NewRegistry()
	RegisterSourcesXMLPresent(r, ds)

	msg, err := r.RunFix(CheckIDSourcesXMLPresent, FixIDCreateDefaultSources, Target{
		Account: account,
		Device:  device,
	})
	if err != nil {
		t.Fatalf("RunFix: %v", err)
	}

	if msg == "" {
		t.Errorf("expected non-empty success message")
	}

	if !ds.HasConfiguredSources(account, device) {
		t.Fatalf("Sources.xml was not materialised by the fix")
	}

	// The defaults should include TUNEIN — that's the load-bearing
	// reason for this check.
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources: %v", err)
	}

	var sawTuneIn bool
	for i := range sources {
		if sources[i].SourceKeyType == "TUNEIN" {
			sawTuneIn = true
			break
		}
	}

	if !sawTuneIn {
		t.Errorf("expected TUNEIN among default sources, got %d entries without it", len(sources))
	}

	// Re-running the check should now report a clean state.
	results := r.RunAll()
	if results[0].Severity != SeverityOK {
		t.Errorf("expected OK after fix, got %q", results[0].Severity)
	}

	if len(results[0].Findings) != 0 {
		t.Errorf("expected no findings after fix, got %d", len(results[0].Findings))
	}
}

func TestSourcesXMLPresent_NilDatastore(t *testing.T) {
	r := NewRegistry()
	RegisterSourcesXMLPresent(r, nil)

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Severity != SeverityOK {
		t.Errorf("nil datastore should produce no findings, got %q", results[0].Severity)
	}
}

func TestSourcesXMLPresent_FixRejectsEmptyTarget(t *testing.T) {
	ds := newTestDatastoreWithDevice(t, "1000001", "DEVICEID01")

	r := NewRegistry()
	RegisterSourcesXMLPresent(r, ds)

	if _, err := r.RunFix(CheckIDSourcesXMLPresent, FixIDCreateDefaultSources, Target{}); err == nil {
		t.Errorf("expected error for empty target, got nil")
	}
}
