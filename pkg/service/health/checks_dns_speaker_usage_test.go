package health

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// newSpeakerUsageDatastore creates a temporary datastore and populates it
// with the given device records. Each entry is [accountID, deviceID, ipAddress].
func newSpeakerUsageDatastore(t *testing.T, devices ...[]string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "dns-speaker-usage-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	for _, d := range devices {
		if err := ds.SaveDeviceInfo(d[0], d[1], &models.ServiceDeviceInfo{
			DeviceID:  d[1],
			AccountID: d[0],
			Name:      "Speaker-" + d[1],
			IPAddress: d[2],
		}); err != nil {
			t.Fatalf("SaveDeviceInfo(%v): %v", d, err)
		}
	}
	return ds
}

func TestDNSSpeakerUsage_DNSNotRunning(t *testing.T) {
	ds := newSpeakerUsageDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})
	statusFn := func() (bool, string) { return false, "" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{"192.0.2.10": time.Now()} }

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when DNS not running, got %+v", got)
	}
}

// TestDNSSpeakerUsage_EmptyQuerierSet_WithDevices verifies that when DNS is
// running but no device has been observed yet (empty querier set), the check
// emits a SeverityInfo finding per device with the probe_dns_path QuickFix,
// and does NOT emit any SeverityWarning.
func TestDNSSpeakerUsage_EmptyQuerierSet_WithDevices(t *testing.T) {
	ds := newSpeakerUsageDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})
	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}

	// Must be info, not warning — the empty querier set no longer emits a warning.
	if got[0].Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo, got %s", got[0].Severity)
	}

	// Must mention the device IP and the action label.
	if !strings.Contains(got[0].Message, "192.0.2.10") {
		t.Errorf("expected device IP in message, got %q", got[0].Message)
	}

	if !strings.Contains(got[0].Message, "Test DNS path") {
		t.Errorf("expected 'Test DNS path' in message, got %q", got[0].Message)
	}

	// Must carry the probe_dns_path QuickFix.
	if len(got[0].QuickFixes) == 0 {
		t.Fatal("expected at least one QuickFix in the info finding")
	}

	if got[0].QuickFixes[0].ID != "probe_dns_path" {
		t.Errorf("expected QuickFix ID 'probe_dns_path', got %q", got[0].QuickFixes[0].ID)
	}

	if got[0].QuickFixes[0].Label != "Test DNS path" {
		t.Errorf("expected QuickFix Label 'Test DNS path', got %q", got[0].QuickFixes[0].Label)
	}

	// Must have no ManualCommands (those were on the old warning).
	if len(got[0].ManualCommands) != 0 {
		t.Errorf("expected no ManualCommands on info finding, got %+v", got[0].ManualCommands)
	}

	// Target must be set so the fix dispatcher knows which device to probe.
	if got[0].Target.Device != "DEVICE01" {
		t.Errorf("expected Target.Device = 'DEVICE01', got %q", got[0].Target.Device)
	}
}

func TestDNSSpeakerUsage_EmptyQuerierSet_NoDevices(t *testing.T) {
	// No devices registered: nothing to assert about, so no findings.
	ds := newSpeakerUsageDatastore(t) // no device entries
	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when no devices registered, got %+v", got)
	}
}

func TestDNSSpeakerUsage_DeviceObserved(t *testing.T) {
	ds := newSpeakerUsageDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})
	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }
	clientIPsFn := func() map[string]time.Time {
		return map[string]time.Time{"192.0.2.10": time.Now()}
	}

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when device IP is in querier set, got %+v", got)
	}
}

func TestDNSSpeakerUsage_Mixed(t *testing.T) {
	// DEVICE01 observed, DEVICE02 not observed.
	ds := newSpeakerUsageDatastore(t,
		[]string{"1000001", "DEVICE01", "192.0.2.10"},
		[]string{"1000001", "DEVICE02", "192.0.2.11"},
	)
	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }
	clientIPsFn := func() map[string]time.Time {
		return map[string]time.Time{"192.0.2.10": time.Now()}
	}

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	if len(got) != 1 {
		t.Fatalf("expected one finding for unobserved device, got %+v", got)
	}

	if got[0].Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo for unobserved device, got %s", got[0].Severity)
	}

	if !strings.Contains(got[0].Message, "192.0.2.11") {
		t.Errorf("expected unobserved IP in message, got %q", got[0].Message)
	}

	if strings.Contains(got[0].Message, "192.0.2.10") {
		t.Errorf("observed device IP should not appear in findings, but it did: %q", got[0].Message)
	}

	// Must carry probe_dns_path QuickFix.
	if len(got[0].QuickFixes) == 0 || got[0].QuickFixes[0].ID != "probe_dns_path" {
		t.Errorf("expected probe_dns_path QuickFix, got %+v", got[0].QuickFixes)
	}

	// Target must identify DEVICE02.
	if got[0].Target.Device != "DEVICE02" {
		t.Errorf("expected Target.Device = 'DEVICE02', got %q", got[0].Target.Device)
	}
}

func TestDNSSpeakerUsage_AllDevicesObserved(t *testing.T) {
	ds := newSpeakerUsageDatastore(t,
		[]string{"1000001", "DEVICE01", "192.0.2.10"},
		[]string{"1000001", "DEVICE02", "192.0.2.11"},
	)
	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }
	clientIPsFn := func() map[string]time.Time {
		return map[string]time.Time{
			"192.0.2.10": time.Now(),
			"192.0.2.11": time.Now(),
		}
	}

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when all devices observed, got %+v", got)
	}
}

func TestDNSSpeakerUsage_DevicesWithNoIP_Skipped(t *testing.T) {
	// A device with no IP address should not contribute to findings.
	tempDir, err := os.MkdirTemp("", "dns-speaker-usage-noip-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	if err := ds.SaveDeviceInfo("1000001", "DEVICE01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICE01",
		AccountID: "1000001",
		Name:      "NoIPSpeaker",
		IPAddress: "", // intentionally empty
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	// knownIPs is empty after skipping the no-IP device, so no findings expected.
	if len(got) != 0 {
		t.Errorf("expected no findings when all devices lack IP, got %+v", got)
	}
}

// TestDNSSpeakerUsage_NoWarningEverEmitted verifies the core invariant: the
// dns_speaker_usage check must never emit a SeverityWarning finding, regardless
// of the querier-set state. Warnings were the source of the false-positive on
// restart that this rework eliminates.
func TestDNSSpeakerUsage_NoWarningEverEmitted(t *testing.T) {
	ds := newSpeakerUsageDatastore(t,
		[]string{"1000001", "DEVICE01", "192.0.2.10"},
		[]string{"1000001", "DEVICE02", "192.0.2.11"},
	)
	statusFn := func() (bool, string) { return true, "0.0.0.0:53" }

	// Test with empty querier set (the classic false-positive scenario).
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }

	got := runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
	for _, f := range got {
		if f.Severity == SeverityWarning {
			t.Errorf("dns_speaker_usage must never emit SeverityWarning; got finding: %+v", f)
		}
	}
}
