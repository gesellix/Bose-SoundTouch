package health

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newDNSBypassDatastore(t *testing.T, devices ...[]string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "dns-bypass-test-*")
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

// TestDNSBypassRisk_DNSOff verifies that with DNS Discovery off there is
// nothing to warn about, regardless of any device's actual DNS path:
// HandleBMXRegistry hands out this service's own address in that case.
func TestDNSBypassRisk_DNSOff(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return false }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }
	// If this were consulted while DNS is off, it would report a bypass —
	// proving the short-circuit, not just an empty result by chance.
	resolutionFn := func(_, _ string) (bool, bool, bool) { return false, false, true }

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, resolutionFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when DNS Discovery is off, got %+v", got)
	}
}

// TestDNSBypassRisk_SSHConfirmsUsingUs verifies that when SSH positively
// confirms the speaker resolves through AfterTouch (matching nameserver, or
// the migration hook installed), no finding is emitted even though DNS is
// enabled.
func TestDNSBypassRisk_SSHConfirmsUsingUs(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }
	resolutionFn := func(ip, expectedIP string) (usesAfterTouch, hookInstalled, sshOK bool) {
		return true, false, true // resolv.conf lists expectedIP as nameserver
	}

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, resolutionFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when SSH confirms the speaker uses AfterTouch's DNS, got %+v", got)
	}
}

// TestDNSBypassRisk_SSHConfirmsHookInstalledOnly verifies the hook-file
// signal alone (no matching nameserver yet, e.g. no DHCP renewal since
// migrating) is also treated as confirmed, not as a bypass.
func TestDNSBypassRisk_SSHConfirmsHookInstalledOnly(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }
	resolutionFn := func(_, _ string) (usesAfterTouch, hookInstalled, sshOK bool) {
		return false, true, true
	}

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, resolutionFn)
	if len(got) != 0 {
		t.Errorf("expected no findings when the migration hook is installed, got %+v", got)
	}
}

// TestDNSBypassRisk_SSHEvidenceOfBypass verifies the strong-signal warning:
// SSH reachable, no AfterTouch nameserver, no migration hook.
func TestDNSBypassRisk_SSHEvidenceOfBypass(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }
	resolutionFn := func(_, _ string) (usesAfterTouch, hookInstalled, sshOK bool) {
		return false, false, true
	}

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, resolutionFn)
	if len(got) != 1 {
		t.Fatalf("expected exactly one finding for confirmed bypass, got %+v", got)
	}

	f := got[0]
	if f.Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %q", f.Severity)
	}

	if !strings.Contains(f.Message, "192.0.2.10") {
		t.Errorf("expected device IP in message, got %q", f.Message)
	}

	if !strings.Contains(f.Details, dnsBypassTroubleshootingURL) {
		t.Errorf("expected troubleshooting link in details, got %q", f.Details)
	}

	if len(f.ManualCommands) != 1 || !strings.Contains(f.ManualCommands[0].Command, "setup migrate") {
		t.Errorf("expected a migrate manual command, got %+v", f.ManualCommands)
	}

	if f.Target.Device != "DEVICE01" {
		t.Errorf("expected Target.Device = DEVICE01, got %q", f.Target.Device)
	}
}

// TestDNSBypassRisk_NoSSH_QueryObserved verifies that without an SSH signal,
// a device observed querying an intercepted hostname is treated as
// confirmed (no finding), same as dns_speaker_usage's own logic.
func TestDNSBypassRisk_NoSSH_QueryObserved(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time {
		return map[string]time.Time{"192.0.2.10": time.Now()}
	}

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, nil)
	if len(got) != 0 {
		t.Errorf("expected no findings when the device has queried through us, got %+v", got)
	}
}

// TestDNSBypassRisk_NoSSH_NoEvidenceEitherWay verifies the weak/ambiguous
// case: no SSH probe function at all, and the device hasn't been observed
// querying an intercepted hostname either. This must stay info-level, never
// warning, since "never queried" isn't positive evidence of a bypass.
func TestDNSBypassRisk_NoSSH_NoEvidenceEitherWay(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, nil)
	if len(got) != 1 {
		t.Fatalf("expected exactly one info finding, got %+v", got)
	}

	f := got[0]
	if f.Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo, got %q", f.Severity)
	}

	if len(f.QuickFixes) == 0 || f.QuickFixes[0].ID != "probe_dns_path" {
		t.Errorf("expected probe_dns_path QuickFix, got %+v", f.QuickFixes)
	}
}

// TestDNSBypassRisk_SSHUnreachableFallsBackToQuerySignal verifies that when
// a resolutionFn is wired but SSH itself fails (sshOK=false) for this
// device, the check falls back to the query-based signal rather than
// treating SSH-unreachable as either confirmation or bypass evidence.
func TestDNSBypassRisk_SSHUnreachableFallsBackToQuerySignal(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", "192.0.2.10"})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time {
		return map[string]time.Time{"192.0.2.10": time.Now()}
	}
	resolutionFn := func(_, _ string) (usesAfterTouch, hookInstalled, sshOK bool) {
		return false, false, false // SSH unreachable
	}

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, resolutionFn)
	if len(got) != 0 {
		t.Errorf("expected fallback to the query signal (confirmed) when SSH is unreachable, got %+v", got)
	}
}

// TestDNSBypassRisk_DevicesWithNoIPSkipped verifies devices without a
// recorded IP address are skipped rather than probed or reported.
func TestDNSBypassRisk_DevicesWithNoIPSkipped(t *testing.T) {
	ds := newDNSBypassDatastore(t, []string{"1000001", "DEVICE01", ""})

	dnsEnabledFn := func() bool { return true }
	expectedIPFn := func() string { return "192.0.2.1" }
	clientIPsFn := func() map[string]time.Time { return map[string]time.Time{} }

	got := runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, nil)
	if len(got) != 0 {
		t.Errorf("expected no findings for a device with no IP, got %+v", got)
	}
}

// TestDNSBypassRisk_NilDatastore verifies the nil-datastore guard.
func TestDNSBypassRisk_NilDatastore(t *testing.T) {
	dnsEnabledFn := func() bool { return true }

	got := runDNSBypassRiskCheck(nil, dnsEnabledFn, nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected no findings for a nil datastore, got %+v", got)
	}
}
