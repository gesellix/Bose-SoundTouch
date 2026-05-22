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

// TestIsXMLMigrated_MalformedServiceURL guards against the false-positive that
// occurs when --service-url is a malformed URL with an empty hostname (e.g.
// "https:/host" instead of "https://host"). url.Parse succeeds but Hostname()
// returns "", and strings.Contains(anything, "") is always true in Go.
func TestIsXMLMigrated_MalformedServiceURL(t *testing.T) {
	m := &Manager{ServerURL: "https:/soundtouch.fritz.box"} // single slash — malformed

	summary := &MigrationSummary{
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "https://streaming.bose.com",
		},
	}

	if m.isXMLMigrated(summary) {
		t.Error("isXMLMigrated = true, want false when ServerURL has no resolvable hostname")
	}
}

// TestCheckIsMigrated_MalformedServiceURL ensures a malformed --service-url
// does not produce IsMigrated=true on an unmigrated speaker.
func TestCheckIsMigrated_MalformedServiceURL(t *testing.T) {
	m := &Manager{
		ServerURL: "https:/soundtouch.fritz.box", // single slash — malformed
		NewSSH: func(string) SSHClient {
			return &mockSSH{runFunc: func(string) (string, error) { return "", errors.New("unused") }}
		},
	}

	summary := &MigrationSummary{
		SSHSuccess:           true,
		TelnetVerifiedConfig: "", // empty — not migrated via telnet either
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "https://streaming.bose.com",
		},
		CurrentResolvConf: "nameserver 8.8.8.8\n",
	}

	m.checkIsMigrated(summary, "192.0.2.1")

	if summary.IsMigrated {
		t.Error("IsMigrated = true, want false when service URL is malformed and speaker still points at streaming.bose.com")
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

// TestCheckIsMigrated_PerAxisBooleansArePopulated locks in that each
// axis is reported individually so the UI can show partial-state cells.
// The mock SSH client claims /etc/hosts has Bose redirects; XML is
// unmigrated; resolv has no marker; telnet sees the redirected URL.
// All four axis flags must reflect their independent verdicts and
// IsMigrated must be the OR.
func TestCheckIsMigrated_PerAxisBooleansArePopulated(t *testing.T) {
	m := &Manager{
		ServerURL: "http://example:8000",
		NewSSH: func(string) SSHClient {
			return &mockSSH{
				runFunc: func(cmd string) (string, error) {
					if cmd == "cat /etc/hosts" {
						return "192.0.2.1\tstreaming.bose.com\n", nil
					}
					return "", errors.New("not implemented in this mock")
				},
			}
		},
	}

	summary := &MigrationSummary{
		SSHSuccess:    true,
		CACertTrusted: true, // hosts migration requires CA trust
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "https://streaming.bose.com",
		},
		TelnetVerifiedConfig: "margeServerUrl=http://example:8000\n",
		CurrentResolvConf:    "nameserver 8.8.8.8\n",
	}

	m.checkIsMigrated(summary, "192.0.2.1")

	if !summary.TelnetMigrated {
		t.Error("TelnetMigrated = false, want true (verified config points at example)")
	}

	if summary.XMLMigrated {
		t.Error("XMLMigrated = true, want false (parsed XML still points at streaming.bose.com)")
	}

	if !summary.HostsMigrated {
		t.Error("HostsMigrated = false, want true (mock hosts content has Bose redirect + CA trusted)")
	}

	if summary.ResolvMigrated {
		t.Error("ResolvMigrated = true, want false (no marker, no example hostname)")
	}

	if !summary.IsMigrated {
		t.Error("IsMigrated = false, want true (TelnetMigrated || HostsMigrated)")
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
