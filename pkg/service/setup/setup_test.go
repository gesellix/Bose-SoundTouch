package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
)

type mockSSH struct {
	runFunc           func(command string) (string, error)
	uploadContentFunc func(content []byte, remotePath string) error
}

func (m *mockSSH) Run(command string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(command)
	}
	return "", nil
}

func (m *mockSSH) UploadContent(content []byte, remotePath string) error {
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

	m := NewManager("http://192.168.1.100:8000", nil, cm)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /etc/hosts" {
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
					if !strings.Contains(string(content), "192.168.1.100\tstreaming.bose.com") {
						t.Errorf("Expected hosts content to contain redirect, got %s", string(content))
					}
				}
				return nil
			},
		}
	}

	_, err = m.migrateViaHosts("192.168.1.10", "http://192.168.1.100:8000")
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

func TestGetLiveDeviceInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Errorf("Expected to request /info, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="08DF1F0BA325">
    <name>Test Speaker</name>
    <type>SoundTouch 20</type>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>19.0.5</softwareVersion>
            <serialNumber>08DF1F0BA325</serialNumber>
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

	if info.SerialNumber != "08DF1F0BA325" {
		t.Errorf("Expected SerialNumber '08DF1F0BA325', got '%s'", info.SerialNumber)
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
	manager := NewManager("http://soundcork:8000", nil, nil)

	// Since we can't easily mock SSH here without a full SSH server,
	// we are testing the logic that depends on ParsedCurrentConfig being nil or not.
	// However, GetMigrationSummary tries to connect via SSH.
	// If SSH fails, ParsedCurrentConfig will be nil.

	options := map[string]string{
		"marge":     "original",
		"stats":     "soundcork",
		"sw_update": "original",
		"bmx":       "soundcork",
	}

	summary, err := manager.GetMigrationSummary(host, "http://target:8000", "http://proxy:8000", options)
	if err != nil {
		t.Fatalf("GetMigrationSummary failed: %v", err)
	}

	// When SSH fails (which it will here), PlannedConfig should be the default one for target:8000
	if !contains(summary.PlannedConfig, "http://target:8000/marge") {
		t.Errorf("Expected default marge URL when SSH fails, got: %s", summary.PlannedConfig)
	}

	// Test PlannedHosts
	if !contains(summary.PlannedHosts, "target\tstreaming.bose.com") {
		t.Errorf("Expected PlannedHosts to contain redirect for target, got: %s", summary.PlannedHosts)
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
	m.checkCACertTrusted(summary, "192.168.1.10")
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
	m.checkCACertTrusted(summary, "192.168.1.10")
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
	m.checkCACertTrusted(summary, "192.168.1.10")
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
	output, err := m.TestConnection("192.168.1.10", "https://localhost:8443/health", false)
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
	output, err = m.TestConnection("192.168.1.10", "https://localhost:8443/health", true)
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

	output, err := m.TestHostsRedirection("192.168.1.10", "http://1.2.3.4:8000")
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

	// Test with IP
	if m.resolveIP("1.2.3.4", nil) != "1.2.3.4" {
		t.Errorf("Expected 1.2.3.4, got %s", m.resolveIP("1.2.3.4", nil))
	}

	// Test with localhost
	if m.resolveIP("localhost", nil) != "127.0.0.1" && m.resolveIP("localhost", nil) != "::1" {
		t.Errorf("Expected localhost resolution, got %s", m.resolveIP("localhost", nil))
	}

	// Test with device resolution (mocked)
	mock := &mockSSH{
		runFunc: func(command string) (string, error) {
			if strings.Contains(command, "ping -c 1 myhost") {
				return "PING myhost (10.0.0.5): 56 data bytes", nil
			}
			return "", nil
		},
	}
	if m.resolveIP("myhost", mock) != "10.0.0.5" {
		t.Errorf("Expected 10.0.0.5 from device, got %s", m.resolveIP("myhost", mock))
	}

	// Test with non-existent host (should fallback to input)
	if m.resolveIP("non-existent.host.fake", nil) != "non-existent.host.fake" {
		t.Errorf("Expected fallback to input, got %s", m.resolveIP("non-existent.host.fake", nil))
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

	m := NewManager("http://192.168.1.100:8000", nil, cm)

	runCalls := []string{}
	m.NewSSH = func(host string) SSHClient {
		return &mockSSH{
			runFunc: func(command string) (string, error) {
				runCalls = append(runCalls, command)
				if command == "cat /etc/hosts" {
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

	_, err = m.migrateViaHosts("192.168.1.10", "http://192.168.1.100:8000")
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
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploadCalls = append(uploadCalls, remotePath)
				return nil
			},
		}
	}

	_, err = m.TrustCACert("192.168.1.10")
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

	// Verify CA upload
	foundUpload := false
	for _, path := range uploadCalls {
		if path == "/etc/pki/tls/certs/ca-bundle.crt" {
			foundUpload = true
			break
		}
	}
	if !foundUpload {
		t.Errorf("Expected updated bundle to be uploaded to /etc/pki/tls/certs/ca-bundle.crt")
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
				// Mock file existence checks for .original files
				if strings.HasPrefix(command, "[ -f") && strings.Contains(command, ".original") {
					return "", nil // file exists
				}
				return "", nil
			},
			uploadContentFunc: func(content []byte, remotePath string) error {
				uploadCalls[remotePath] = string(content)
				return nil
			},
		}
	}

	_, err := m.RevertMigration("192.168.1.10")
	if err != nil {
		t.Fatalf("RevertMigration failed: %v", err)
	}

	// Verify revert commands
	foundXMLRevert := false
	foundHostsRevert := false
	foundReboot := false
	for _, call := range runCalls {
		if strings.Contains(call, "cp "+SoundTouchSdkPrivateCfgPath+".original "+SoundTouchSdkPrivateCfgPath) {
			foundXMLRevert = true
		}
		if strings.Contains(call, "cp /etc/hosts.original /etc/hosts") {
			foundHostsRevert = true
		}
		if strings.Contains(call, "reboot") {
			foundReboot = true
		}
	}

	if !foundXMLRevert {
		t.Errorf("Expected XML config revert")
	}
	if !foundHostsRevert {
		t.Errorf("Expected /etc/hosts revert")
	}
	if foundReboot {
		t.Errorf("Expected reboot NOT to be called automatically during revert")
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

	_, err := m.RevertMigration("192.168.1.10")
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

	_, err := m.Reboot("192.168.1.10")
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

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
