package setup

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

type mockSSH struct {
	runFunc           func(command string) (string, error)
	uploadContentFunc func(content []byte, remotePath string) error

	// uploaded mirrors UploadContent calls so that a subsequent
	// `cat <path>` against a path that the test didn't explicitly
	// script via runFunc returns what we just wrote there. This is
	// what makes the tmp-then-mv flow in TrustCACertFromBytes work
	// against tests that only scripted the live-bundle path. Tests
	// that *do* script `cat <path>` keep priority — runFunc is
	// consulted first and the upload mirror is the fallback.
	uploaded map[string][]byte
}

// probeScriptHeader is the first line of the batched probe script
// emitted by buildSpeakerProbeScript. We use it as a sentinel so that
// per-command test mocks (which only know `cat` / `[ -f ]` / etc.) can
// still satisfy GetMigrationSummary after the SSH probes were batched
// into a single Run() call — the mock synthesizes the framed probe
// response by invoking its existing runFunc for each path the script
// would have probed.
const probeScriptHeader = "echo '@SSH_OK@'"

func (m *mockSSH) Run(command string) (string, error) {
	if strings.HasPrefix(command, probeScriptHeader) {
		return m.synthesizeProbeResponse(command)
	}

	if m.runFunc != nil {
		out, err := m.runFunc(command)
		if err != nil {
			return out, err
		}

		if out != "" {
			return out, nil
		}
		// runFunc returned ("", nil) — fall through to the upload
		// mirror so tmp readbacks that the test didn't script
		// explicitly still produce the bytes we just wrote there.
	}

	if strings.HasPrefix(command, "cat ") {
		path := strings.TrimPrefix(command, "cat ")
		if body, ok := m.uploaded[path]; ok {
			return string(body), nil
		}
	}

	return "", nil
}

// synthesizeProbeResponse parses the batched probe script for the file
// and existence paths it references, calls the test's runFunc to find
// out what each one "contains," and emits the framed response format
// that parseSpeakerProbe expects. Lets existing per-command test mocks
// drive the batched probe without any test-side changes.
//
// If runFunc errors on a simple reachability probe (`ls /`), we treat
// the SSH connection as down and return the same error — matching the
// behaviour tests expect when they wire a runFunc that errors on every
// command.
func (m *mockSSH) synthesizeProbeResponse(script string) (string, error) {
	if m.runFunc == nil {
		return "@SSH_OK@\n", nil
	}

	// SSH-reachability probe: if a simple read fails, the connection
	// itself is "down" in mock-land and the real batched script would
	// also have produced an error from ssh.Dial.
	if _, err := m.runFunc("ls /"); err != nil {
		return "", err
	}

	var out strings.Builder

	out.WriteString("@SSH_OK@\n")

	fileRE := regexp.MustCompile(`\[ -f '([^']+)' \]`)
	for _, match := range fileRE.FindAllStringSubmatch(script, -1) {
		path := match[1]

		content, err := m.runFunc("cat " + path)
		if err != nil || content == "" {
			continue
		}

		out.WriteString("@FILE@" + path + "@\n")
		out.WriteString(base64.StdEncoding.EncodeToString([]byte(content)))
		out.WriteString("\n@END@\n")
	}

	existsRE := regexp.MustCompile(`\[ -e '([^']+)' \]`)
	for _, match := range existsRE.FindAllStringSubmatch(script, -1) {
		path := match[1]
		if _, err := m.runFunc("[ -e " + path + " ]"); err == nil {
			out.WriteString("@EXISTS@" + path + "@\n")
		}
	}

	return out.String(), nil
}

func (m *mockSSH) UploadContent(content []byte, remotePath string) error {
	if m.uploaded == nil {
		m.uploaded = make(map[string][]byte)
	}

	m.uploaded[remotePath] = append([]byte(nil), content...)

	if m.uploadContentFunc != nil {
		return m.uploadContentFunc(content, remotePath)
	}
	return nil
}

func TestMigrateViaHosts(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://192.0.2.100:8000", nil, cm)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /etc/hosts" {
					// Handle both initial read and verification read
					if len(runCalls) > 2 { // Rough heuristic: verification happens after upload
						return "192.0.2.100\tstreaming.bose.com\n192.0.2.100\tupdates.bose.com\n192.0.2.100\tstats.bose.com\n192.0.2.100\tbmx.bose.com\n192.0.2.100\tcontent.api.bose.io\n192.0.2.100\tevents.api.bosecm.com\n192.0.2.100\taudionotification.api.bosecm.com\n192.0.2.100\taudionotificationdev.api.bosecm.com\n192.0.2.100\tbose-prod.apigee.net\n192.0.2.100\tworldwide.bose.com\n192.0.2.100\tmedia.bose.io\n192.0.2.100\tdownloads.bose.com\n192.0.2.100\tvoice.api.bose.io", nil
					}
					return "127.0.0.1 localhost", nil
				}
				if strings.HasPrefix(command, "[ -f") {
					return "", fmt.Errorf("file not found")
				}
				if strings.HasPrefix(command, "grep -F") {
					return "", fmt.Errorf("not found")
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				if remotePath == "/etc/hosts" {
					if !strings.Contains(string(content), "192.0.2.100\tstreaming.bose.com") {
						t.Errorf("Expected hosts content to contain redirect, got %s", string(content))
					}
				}
				return nil
			},
		}
	}

	_, err = m.migrateViaHosts("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("migrateViaHosts failed: %v", err)
	}

	// Verify backups were attempted
	foundHostsBackup := false
	foundBundleBackup := false
	for _, call := range runCalls {
		if strings.Contains(call, "cp /etc/hosts /etc/hosts.original") {
			foundHostsBackup = true
		}
		if strings.Contains(call, "cp /etc/pki/tls/certs/ca-bundle.crt /etc/pki/tls/certs/ca-bundle.crt.original") {
			foundBundleBackup = true
		}
	}
	if !foundHostsBackup {
		t.Errorf("Expected /etc/hosts backup to be attempted")
	}
	if !foundBundleBackup {
		t.Errorf("Expected ca-bundle.crt backup to be attempted")
	}

	// Verify reboot was NOT called
	foundReboot := false
	for _, call := range runCalls {
		if strings.Contains(call, "reboot") {
			foundReboot = true
			break
		}
	}
	if foundReboot {
		t.Errorf("Expected reboot NOT to be called automatically")
	}
}

func TestMigrateViaHosts_UpdateExisting(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-update")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	_ = cm.EnsureCA()

	m := NewManager("http://192.0.2.100:8000", nil, cm)

	m.NewSSH = func(host string) SSHClient {
		runCount := 0
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCount++
				if command == "cat /etc/hosts" {
					if runCount > 1 {
						return "127.0.0.1 localhost\n192.0.2.100\tstreaming.bose.com\n192.0.2.100\tupdates.bose.com\n192.0.2.100\tstats.bose.com\n192.0.2.100\tbmx.bose.com\n192.0.2.100\tcontent.api.bose.io\n192.0.2.100\tevents.api.bosecm.com\n192.0.2.100\taudionotification.api.bosecm.com\n192.0.2.100\taudionotificationdev.api.bosecm.com\n192.0.2.100\tbose-prod.apigee.net\n192.0.2.100\tworldwide.bose.com\n192.0.2.100\tmedia.bose.io\n192.0.2.100\tdownloads.bose.com\n192.0.2.100\tvoice.api.bose.io", nil
					}
					return "127.0.0.1 localhost\n1.2.3.4\tstreaming.bose.com\n1.2.3.4\tupdates.bose.com", nil
				}
				if strings.HasPrefix(command, "[ -f") {
					return "", nil // Backup already exists
				}
				if strings.HasPrefix(command, "grep -F") {
					return "matched", nil // CA already trusted
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				if remotePath == "/etc/hosts" {
					c := string(content)
					if !strings.Contains(c, "192.0.2.100\tstreaming.bose.com") {
						t.Errorf("Expected updated IP for streaming.bose.com, got:\n%s", c)
					}
					if !strings.Contains(c, "192.0.2.100\tupdates.bose.com") {
						t.Errorf("Expected updated IP for updates.bose.com, got:\n%s", c)
					}
					if !strings.Contains(c, "192.0.2.100\tevents.api.bosecm.com") {
						t.Errorf("Expected new domain events.api.bosecm.com, got:\n%s", c)
					}
					// Ensure no duplicates
					if strings.Count(c, "streaming.bose.com") != 1 {
						t.Errorf("Expected streaming.bose.com to appear exactly once, got %d", strings.Count(c, "streaming.bose.com"))
					}
				}
				return nil
			},
		}
	}

	_, err = m.migrateViaHosts("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("migrateViaHosts failed: %v", err)
	}
}

func TestGetLiveDeviceInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Errorf("Expected to request /info, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="AABBCCDDEE0A">
    <name>Test Speaker</name>
    <type>SoundTouch 20</type>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>19.0.5</softwareVersion>
            <serialNumber>AABBCCDDEE0A</serialNumber>
        </component>
    </components>
</info>`)
	}))
	defer server.Close()

	// Extract IP and port from the test server URL
	// The test server URL is like http://127.0.0.1:54321
	host := server.Listener.Addr().String()

	manager := NewManager("http://localhost:8000", nil, nil)

	info, err := manager.GetLiveDeviceInfo(host)
	if err != nil {
		t.Fatalf("Failed to get live device info: %v", err)
	}

	if info.Name != "Test Speaker" {
		t.Errorf("Expected Name 'Test Speaker', got '%s'", info.Name)
	}

	if info.SoftwareVer != "19.0.5" {
		t.Errorf("Expected SoftwareVer '19.0.5', got '%s'", info.SoftwareVer)
	}

	if info.SerialNumber != "AABBCCDDEE0A" {
		t.Errorf("Expected SerialNumber 'AABBCCDDEE0A', got '%s'", info.SerialNumber)
	}
}

func TestGetMigrationSummary_SSHFailure(t *testing.T) {
	// Use an IP that is unlikely to have an SSH server running or reachable
	// or use a local port that is closed.
	// We'll use a local port that we know is closed.
	manager := NewManager("http://localhost:8000", nil, nil)
	summary, err := manager.GetMigrationSummary("127.0.0.1", "", "", nil)

	// Currently it might return an error OR it might return a summary with SSHSuccess: false
	// but the issue description says the user is told connection SUCCEEDED.

	if err == nil {
		if summary.SSHSuccess {
			t.Errorf("Expected SSHSuccess to be false for closed port, got true")
		}

		if summary.CurrentConfig == "" {
			t.Errorf("Expected CurrentConfig to contain error message, got empty string")
		}
	} else {
		t.Errorf("Expected no error from GetMigrationSummary, got %v", err)
	}
}

func TestGetMigrationSummary_WithProxyOptions(t *testing.T) {
	// Setup a mock server for live info
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<info deviceID="123"><name>Test</name></info>`)
	}))
	defer server.Close()

	host := server.Listener.Addr().String()
	manager := NewManager("http://st-service:8000", nil, nil)

	// Since we can't easily mock SSH here without a full SSH server,
	// we are testing the logic that depends on ParsedCurrentConfig being nil or not.
	// However, GetMigrationSummary tries to connect via SSH.
	// If SSH fails, ParsedCurrentConfig will be nil.

	options := map[string]string{
		"marge":     "proxied",
		"stats":     "self",
		"sw_update": "proxied",
		"bmx":       "self",
	}

	summary, err := manager.GetMigrationSummary(host, "http://target:8000", "http://proxy:8000", options)
	if err != nil {
		t.Fatalf("GetMigrationSummary failed: %v", err)
	}

	// When SSH fails (which it will here), PlannedConfig should be the default one for target:8000
	if !contains(summary.PlannedConfig, "http://target:8000") {
		t.Errorf("Expected default marge URL when SSH fails, got: %s", summary.PlannedConfig)
	}

	// Test PlannedResolv
	if !contains(summary.PlannedResolv, "nameserver target") {
		t.Errorf("Expected PlannedResolv to contain nameserver target, got: %s", summary.PlannedResolv)
	}
}

func TestCheckCACertTrusted(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ca-trust-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://localhost:8000", nil, cm)

	// Test 1: Found via label
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.HasPrefix(command, "grep -F") && strings.Contains(command, CALabel) {
					return CALabel, nil
				}
				return "", nil
			},
		}
	}

	summary := &MigrationSummary{}
	m.checkCACertTrusted(summary, "192.0.2.10")
	if !summary.CACertTrusted {
		t.Errorf("Expected CACertTrusted to be true when label is found")
	}

	// Test 2: Found via data snippet (label missing)
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.HasPrefix(command, "grep -F") {
					if strings.Contains(command, CALabel) {
						return "", fmt.Errorf("not found")
					}
					// Searching for cert data
					return "found data", nil
				}
				return "", nil
			},
		}
	}

	summary = &MigrationSummary{}
	m.checkCACertTrusted(summary, "192.0.2.10")
	if !summary.CACertTrusted {
		t.Errorf("Expected CACertTrusted to be true when cert data is found")
	}

	// Test 3: Not found
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.HasPrefix(command, "grep -F") {
					return "", fmt.Errorf("not found")
				}
				return "", nil
			},
		}
	}

	summary = &MigrationSummary{}
	m.checkCACertTrusted(summary, "192.0.2.10")
	if summary.CACertTrusted {
		t.Errorf("Expected CACertTrusted to be false when nothing is found")
	}
}

func TestTestConnection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-connection")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://localhost:8000", nil, cm)

	runCalls := []string{}
	uploadCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if strings.Contains(command, "curl") {
					return "HTTP/1.1 200 OK", nil
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploadCalls = append(uploadCalls, remotePath)
				return nil
			},
		}
	}

	// Test 1: Shared trust store (no explicit CA)
	output, err := m.TestConnection("192.0.2.10", "https://localhost:8443/health", false)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if !strings.Contains(output, "200 OK") {
		t.Errorf("Expected output to contain '200 OK', got %s", output)
	}
	if len(uploadCalls) != 0 {
		t.Errorf("Expected no uploads for shared trust store test, got %v", uploadCalls)
	}

	// Test 2: Explicit CA
	output, err = m.TestConnection("192.0.2.10", "https://localhost:8443/health", true)
	if err != nil {
		t.Fatalf("TestConnection failed: %v", err)
	}
	if !strings.Contains(output, "200 OK") {
		t.Errorf("Expected output to contain '200 OK', got %s", output)
	}
	foundUpload := false
	for _, path := range uploadCalls {
		if path == "/tmp/soundtouch-test-ca.crt" {
			foundUpload = true
			break
		}
	}
	if !foundUpload {
		t.Errorf("Expected CA to be uploaded to /tmp/soundtouch-test-ca.crt")
	}

	foundCurlWithCA := false
	for _, call := range runCalls {
		if strings.Contains(call, "curl") && strings.Contains(call, "--cacert /tmp/soundtouch-test-ca.crt") {
			foundCurlWithCA = true
			break
		}
	}
	if !foundCurlWithCA {
		t.Errorf("Expected curl command to use --cacert")
	}

	// Verify cleanup
	foundRm := false
	for _, call := range runCalls {
		if call == "rm /tmp/soundtouch-test-ca.crt" {
			foundRm = true
			break
		}
	}
	if !foundRm {
		t.Errorf("Expected cleanup command 'rm /tmp/soundtouch-test-ca.crt' to be called")
	}
}

func TestTestHostsRedirection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hosts-redirection-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://localhost:8000", nil, cm)

	runCalls := []string{}
	uploadCalls := []string{}
	var currentHostsContent = "127.0.0.1 localhost\n"
	m.NewSSH = func(host string) SSHClient {
		mock := &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /etc/hosts" {
					return currentHostsContent, nil
				}
				if strings.Contains(command, "curl") {
					return "HTTP/1.1 200 OK", nil
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploadCalls = append(uploadCalls, remotePath)
				if remotePath == "/etc/hosts" {
					currentHostsContent = string(content)
					if strings.Contains(string(content), "custom-test-api.bose.fake") {
						if !strings.Contains(string(content), "1.2.3.4\tcustom-test-api.bose.fake") {
							t.Errorf("Expected hosts content to contain test redirect with IP 1.2.3.4, got %s", string(content))
						}
					}
				}
				return nil
			},
		}
		return mock
	}

	output, err := m.TestHostsRedirection("192.0.2.10", "http://1.2.3.4:8000")
	if err != nil {
		t.Fatalf("TestHostsRedirection failed: %v", err)
	}

	if !strings.Contains(output, "200 OK") {
		t.Errorf("Expected output to contain '200 OK', got %s", output)
	}

	// Verify upload of test hosts
	foundHostsUpload := false
	foundCAUpload := false
	for _, path := range uploadCalls {
		if path == "/etc/hosts" {
			foundHostsUpload = true
		}
		if path == "/tmp/soundtouch-test-ca.crt" {
			foundCAUpload = true
		}
	}
	if !foundHostsUpload {
		t.Errorf("Expected /etc/hosts to be uploaded")
	}
	if !foundCAUpload {
		t.Errorf("Expected CA to be uploaded to /tmp/soundtouch-test-ca.crt")
	}

	// Verify curl calls for both HTTP and HTTPS
	foundHTTP := false
	foundHTTPSWithCA := false
	for _, call := range runCalls {
		if strings.Contains(call, "curl") {
			if strings.Contains(call, "http://") {
				foundHTTP = true
			}
			if strings.Contains(call, "https://") && strings.Contains(call, "--cacert /tmp/soundtouch-test-ca.crt") {
				foundHTTPSWithCA = true
			}
		}
	}
	if !foundHTTP {
		t.Errorf("Expected HTTP curl call")
	}
	if !foundHTTPSWithCA {
		t.Errorf("Expected HTTPS curl call with --cacert")
	}

	// Verify cleanup
	foundRmCA := false
	for _, call := range runCalls {
		if call == "rm /tmp/soundtouch-test-ca.crt" {
			foundRmCA = true
			break
		}
	}
	if !foundRmCA {
		t.Errorf("Expected cleanup command 'rm /tmp/soundtouch-test-ca.crt' to be called")
	}

	cleanupHostsCount := 0
	for _, path := range uploadCalls {
		if path == "/etc/hosts" {
			cleanupHostsCount++
		}
	}
	if cleanupHostsCount < 2 {
		t.Errorf("Expected at least 2 uploads to /etc/hosts (one for test, one for cleanup), got %d", cleanupHostsCount)
	}
}

func TestResolveIP(t *testing.T) {
	m := &Manager{}

	// IP passthrough: no resolution needed, no error
	ip, err := m.resolveIP("1.2.3.4", nil)
	if ip != "1.2.3.4" || err != nil {
		t.Errorf("Expected 1.2.3.4/nil, got %s/%v", ip, err)
	}

	// localhost resolves from service DNS; error expected (no SSH client)
	ip, err = m.resolveIP("localhost", nil)
	if ip != "127.0.0.1" && ip != "::1" {
		t.Errorf("Expected localhost resolution, got %s", ip)
	}
	if err == nil {
		t.Errorf("Expected error for service-side fallback, got nil")
	}

	// Device SSH ping succeeds: IP returned, no error
	mock := &mockSSH{
		runFunc: func(command string) (string, error) {
			if strings.Contains(command, "ping -c 1 myhost") {
				return "PING myhost (10.0.0.5): 56 data bytes", nil
			}
			return "", nil
		},
	}
	ip, err = m.resolveIP("myhost", mock)
	if ip != "10.0.0.5" || err != nil {
		t.Errorf("Expected 10.0.0.5/nil from device, got %s/%v", ip, err)
	}

	// Non-existent host, no SSH client: both methods fail, error returned
	ip, err = m.resolveIP("non-existent.host.fake", nil)
	if ip != "" {
		t.Errorf("Expected empty IP on failure, got %s", ip)
	}
	if err == nil {
		t.Errorf("Expected error for unresolvable host, got nil")
	}
}

// TestResolveIP_ServiceFallbackTagsSentinel pins the #282 fix:
// service-side fallback returns the resolved IP wrapped with
// ErrResolvedFromServiceOnly. Callers can errors.Is()-check the
// sentinel to distinguish informational fallback from a hard
// failure, which fixes the long-standing CLI/UI ❌ row that appeared
// every time a hostname target was used with SSH off.
func TestResolveIP_ServiceFallbackTagsSentinel(t *testing.T) {
	m := &Manager{}

	// No SSH client → forces the service-side fallback path.
	ip, err := m.resolveIP("localhost", nil)
	if ip != "127.0.0.1" && ip != "::1" {
		t.Fatalf("expected localhost to resolve service-side, got ip=%q err=%v", ip, err)
	}

	if err == nil {
		t.Fatalf("expected service-side fallback to surface ErrResolvedFromServiceOnly, got nil err")
	}

	if !errors.Is(err, ErrResolvedFromServiceOnly) {
		t.Errorf("expected error to wrap ErrResolvedFromServiceOnly, got %v", err)
	}
}

// TestResolveIP_DeviceSuccessReturnsNilError keeps the happy-path
// guarantee explicit alongside the sentinel-tagging contract above.
func TestResolveIP_DeviceSuccessReturnsNilError(t *testing.T) {
	m := &Manager{}

	mock := &mockSSH{
		runFunc: func(command string) (string, error) {
			if strings.Contains(command, "ping -c 1 myhost") {
				return "PING myhost (10.0.0.5): 56 data bytes", nil
			}
			return "", nil
		},
	}

	ip, err := m.resolveIP("myhost", mock)
	if ip != "10.0.0.5" {
		t.Errorf("expected 10.0.0.5, got %s", ip)
	}

	if err != nil {
		t.Errorf("device-side success must return nil error, got %v", err)
	}

	if errors.Is(err, ErrResolvedFromServiceOnly) {
		t.Errorf("device-side success must NOT carry ErrResolvedFromServiceOnly sentinel")
	}
}

func TestMigrateViaHosts_SkipCAIfTrusted(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-skip-ca")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://192.0.2.100:8000", nil, cm)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /etc/hosts" {
					// Handle both initial read and verification read
					if len(runCalls) > 2 { // Rough heuristic: verification happens after upload
						return "192.0.2.100\tstreaming.bose.com\n192.0.2.100\tupdates.bose.com\n192.0.2.100\tstats.bose.com\n192.0.2.100\tbmx.bose.com\n192.0.2.100\tcontent.api.bose.io\n192.0.2.100\tevents.api.bosecm.com\n192.0.2.100\taudionotification.api.bosecm.com\n192.0.2.100\taudionotificationdev.api.bosecm.com\n192.0.2.100\tbose-prod.apigee.net\n192.0.2.100\tworldwide.bose.com\n192.0.2.100\tmedia.bose.io\n192.0.2.100\tdownloads.bose.com\n192.0.2.100\tvoice.api.bose.io", nil
					}
					return "127.0.0.1 localhost", nil
				}
				if strings.HasPrefix(command, "grep -F") {
					// Simulate CA already trusted
					return "found", nil
				}
				return "", nil
			},
		}
	}

	_, err = m.migrateViaHosts("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("migrateViaHosts failed: %v", err)
	}

	// Verify CA injection was skipped
	foundCAInjection := false
	for _, call := range runCalls {
		if strings.Contains(call, "cat /tmp/local-ca.crt >> /etc/pki/tls/certs/ca-bundle.crt") {
			foundCAInjection = true
			break
		}
	}
	if foundCAInjection {
		t.Errorf("Expected CA injection to be skipped when already trusted")
	}
}

func TestTrustCACert(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "trust-ca-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://localhost:8000", nil, cm)

	runCalls := []string{}
	uploadCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if strings.HasPrefix(command, "[ -f") {
					return "", fmt.Errorf("file not found")
				}
				return "", nil
				// mockSSH automatically mirrors uploads back on
				// `cat <path>` when runFunc returns ("", nil), so the
				// post-upload tmp readback in TrustCACertFromBytes
				// works without test-side wiring.
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploadCalls = append(uploadCalls, remotePath)
				return nil
			},
		}
	}

	_, err = m.TrustCACert("192.0.2.10")
	if err != nil {
		t.Fatalf("TrustCACert failed: %v", err)
	}

	// Verify CA backup and injection
	foundBackup := false
	for _, call := range runCalls {
		if strings.Contains(call, "cp /etc/pki/tls/certs/ca-bundle.crt /etc/pki/tls/certs/ca-bundle.crt.original") {
			foundBackup = true
		}
	}

	if !foundBackup {
		t.Errorf("Expected ca-bundle.crt backup")
	}

	// Verify CA upload landed on the tmp path (atomic-replace flow).
	foundTmpUpload := false
	for _, path := range uploadCalls {
		if path == "/etc/pki/tls/certs/ca-bundle.crt.aftertouch.tmp" {
			foundTmpUpload = true
			break
		}
	}

	if !foundTmpUpload {
		t.Errorf("Expected candidate bundle to be uploaded to ca-bundle.crt.aftertouch.tmp; got upload paths: %v", uploadCalls)
	}

	// Verify the live bundle was NOT touched directly by UploadContent —
	// the rename via Run() is the only path that touches the live file.
	for _, path := range uploadCalls {
		if path == "/etc/pki/tls/certs/ca-bundle.crt" {
			t.Errorf("UploadContent wrote directly to live bundle %s — atomic-replace flow expects tmp + mv only", path)
		}
	}

	// Verify the atomic rename ran and that no rm of the tmp happened
	// (rm only fires on a verification failure).
	foundMv := false
	foundRm := false

	for _, call := range runCalls {
		if call == "mv /etc/pki/tls/certs/ca-bundle.crt.aftertouch.tmp /etc/pki/tls/certs/ca-bundle.crt" {
			foundMv = true
		}

		if strings.HasPrefix(call, "rm -f /etc/pki/tls/certs/ca-bundle.crt.aftertouch.tmp") {
			foundRm = true
		}
	}

	if !foundMv {
		t.Errorf("Expected atomic mv from .aftertouch.tmp to live bundle; got run calls: %v", runCalls)
	}

	if foundRm {
		t.Errorf("Did not expect a cleanup rm on the happy path; got run calls: %v", runCalls)
	}
}

// TestTrustCACert_StripsMultipleStaleEntriesSilently pins the
// behaviour the user flagged for AfterTouch installs that pre-date
// the strip-then-append logic: live bundles in the field can carry
// two or more copies of our CA from older releases that appended
// without cleanup. The new install must:
//
//   - strip every stale AfterTouch entry,
//   - log how many duplicates were cleaned up,
//   - append exactly one fresh entry,
//   - upload to the tmp path,
//   - verify and rename — i.e. the cleanup itself must not break the
//     validation or trigger the rollback path.
func TestTrustCACert_StripsMultipleStaleEntriesSilently(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "trust-ca-multi-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://localhost:8000", nil, cm)

	const (
		bundlePath = "/etc/pki/tls/certs/ca-bundle.crt"
		tmpPath    = bundlePath + ".aftertouch.tmp"
	)

	// Pre-existing bundle: one legitimate upstream cert plus two
	// stale AfterTouch entries from old installs. Generated inline
	// to stay self-contained.
	upstream := generatePEMCertificate(t, "upstream-root")
	stale1 := generatePEMCertificate(t, "aftertouch-stale-1")
	stale2 := generatePEMCertificate(t, "aftertouch-stale-2")
	preexisting := string(upstream) +
		CALabel + "\n" + string(stale1) + CALabel + "\n" +
		CALabel + "\n" + string(stale2) + CALabel + "\n"

	runCalls := []string{}

	var sshMock *mockSSH

	m.NewSSH = func(_ string) SSHClient {
		sshMock = &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)

				if strings.HasPrefix(command, "[ -f") {
					// .original doesn't exist yet → triggers initial backup
					return "", fmt.Errorf("file not found")
				}

				if command == "cat "+bundlePath {
					return preexisting, nil
				}

				return "", nil
				// Tmp readback falls through to mockSSH's upload mirror.
			},
		}

		return sshMock
	}

	logs, err := m.TrustCACert("192.0.2.10")
	if err != nil {
		t.Fatalf("TrustCACert failed: %v", err)
	}

	if !strings.Contains(logs, "Cleaned up 2 duplicate AfterTouch CA entries") {
		t.Errorf("logs do not mention duplicate cleanup; got:\n%s", logs)
	}

	uploaded, ok := sshMock.uploaded[tmpPath]
	if !ok {
		t.Fatalf("nothing uploaded to %s; only got: %v", tmpPath, uploadKeys(sshMock.uploaded))
	}

	// The uploaded bundle must contain exactly two sentinels (open +
	// close) bracketing exactly one CERTIFICATE block, regardless of
	// how many stale entries the input had.
	if err := validateAfterTouchLabelBracketing(uploaded); err != nil {
		t.Errorf("uploaded bundle has malformed AfterTouch bracketing despite the cleanup: %v", err)
	}

	// And the cleanup must not have dropped the legitimate upstream cert.
	count, err := validateCABundleBytes(uploaded)
	if err != nil {
		t.Fatalf("uploaded bundle does not validate: %v", err)
	}

	if count != 2 {
		t.Errorf("uploaded bundle has %d CERTIFICATE blocks, want 2 (the upstream root + our fresh AfterTouch CA)", count)
	}

	// Atomic rename should have fired, and no rollback rm.
	foundMv := false
	foundRm := false

	for _, call := range runCalls {
		if call == "mv "+tmpPath+" "+bundlePath {
			foundMv = true
		}

		if strings.HasPrefix(call, "rm -f "+tmpPath) {
			foundRm = true
		}
	}

	if !foundMv {
		t.Errorf("Expected atomic mv after cleanup; got run calls: %v", runCalls)
	}

	if foundRm {
		t.Errorf("Cleanup path triggered rollback rm — multi-entry input should not be a failure case; got: %v", runCalls)
	}
}

func uploadKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

// TestTrustCACert_PostUploadVerificationFailureCleansUpTmp pins the
// rollback-free recovery story from issue #262: when the tmp file's
// readback doesn't validate (here we simulate transport truncation by
// returning the tmp content stripped of its closing AfterTouch label),
// the rename must NOT fire, the tmp must be removed, and the error
// must name the verification failure plus reassure the caller the
// live bundle wasn't touched.
func TestTrustCACert_PostUploadVerificationFailureCleansUpTmp(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "trust-ca-fail-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://localhost:8000", nil, cm)

	const (
		bundlePath = "/etc/pki/tls/certs/ca-bundle.crt"
		tmpPath    = bundlePath + ".aftertouch.tmp"
	)

	runCalls := []string{}

	var sshMock *mockSSH

	m.NewSSH = func(_ string) SSHClient {
		sshMock = &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)

				switch {
				case strings.HasPrefix(command, "[ -f"):
					return "", fmt.Errorf("file not found")
				case command == "cat "+tmpPath:
					// Simulate transport corruption: chop the closing
					// AfterTouch sentinel off the bytes mockSSH would
					// otherwise mirror back. Pre-upload validation
					// passed (the full bytes were well-formed), but
					// the readback doesn't bracket cleanly anymore.
					return strings.Replace(string(sshMock.uploaded[tmpPath]), "\n"+CALabel+"\n", "\n", 1), nil
				}

				return "", nil
			},
		}

		return sshMock
	}

	_, err = m.TrustCACert("192.0.2.10")
	if err == nil {
		t.Fatalf("TrustCACert succeeded, want a verification failure")
	}

	if !strings.Contains(err.Error(), "verification of "+tmpPath+" failed") {
		t.Errorf("error does not name the verification target: %v", err)
	}

	if !strings.Contains(err.Error(), "live bundle untouched") {
		t.Errorf("error does not reassure that the live bundle was untouched: %v", err)
	}

	foundRm := false
	foundMv := false

	for _, call := range runCalls {
		if call == "rm -f "+tmpPath {
			foundRm = true
		}

		if strings.HasPrefix(call, "mv "+tmpPath) {
			foundMv = true
		}
	}

	if !foundRm {
		t.Errorf("Expected cleanup rm of %s after verification failure; got run calls: %v", tmpPath, runCalls)
	}

	if foundMv {
		t.Errorf("mv ran despite verification failure — live bundle was overwritten with bad content; run calls: %v", runCalls)
	}
}

func TestRevertMigration(t *testing.T) {
	m := NewManager("http://localhost:8000", nil, nil)

	runCalls := []string{}
	uploadCalls := make(map[string]string)
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /etc/pki/tls/certs/ca-bundle.crt" {
					return "existing content\n" + CALabel + "\nCERT DATA\n" + CALabel + "\nmore content", nil
				}
				if command == "cat /mnt/nv/rc.local" {
					return "#!/bin/sh\n# Aftertouch DNS hook\nlogic\nfi\n", nil
				}
				// Mock file existence checks for .original files
				if strings.HasPrefix(command, "[ -f") {
					if strings.Contains(command, ".original") || strings.Contains(command, "/mnt/nv/soundtouch-service/aftertouch.resolv.conf") || strings.Contains(command, "/mnt/nv/aftertouch.resolv.conf") {
						return "", nil // file exists
					}
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploadCalls[remotePath] = string(content)
				return nil
			},
		}
	}

	_, err := m.RevertMigration("192.0.2.10")
	if err != nil {
		t.Fatalf("RevertMigration failed: %v", err)
	}

	// Verify revert commands
	foundXMLRevert := false
	foundHostsRevert := false
	foundResolvRevert := false
	foundChattrRemove := false
	foundReboot := false
	foundAftertouchConfRemove := false
	foundDHCPRevert := false

	for _, call := range runCalls {
		if strings.Contains(call, "cp "+SoundTouchSdkPrivateCfgPath+".original "+SoundTouchSdkPrivateCfgPath) {
			foundXMLRevert = true
		}
		if strings.Contains(call, "cp /etc/hosts.original /etc/hosts") {
			foundHostsRevert = true
		}
		if strings.Contains(call, "cp /etc/resolv.conf.original /etc/resolv.conf") {
			foundResolvRevert = true
		}
		if strings.Contains(call, "chattr -i /etc/resolv.conf") {
			foundChattrRemove = true
		}
		if strings.Contains(call, "reboot") {
			foundReboot = true
		}
		if strings.Contains(call, "rm /mnt/nv/aftertouch.resolv.conf") {
			foundAftertouchConfRemove = true
		}
		if strings.Contains(call, "cp /etc/udhcpc.d/50default.original /etc/udhcpc.d/50default") {
			foundDHCPRevert = true
		}
	}

	if !foundXMLRevert {
		t.Errorf("Expected XML config revert")
	}
	if !foundHostsRevert {
		t.Errorf("Expected /etc/hosts revert")
	}
	if !foundResolvRevert {
		t.Errorf("Expected /etc/resolv.conf revert")
	}
	if !foundChattrRemove {
		t.Errorf("Expected chattr -i /etc/resolv.conf")
	}
	if !foundAftertouchConfRemove {
		t.Errorf("Expected /mnt/nv/aftertouch.resolv.conf removal")
	}
	if !foundDHCPRevert {
		t.Errorf("Expected /etc/udhcpc.d/50default revert")
	}
	if foundReboot {
		t.Errorf("Expected reboot NOT to be called automatically during revert")
	}

	// Verify rc.local cleanup
	if content, ok := uploadCalls["/mnt/nv/rc.local"]; ok {
		if strings.Contains(content, "# Aftertouch DNS hook") {
			t.Errorf("Expected Aftertouch hook to be removed from rc.local, got: %s", content)
		}
	} else {
		t.Errorf("Expected rc.local to be updated")
	}

	// Verify RemoveRemoteServices was NOT called
	for _, call := range runCalls {
		if strings.Contains(call, "rm -f /etc/remote_services") {
			t.Errorf("Remote services should NOT be removed during revert")
		}
	}

	// Verify CA removal
	if content, ok := uploadCalls["/etc/pki/tls/certs/ca-bundle.crt"]; ok {
		if strings.Contains(content, CALabel) {
			t.Errorf("Expected CA label to be removed from bundle, got: %s", content)
		}
		if !strings.Contains(content, "existing content") || !strings.Contains(content, "more content") {
			t.Errorf("Expected existing content to be preserved in bundle, got: %s", content)
		}
	} else {
		t.Errorf("Expected updated bundle to be uploaded")
	}
}

func TestRevertMigration_CorruptedRcLocal(t *testing.T) {
	m := NewManager("http://localhost:8000", nil, nil)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /mnt/nv/rc.local" {
					return "cat: can't open '/mnt/nv/rc.local': No such file or directory", nil
				}
				if strings.HasPrefix(command, "[ -f") {
					if strings.Contains(command, ".original") {
						if strings.Contains(command, "SoundTouchSdkPrivateCfg.xml") {
							return "", nil // Pretend XML backup exists to satisfy RevertMigration
						}
						return "", fmt.Errorf("not found")
					}
				}
				return "", nil
			},
		}
	}

	_, err := m.RevertMigration("192.0.2.10")
	if err != nil {
		t.Fatalf("RevertMigration failed: %v", err)
	}

	foundRmRcLocal := false
	for _, call := range runCalls {
		if call == "rm /mnt/nv/rc.local" {
			foundRmRcLocal = true
			break
		}
	}
	if !foundRmRcLocal {
		t.Errorf("Expected corrupted rc.local to be removed")
	}
}

func TestRevertMigration_NoBackup(t *testing.T) {
	m := NewManager("http://localhost:8000", nil, nil)

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.HasPrefix(command, "[ -f") {
					return "", fmt.Errorf("file not found")
				}
				return "", nil
			},
		}
	}

	_, err := m.RevertMigration("192.0.2.10")
	if err == nil {
		t.Errorf("Expected error when backup is missing, got nil")
	} else if !strings.Contains(err.Error(), "backup") {
		t.Errorf("Expected error about missing backup, got: %v", err)
	}
}

func TestReboot(t *testing.T) {
	m := NewManager("http://localhost:8000", nil, nil)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				return "", nil
			},
		}
	}

	_, err := m.Reboot("192.0.2.10", "")
	if err != nil {
		t.Fatalf("Reboot failed: %v", err)
	}

	foundReboot := false
	for _, call := range runCalls {
		if strings.Contains(call, "reboot") {
			foundReboot = true
			break
		}
	}
	if !foundReboot {
		t.Errorf("Expected reboot command to be called")
	}
}

func TestTestDNSRedirection(t *testing.T) {
	m := NewManager("http://192.0.2.100:8000", nil, nil)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if !strings.Contains(command, "-u") && strings.Contains(command, "nc") {
					// Verify TCP length prefix is present: \x00\x21
					if !strings.Contains(command, "\\x00\\x21") {
						return "", fmt.Errorf("missing TCP length prefix in nc command")
					}
					// Mock od output: " 192 0 2 100"
					return " 192 0 2 100", nil
				}
				if strings.HasPrefix(command, "nslookup aftertouch.test 192.0.2.100") {
					return "Server: 192.0.2.100\nAddress 1: 192.0.2.100\n\nName: aftertouch.test\nAddress 1: 192.0.2.100", nil
				}
				return "", nil
			},
		}
	}

	output, err := m.TestDNSRedirection("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("TestDNSRedirection failed: %v", err)
	}

	if !strings.Contains(output, "192.0.2.100") {
		t.Errorf("Expected output to contain service IP, got %s", output)
	}

	foundNc := false
	for _, call := range runCalls {
		if strings.Contains(call, "nc") && !strings.Contains(call, "-u") && strings.Contains(call, "192.0.2.100 53") {
			foundNc = true
			break
		}
	}
	if !foundNc {
		t.Errorf("Expected nc command with port 53, got calls: %v", runCalls)
	}
}

func TestTestDNSRedirection_CustomPort(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-dns-port")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()
	_ = ds.SaveSettings(datastore.Settings{
		DNSBindAddr: ":1053",
	})

	m := NewManager("http://192.0.2.100:8000", ds, nil)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if !strings.Contains(command, "-u") && strings.Contains(command, "nc") {
					// Verify TCP length prefix is present: \x00\x21
					if !strings.Contains(command, "\\x00\\x21") {
						return "", fmt.Errorf("missing TCP length prefix in nc command")
					}
					return " 192 0 2 100", nil
				}
				return "", nil
			},
		}
	}

	output, err := m.TestDNSRedirection("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("TestDNSRedirection failed: %v", err)
	}

	if !strings.Contains(output, "192.0.2.100") {
		t.Errorf("Expected output to contain service IP, got %s", output)
	}

	foundNc := false
	for _, call := range runCalls {
		if strings.Contains(call, "nc") && !strings.Contains(call, "-u") && strings.Contains(call, "192.0.2.100 1053") {
			foundNc = true
			break
		}
	}
	if !foundNc {
		t.Errorf("Expected nc command with custom port 1053, got calls: %v", runCalls)
	}
}

func TestBackupConfigOffDevice(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "backup-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	m := NewManager("http://localhost:8000", ds, nil)

	serial := "AABBCCDDEE0A"
	accountID := "1000001"

	// Mock info server
	infoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<info deviceID="%s"><name>Test</name><margeAccountUUID>%s</margeAccountUUID><components><component><componentCategory>SCM</componentCategory><serialNumber>%s</serialNumber></component></components></info>`, serial, accountID, serial)
	}))
	defer infoServer.Close()

	// Extract IP and port
	deviceIP := infoServer.Listener.Addr().String()

	// Mock SSH to return some config and hosts content
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.Contains(command, SoundTouchSdkPrivateCfgPath) {
					return "<SoundTouchSdkPrivateCfg><margeServerUrl>http://original</margeServerUrl></SoundTouchSdkPrivateCfg>", nil
				}
				if strings.Contains(command, "/etc/hosts") {
					return "127.0.0.1 localhost\n192.0.2.1 bmx.bose.com", nil
				}
				return "", nil
			},
		}
	}

	err = m.BackupConfigOffDevice(deviceIP)
	if err != nil {
		t.Fatalf("BackupConfigOffDevice failed: %v", err)
	}

	// Verify files were created in datastore
	deviceDir := m.DataStore.AccountDeviceDir(accountID, serial)
	configPath := filepath.Join(deviceDir, "SoundTouchSdkPrivateCfg.xml.bak")
	hostsPath := filepath.Join(deviceDir, "hosts.bak")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("Expected config backup at %s, but it doesn't exist", configPath)
	}

	if _, err := os.Stat(hostsPath); os.IsNotExist(err) {
		t.Errorf("Expected hosts backup at %s, but it doesn't exist", hostsPath)
	}

	// Verify content
	configContent, _ := os.ReadFile(configPath)
	if !strings.Contains(string(configContent), "http://original") {
		t.Errorf("Unexpected config backup content: %s", string(configContent))
	}

	hostsContent, _ := os.ReadFile(hostsPath)
	if !strings.Contains(string(hostsContent), "bmx.bose.com") {
		t.Errorf("Unexpected hosts backup content: %s", string(hostsContent))
	}
}

func TestMigrateSpeaker_PreFlightFailure(t *testing.T) {
	m := NewManager("http://localhost:8000", nil, nil)
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.Contains(command, "mount -o remount,rw /") {
					return "mount: / is read-only", fmt.Errorf("remount failed")
				}
				return "", nil
			},
		}
	}

	_, err := m.MigrateSpeaker("192.0.2.10", "", "", nil, MigrationMethodXML)
	if err == nil {
		t.Errorf("Expected error during pre-flight write check, got nil")
	}
	if !strings.Contains(err.Error(), "pre-flight check failed") {
		t.Errorf("Expected pre-flight error message, got: %v", err)
	}
}

func TestMigrateViaResolvConf(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-resolv")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://192.0.2.100:8000", nil, cm)

	runCalls := []string{}
	uploads := make(map[string]string)

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /mnt/nv/rc.local" {
					return "#!/bin/sh\n", nil
				}
				if strings.HasPrefix(command, "grep -q \"/mnt/nv/soundtouch-service/aftertouch.resolv.conf\"") {
					return "OK", nil
				}
				if strings.HasPrefix(command, "[ -f") {
					return "", fmt.Errorf("file not found")
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploads[remotePath] = string(content)
				return nil
			},
		}
	}

	_, err = m.migrateViaResolvConf("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("migrateViaResolvConf failed: %v", err)
	}

	// Verify uploads
	if !strings.Contains(uploads["/mnt/nv/soundtouch-service/aftertouch.resolv.conf"], "nameserver 192.0.2.100") {
		t.Errorf("aftertouch.resolv.conf missing nameserver")
	}

	if !strings.Contains(uploads["/mnt/nv/rc.local"], "/mnt/nv/soundtouch-service/aftertouch.resolv.conf") {
		t.Errorf("rc.local missing hook logic")
	}

	// Verify immediate patch
	foundPatch := false
	for _, call := range runCalls {
		if strings.Contains(call, "sed -i") && strings.Contains(call, "/etc/udhcpc.d/50default") {
			foundPatch = true
			break
		}
	}
	if !foundPatch {
		t.Errorf("Expected immediate patch to /etc/udhcpc.d/50default")
	}
}

func TestMigrateViaResolvConf_CorruptedRcLocal(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-resolv-corrupted")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://192.0.2.100:8000", nil, cm)

	uploads := make(map[string]string)

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if command == "cat /mnt/nv/rc.local" {
					// Simulate corrupted file containing error message
					return "cat: can't open '/mnt/nv/rc.local': No such file or directory", nil
				}
				if strings.HasPrefix(command, "grep -q \"/mnt/nv/soundtouch-service/aftertouch.resolv.conf\"") {
					return "OK", nil
				}
				if strings.HasPrefix(command, "[ -f") {
					return "", fmt.Errorf("file not found")
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploads[remotePath] = string(content)
				return nil
			},
		}
	}

	_, err = m.migrateViaResolvConf("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("migrateViaResolvConf failed: %v", err)
	}

	// Verify uploads - rc.local should have been sanitized and only contain shebang and hook
	rcLocal := uploads["/mnt/nv/rc.local"]
	if strings.Contains(rcLocal, "cat: can't open") {
		t.Errorf("rc.local still contains corrupted content: %s", rcLocal)
	}
	if !strings.HasPrefix(rcLocal, "#!/bin/sh") {
		t.Errorf("rc.local missing shebang: %s", rcLocal)
	}
	if !strings.Contains(rcLocal, "/mnt/nv/soundtouch-service/aftertouch.resolv.conf") {
		t.Errorf("rc.local missing hook logic: %s", rcLocal)
	}
}

func TestMigrateViaResolvConf_UdhcpcScript(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-resolv-script")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	m := NewManager("http://192.0.2.100:8000", nil, cm)

	runCalls := []string{}
	uploads := make(map[string]string)

	targetScript := "/opt/Bose/udhcpc.script"

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /mnt/nv/rc.local" {
					return "#!/bin/sh\n", nil
				}
				if strings.HasPrefix(command, "grep -q \"/mnt/nv/soundtouch-service/aftertouch.resolv.conf\"") {
					return "OK", nil
				}
				if command == "[ -f "+targetScript+" ]" {
					return "", nil // file exists
				}
				if strings.HasPrefix(command, "[ -f") {
					return "", fmt.Errorf("file not found")
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploads[remotePath] = string(content)
				return nil
			},
		}
	}

	_, err = m.migrateViaResolvConf("192.0.2.10", "http://192.0.2.100:8000")
	if err != nil {
		t.Fatalf("migrateViaResolvConf failed: %v", err)
	}

	// Verify immediate patch to udhcpc.script
	foundPatch := false
	for _, call := range runCalls {
		if strings.Contains(call, "sed -i") && strings.Contains(call, targetScript) {
			foundPatch = true
			break
		}
	}
	if !foundPatch {
		t.Errorf("Expected immediate patch to %s", targetScript)
	}

	// Verify rc.local contains patch for udhcpc.script
	rcLocal := uploads["/mnt/nv/rc.local"]
	if !strings.Contains(rcLocal, "targetScript=\"/opt/Bose/udhcpc.script\"") {
		t.Errorf("rc.local missing targetScript definition: %s", rcLocal)
	}
	if !strings.Contains(rcLocal, "sed -i '/echo \"search \\$search_list # \\$interface\" >> \\$RESOLV_CONF/a \\                [ -f '\"$HOOK_MARKER\"' ] && cat '\"$HOOK_MARKER\"' >> '\"\\$RESOLV_CONF\"' && dns=\"\"' \"$targetScript\"") {
		// Note: The actual string in rcLocal might have variables expanded or escaped depending on how it was constructed.
		// Let's check for the critical part: the escaped $RESOLV_CONF
		if !strings.Contains(rcLocal, ">> '\"\\$RESOLV_CONF\"'") {
			t.Errorf("rc.local missing correctly escaped RESOLV_CONF in sed patch for udhcpc.script: %s", rcLocal)
		}
	}
}

func TestRevertMigration_ResolvConf(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test-revert-resolv")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m := NewManager("http://192.0.2.100:8000", nil, nil)

	runCalls := []string{}
	uploads := make(map[string]string)
	targetDHCPFile := "/etc/udhcpc.d/50default"
	targetScript := "/opt/Bose/udhcpc.script"

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /mnt/nv/rc.local" {
					return "#!/bin/sh\n# Aftertouch DNS hook\nif [ -f \"/mnt/nv/soundtouch-service/aftertouch.resolv.conf\" ]; then\n    sed ...\nfi\n", nil
				}
				if strings.Contains(command, ".original ]") {
					return "", nil // backup exists
				}
				if strings.Contains(command, "[ -f /mnt/nv/soundtouch-service/aftertouch.resolv.conf ]") || strings.Contains(command, "[ -f /mnt/nv/aftertouch.resolv.conf ]") {
					return "", nil
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploads[remotePath] = string(content)
				return nil
			},
		}
	}

	_, err = m.RevertMigration("192.0.2.10")
	if err != nil {
		t.Fatalf("RevertMigration failed: %v", err)
	}

	// Verify backups were restored
	foundDHCPRestore := false
	foundScriptRestore := false
	for _, call := range runCalls {
		if strings.Contains(call, "cp "+targetDHCPFile+".original "+targetDHCPFile) {
			foundDHCPRestore = true
		}
		if strings.Contains(call, "cp "+targetScript+".original "+targetScript) {
			foundScriptRestore = true
		}
	}

	if !foundDHCPRestore {
		t.Errorf("Expected %s to be restored from backup", targetDHCPFile)
	}
	if !foundScriptRestore {
		t.Errorf("Expected %s to be restored from backup", targetScript)
	}

	// Verify rc.local was cleaned up
	rcLocal := uploads["/mnt/nv/rc.local"]
	if strings.Contains(rcLocal, "# Aftertouch DNS hook") {
		t.Errorf("rc.local still contains hook logic after revert: %s", rcLocal)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestCheckIsMigrated(t *testing.T) {
	m := NewManager("http://aftertouch:8000", nil, nil)

	t.Run("XML Migrated", func(t *testing.T) {
		summary := &MigrationSummary{
			SSHSuccess: true,
			ParsedCurrentConfig: &PrivateCfg{
				MargeServerUrl: "http://aftertouch:8000",
			},
		}
		m.checkIsMigrated(summary, "127.0.0.1")
		if !summary.IsMigrated {
			t.Errorf("Expected IsMigrated to be true for XML migration")
		}
	})

	t.Run("Hosts Migrated", func(t *testing.T) {
		m.NewSSH = func(host string) SSHClient {
			return &mockSSH{
				runFunc: func(command string) (string, error) {
					if command == "cat /etc/hosts" {
						return "127.0.0.1\tstreaming.bose.com", nil
					}
					return "", nil
				},
			}
		}
		summary := &MigrationSummary{
			SSHSuccess:    true,
			CACertTrusted: true,
		}
		m.checkIsMigrated(summary, "127.0.0.1")
		if !summary.IsMigrated {
			t.Errorf("Expected IsMigrated to be true for hosts migration")
		}
	})

	t.Run("ResolvConf Migrated (Marker)", func(t *testing.T) {
		m.NewSSH = func(host string) SSHClient {
			return &mockSSH{
				runFunc: func(command string) (string, error) {
					if command == "cat /etc/hosts" {
						return "127.0.0.1\tlocalhost", nil
					}
					if command == "[ -f /mnt/nv/aftertouch.resolv.conf ]" {
						return "", fmt.Errorf("not found")
					}
					return "", nil
				},
			}
		}
		summary := &MigrationSummary{
			SSHSuccess:        true,
			CACertTrusted:     true,
			CurrentResolvConf: "# Priority nameserver for Bose service redirection\nnameserver 192.0.2.1\n",
		}
		m.checkIsMigrated(summary, "127.0.0.1")
		if !summary.IsMigrated {
			t.Errorf("Expected IsMigrated to be true for resolv.conf migration with marker comment")
		}
	})

	t.Run("ResolvConf Migrated (IP)", func(t *testing.T) {
		m.NewSSH = func(host string) SSHClient {
			return &mockSSH{
				runFunc: func(command string) (string, error) {
					if command == "cat /etc/hosts" {
						return "127.0.0.1\tlocalhost", nil
					}
					if command == "[ -f /mnt/nv/aftertouch.resolv.conf ]" {
						return "", fmt.Errorf("not found")
					}
					// Mock resolveIP by mocking its SSH commands if any, or just wait for it to return targetHost
					return "", nil
				},
			}
		}
		// m.ServerURL is "http://aftertouch:8000" in this test (see top of TestCheckIsMigrated)
		summary := &MigrationSummary{
			SSHSuccess:        true,
			CACertTrusted:     true,
			CurrentResolvConf: "nameserver aftertouch\n",
		}
		m.checkIsMigrated(summary, "127.0.0.1")
		if !summary.IsMigrated {
			t.Errorf("Expected IsMigrated to be true for resolv.conf migration with matching hostname/IP")
		}
	})

	t.Run("Not Migrated", func(t *testing.T) {
		m.NewSSH = func(host string) SSHClient {
			return &mockSSH{
				runFunc: func(command string) (string, error) {
					if command == "cat /etc/hosts" {
						return "127.0.0.1\tlocalhost", nil
					}
					return "", nil
				},
			}
		}
		summary := &MigrationSummary{
			SSHSuccess: true,
			ParsedCurrentConfig: &PrivateCfg{
				MargeServerUrl: "http://streaming.bose.com",
			},
			CACertTrusted: false,
		}
		m.checkIsMigrated(summary, "127.0.0.1")
		if summary.IsMigrated {
			t.Errorf("Expected IsMigrated to be false for non-migrated device")
		}
	})
}

// TestCheckCurrentConfig_ReadsOriginalPath verifies that checkCurrentConfig reads
// from SoundTouchSdkPrivateCfgPath on an unmigrated device (issue #214 regression test).
func TestCheckCurrentConfig_ReadsOriginalPath(t *testing.T) {
	m := NewManager("http://aftertouch:8000", nil, nil)

	originalCfg := "<SoundTouchSdkPrivateCfg><margeServerUrl>http://streaming.bose.com</margeServerUrl></SoundTouchSdkPrivateCfg>"

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				if strings.HasPrefix(command, "[ -f ") && strings.Contains(command, ".original") {
					return "", fmt.Errorf("exit status 1")
				}
				if command == fmt.Sprintf("cat %s", SoundTouchSdkPrivateCfgPath) {
					return originalCfg, nil
				}
				return "", nil
			},
		}
	}

	summary := &MigrationSummary{}
	cfg, err := m.checkCurrentConfig(summary, "127.0.0.1")
	if err != nil {
		t.Fatalf("checkCurrentConfig returned unexpected error: %v", err)
	}
	if cfg != originalCfg {
		t.Errorf("Expected current config to be the original SoundTouchSdkPrivateCfg.xml, got %q", cfg)
	}
	if !summary.SSHSuccess {
		t.Errorf("Expected SSHSuccess to be true when original config is readable")
	}
}

func TestMigrateSpeaker_ResolvBlocking(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	m := NewManager("http://192.0.2.100:8000", ds, cm)

	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				return "", nil
			},
		}
	}

	// 1. DNS Disabled
	ds.SaveSettings(datastore.Settings{
		DNSEnabled:  false,
		DNSBindAddr: ":53",
	})

	// Mock HTTP server for device info
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<info deviceID="12345"><name>Test Speaker</name><type>ST10</type><maccAddress>00:11:22:33:44:55</maccAddress><margeAccountUUID>acc-123</margeAccountUUID></info>`))
		}
	}))
	defer ts.Close()

	// Use the test server address as device IP
	tsIP := strings.TrimPrefix(ts.URL, "http://")

	_, err = m.MigrateSpeaker(tsIP, "", "", nil, MigrationMethodResolvConf)
	if err == nil || !strings.Contains(err.Error(), "DNS discovery server is not enabled") {
		t.Errorf("Expected error about DNS not being enabled, got %v", err)
	}

	// 2. DNS Enabled but wrong port
	ds.SaveSettings(datastore.Settings{
		DNSEnabled:  true,
		DNSBindAddr: ":5353",
	})

	_, err = m.MigrateSpeaker(tsIP, "", "", nil, MigrationMethodResolvConf)
	if err == nil || !strings.Contains(err.Error(), "port 53 is required") {
		t.Errorf("Expected error about port 53 required, got %v", err)
	}

	// 3. DNS Enabled and port 53, but not running
	ds.SaveSettings(datastore.Settings{
		DNSEnabled:  true,
		DNSBindAddr: ":53",
	})

	m.GetDNSRunning = func() (bool, string) {
		return false, ":53"
	}

	_, err = m.MigrateSpeaker(tsIP, "", "", nil, MigrationMethodResolvConf)
	if err == nil || !strings.Contains(err.Error(), "not actually running") {
		t.Errorf("Expected error about DNS not actually running, got %v", err)
	}

	// 4. DNS Enabled and port 53, and running
	m.GetDNSRunning = func() (bool, string) {
		return true, ":53"
	}

	// This should now proceed to migrateViaResolvConf
	_, err = m.MigrateSpeaker(tsIP, "", "", nil, MigrationMethodResolvConf)
	if err != nil && (strings.Contains(err.Error(), "DNS discovery server is not enabled") ||
		strings.Contains(err.Error(), "port 53 is required") ||
		strings.Contains(err.Error(), "not actually running")) {
		t.Errorf("Did not expect pre-flight DNS errors, got %v", err)
	}
}
