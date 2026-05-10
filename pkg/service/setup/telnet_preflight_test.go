package setup

import (
	"errors"
	"strings"
	"testing"
)

func TestTelnetPreflight_HappyPath(t *testing.T) {
	target := "http://example:8000"
	f := &fakeTelnet{
		banner: "BoseShell\n-> ",
		responses: map[string]string{
			"getpdo CurrentSystemConfiguration": "margeServerUrl=" + target + "\nbmxRegistryUrl=" + target + "/bmx/registry/v1/services\n",
		},
	}
	m := newFakeTelnetManager(f)

	summary := &MigrationSummary{}
	m.telnetPreflight(summary, "192.0.2.1")

	if !summary.TelnetReachable {
		t.Errorf("TelnetReachable = false, want true")
	}

	if !strings.Contains(summary.TelnetBanner, "BoseShell") {
		t.Errorf("TelnetBanner = %q, want it to contain BoseShell", summary.TelnetBanner)
	}

	if !strings.Contains(summary.TelnetVerifiedConfig, target) {
		t.Errorf("TelnetVerifiedConfig = %q, want it to contain %q", summary.TelnetVerifiedConfig, target)
	}

	if summary.TelnetProbeError != "" {
		t.Errorf("TelnetProbeError = %q, want empty on happy path", summary.TelnetProbeError)
	}
}

func TestTelnetPreflight_DialFailureRecorded(t *testing.T) {
	f := &fakeTelnet{dialErr: errors.New("connection refused")}
	m := newFakeTelnetManager(f)

	summary := &MigrationSummary{}
	m.telnetPreflight(summary, "192.0.2.1")

	if summary.TelnetReachable {
		t.Errorf("TelnetReachable = true, want false on dial failure")
	}

	if !strings.Contains(summary.TelnetProbeError, "connection refused") {
		t.Errorf("TelnetProbeError = %q, want it to wrap connection refused", summary.TelnetProbeError)
	}

	if len(f.commands) != 0 {
		t.Errorf("commands sent on dial failure: %v, want none", f.commands)
	}
}

func TestTelnetPreflight_GetpdoCommandNotFoundRecorded(t *testing.T) {
	f := &fakeTelnet{
		responses: map[string]string{
			// Default fakeTelnet behaviour returns "Command not found\n" for
			// any command not in the map. We rely on that here.
		},
	}
	m := newFakeTelnetManager(f)

	summary := &MigrationSummary{}
	m.telnetPreflight(summary, "192.0.2.1")

	if !summary.TelnetReachable {
		t.Errorf("TelnetReachable = false, want true (TCP dial succeeded)")
	}

	if summary.TelnetVerifiedConfig != "" {
		t.Errorf("TelnetVerifiedConfig = %q, want empty when getpdo is rejected", summary.TelnetVerifiedConfig)
	}

	if !strings.Contains(summary.TelnetProbeError, "getpdo") {
		t.Errorf("TelnetProbeError = %q, want it to mention the rejected command", summary.TelnetProbeError)
	}
}

func TestTelnetPreflight_TransportErrorRecorded(t *testing.T) {
	f := &fakeTelnet{
		fail: map[string]error{
			"getpdo CurrentSystemConfiguration": errors.New("read: broken pipe"),
		},
	}
	m := newFakeTelnetManager(f)

	summary := &MigrationSummary{}
	m.telnetPreflight(summary, "192.0.2.1")

	if !summary.TelnetReachable {
		t.Errorf("TelnetReachable = false, want true (dial succeeded before send)")
	}

	if !strings.Contains(summary.TelnetProbeError, "broken pipe") {
		t.Errorf("TelnetProbeError = %q, want it to wrap broken pipe", summary.TelnetProbeError)
	}
}

func TestTelnetPreflight_NoNewTelnetRecordsConfigurationError(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"} // NewTelnet deliberately nil

	summary := &MigrationSummary{}
	m.telnetPreflight(summary, "192.0.2.1")

	if summary.TelnetReachable {
		t.Errorf("TelnetReachable = true, want false when NewTelnet is nil")
	}

	if !strings.Contains(summary.TelnetProbeError, "not configured") {
		t.Errorf("TelnetProbeError = %q, want it to mention configuration", summary.TelnetProbeError)
	}
}
