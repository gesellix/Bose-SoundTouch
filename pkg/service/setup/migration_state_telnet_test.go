package setup

import (
	"errors"
	"testing"
)

func TestIsTelnetMigrated_TargetHostnamePresent(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{
		TelnetVerifiedConfig: "margeServerUrl=http://example:8000\nbmxRegistryUrl=http://example:8000/bmx/registry/v1/services\n",
	}

	if !m.isTelnetMigrated(summary) {
		t.Error("isTelnetMigrated = false, want true when getpdo response contains our hostname")
	}
}

func TestIsTelnetMigrated_DifferentHostname(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{
		TelnetVerifiedConfig: "margeServerUrl=https://streaming.bose.com\n",
	}

	if m.isTelnetMigrated(summary) {
		t.Error("isTelnetMigrated = true, want false when getpdo response points at the original cloud")
	}
}

func TestIsTelnetMigrated_EmptyVerifiedConfig(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{} // TelnetVerifiedConfig empty

	if m.isTelnetMigrated(summary) {
		t.Error("isTelnetMigrated = true, want false when TelnetVerifiedConfig is empty")
	}
}

// TestCheckIsMigrated_TelnetOnlyMigratedDevice covers the gap that motivated
// this iteration: SSH is unreachable, but the speaker has been pointed at
// our service via telnet (e.g. a firmware that refuses USB unlock). The
// migration UI must still report IsMigrated: true.
func TestCheckIsMigrated_TelnetOnlyMigratedDevice(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{
		SSHSuccess:           false,
		TelnetVerifiedConfig: "margeServerUrl=http://example:8000\n",
	}

	m.checkIsMigrated(summary, "192.0.2.1")

	if !summary.IsMigrated {
		t.Error("IsMigrated = false, want true on a telnet-only migrated device with no SSH")
	}
}

// TestCheckIsMigrated_NoTelnetNoSSH ensures we don't false-positive when
// neither transport sees the redirect.
func TestCheckIsMigrated_NoTelnetNoSSH(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{
		SSHSuccess:           false,
		TelnetVerifiedConfig: "", // probe failed
	}

	m.checkIsMigrated(summary, "192.0.2.1")

	if summary.IsMigrated {
		t.Error("IsMigrated = true, want false when neither SSH nor telnet sees the redirect")
	}
}

// TestCheckIsMigrated_TelnetSeesOriginalSSHSeesOriginal ensures we don't
// false-positive when both transports report unmigrated state.
func TestCheckIsMigrated_TelnetSeesOriginalSSHSeesOriginal(t *testing.T) {
	m := &Manager{
		ServerURL: "http://example:8000",
		NewSSH: func(string) SSHClient {
			return &mockSSH{runFunc: func(string) (string, error) { return "", errors.New("file not found") }}
		},
	}

	summary := &MigrationSummary{
		SSHSuccess:           true,
		TelnetVerifiedConfig: "margeServerUrl=https://streaming.bose.com\n",
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "https://streaming.bose.com",
		},
		CurrentResolvConf: "nameserver 8.8.8.8\n",
	}

	m.checkIsMigrated(summary, "192.0.2.1")

	if summary.IsMigrated {
		t.Error("IsMigrated = true, want false when both SSH and telnet see the original cloud URLs")
	}
}
