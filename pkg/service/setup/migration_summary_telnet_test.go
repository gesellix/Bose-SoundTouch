package setup

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// telnetSummaryEnv builds a Manager whose:
//   - SSH client is the supplied mockSSH (or a no-op if nil).
//   - Telnet client is the supplied fakeTelnet.
//   - Live :8090/info call hits an httptest server returning a minimal XML.
//
// The deviceIP returned is the httptest server's listener addr ("host:port"),
// so the live-info call works; the SSH and telnet clients ignore the addr
// and return whatever the fakes are scripted to return.
func telnetSummaryEnv(t *testing.T, ssh *mockSSH, ft *fakeTelnet) (*Manager, string, func()) {
	t.Helper()
	return telnetSummaryEnvWithInfo(t, ssh, ft, `<info deviceID="123"><name>Test</name></info>`)
}

// telnetSummaryEnvWithInfo is telnetSummaryEnv with a caller-supplied
// :8090/info XML body, so individual tests can exercise device-info
// fields that affect summary state (e.g. margeAccountUUID for IsPaired).
func telnetSummaryEnvWithInfo(t *testing.T, ssh *mockSSH, ft *fakeTelnet, infoXML string) (*Manager, string, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, infoXML)
	}))

	m := NewManager("http://example:8000", nil, nil)
	m.NewSSH = func(string) SSHClient {
		if ssh != nil {
			return ssh
		}
		return &mockSSH{runFunc: func(string) (string, error) { return "", errors.New("ssh disabled in test") }}
	}
	m.NewTelnet = func(string) TelnetClient { return ft }

	return m, server.Listener.Addr().String(), server.Close
}

func TestGetMigrationSummary_TelnetSucceedsSSHFails(t *testing.T) {
	target := "http://example:8000"
	ft := &fakeTelnet{
		banner: "BoseShell\n-> ",
		responses: map[string]string{
			"getpdo CurrentSystemConfiguration": "margeServerUrl=" + target + "\n",
		},
	}

	m, host, cleanup := telnetSummaryEnv(t, nil, ft)
	defer cleanup()

	summary, err := m.GetMigrationSummary(host, "", "", nil)
	if err != nil {
		t.Fatalf("GetMigrationSummary: %v", err)
	}

	if summary.SSHSuccess {
		t.Errorf("SSHSuccess = true, want false")
	}

	if !summary.TelnetReachable {
		t.Errorf("TelnetReachable = false, want true")
	}

	if !strings.Contains(summary.TelnetBanner, "BoseShell") {
		t.Errorf("TelnetBanner = %q, want it to contain BoseShell", summary.TelnetBanner)
	}

	if !strings.Contains(summary.TelnetVerifiedConfig, target) {
		t.Errorf("TelnetVerifiedConfig = %q, want it to contain %q", summary.TelnetVerifiedConfig, target)
	}
}

func TestGetMigrationSummary_TelnetFailsSSHFails(t *testing.T) {
	ft := &fakeTelnet{dialErr: errors.New("connection refused")}

	m, host, cleanup := telnetSummaryEnv(t, nil, ft)
	defer cleanup()

	summary, err := m.GetMigrationSummary(host, "", "", nil)
	if err != nil {
		t.Fatalf("GetMigrationSummary: %v", err)
	}

	if summary.SSHSuccess {
		t.Errorf("SSHSuccess = true, want false")
	}

	if summary.TelnetReachable {
		t.Errorf("TelnetReachable = true, want false")
	}

	if !strings.Contains(summary.TelnetProbeError, "connection refused") {
		t.Errorf("TelnetProbeError = %q, want connection refused", summary.TelnetProbeError)
	}
}

func TestGetMigrationSummary_IsPairedFromLiveInfo(t *testing.T) {
	ft := &fakeTelnet{dialErr: errors.New("not the focus of this test")}

	t.Run("with margeAccountUUID", func(t *testing.T) {
		m, host, cleanup := telnetSummaryEnvWithInfo(t, nil, ft,
			`<info deviceID="123"><name>Test</name><margeAccountUUID>3230304</margeAccountUUID></info>`,
		)
		defer cleanup()

		summary, err := m.GetMigrationSummary(host, "", "", nil)
		if err != nil {
			t.Fatalf("GetMigrationSummary: %v", err)
		}

		if !summary.IsPaired {
			t.Errorf("IsPaired = false, want true (margeAccountUUID present in :8090/info)")
		}

		if summary.AccountID != "3230304" {
			t.Errorf("AccountID = %q, want 3230304 (live info should populate)", summary.AccountID)
		}
	})

	t.Run("without margeAccountUUID", func(t *testing.T) {
		m, host, cleanup := telnetSummaryEnvWithInfo(t, nil, ft,
			`<info deviceID="123"><name>Test</name><margeAccountUUID></margeAccountUUID></info>`,
		)
		defer cleanup()

		summary, err := m.GetMigrationSummary(host, "", "", nil)
		if err != nil {
			t.Fatalf("GetMigrationSummary: %v", err)
		}

		if summary.IsPaired {
			t.Errorf("IsPaired = true, want false (factory-reset device with empty margeAccountUUID)")
		}
	})
}

func TestGetMigrationSummary_TelnetSucceedsSSHSucceeds(t *testing.T) {
	target := "http://example:8000"
	ft := &fakeTelnet{
		responses: map[string]string{
			"getpdo CurrentSystemConfiguration": "margeServerUrl=" + target + "\n",
		},
	}

	// SSH mock returns enough for SSHSuccess to be true (cat /opt/Bose/etc/...).
	ssh := &mockSSH{
		runFunc: func(cmd string) (string, error) {
			switch {
			case strings.HasPrefix(cmd, "cat "+SoundTouchSdkPrivateCfgPath):
				return `<?xml version="1.0"?><SoundTouchSdkPrivateCfg><margeServerUrl>` + target + `</margeServerUrl></SoundTouchSdkPrivateCfg>`, nil
			case strings.HasPrefix(cmd, "[ -f"):
				return "", errors.New("not found")
			default:
				return "", nil
			}
		},
	}

	m, host, cleanup := telnetSummaryEnv(t, ssh, ft)
	defer cleanup()

	summary, err := m.GetMigrationSummary(host, "", "", nil)
	if err != nil {
		t.Fatalf("GetMigrationSummary: %v", err)
	}

	if !summary.SSHSuccess {
		t.Errorf("SSHSuccess = false, want true")
	}

	if !summary.TelnetReachable {
		t.Errorf("TelnetReachable = false, want true")
	}

	if !strings.Contains(summary.TelnetVerifiedConfig, target) {
		t.Errorf("TelnetVerifiedConfig = %q, want %q", summary.TelnetVerifiedConfig, target)
	}
}
