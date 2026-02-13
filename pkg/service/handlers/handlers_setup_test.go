package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

func TestProxySettingsAPI(t *testing.T) {
	r, server := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Initial State
	server.proxyRedact = true
	server.proxyLogBody = false

	// 1. Test GET
	res, err := http.Get(ts.URL + "/setup/proxy-settings")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("GET: Expected status OK, got %v", res.Status)
	}

	var settings map[string]bool
	if decodeErr := json.NewDecoder(res.Body).Decode(&settings); decodeErr != nil {
		t.Fatalf("GET: Failed to decode response: %v", decodeErr)
	}

	if settings["redact"] != true || settings["log_body"] != false {
		t.Errorf("GET: Unexpected settings: %+v", settings)
	}

	// 2. Test POST
	update := map[string]bool{
		"redact":   false,
		"log_body": true,
	}

	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Failed to marshal update data: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/proxy-settings", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("POST: Expected status OK, got %v", res.Status)
	}

	// Verify server state
	if server.proxyRedact != false || server.proxyLogBody != true {
		t.Errorf("POST: Server state did not update: redact=%v, logBody=%v", server.proxyRedact, server.proxyLogBody)
	}

	res, err = http.Get(ts.URL + "/setup/proxy-settings")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if err := json.NewDecoder(res.Body).Decode(&settings); err != nil {
		t.Fatalf("GET (after update): Failed to decode response: %v", err)
	}

	if settings["redact"] != false || settings["log_body"] != true {
		t.Errorf("GET (after update): Unexpected settings: %+v", settings)
	}
}

func TestMigrationAndCA(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "handlers-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()
	cm := certmanager.NewCertificateManager(filepath.Join(tempDir, "certs"))
	_ = cm.EnsureCA()

	sm := setup.NewManager("http://localhost:8000", ds, cm)
	// Mock SSH to avoid real connections
	sm.NewSSH = func(host string) setup.SSHClient {
		return &mockSSH{}
	}

	r, server := setupRouter("http://localhost:8001", ds)
	server.sm = sm // Inject our manager with mock SSH

	ts := httptest.NewServer(r)
	defer ts.Close()

	// 1. Test GET /setup/ca.crt
	res, err := http.Get(ts.URL + "/setup/ca.crt")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("CA: Expected status OK, got %v", res.Status)
	}
	if res.Header.Get("Content-Type") != "application/x-x509-ca-cert" {
		t.Errorf("CA: Unexpected content type: %s", res.Header.Get("Content-Type"))
	}

	// 2. Test POST /setup/migrate/{deviceIP}?method=hosts
	res, err = http.Post(ts.URL+"/setup/migrate/192.168.1.10?method=hosts&target_url=http://192.168.1.100:8000", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Migrate: Expected status OK, got %v", res.Status)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("Migrate: Failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Migrate: Expected ok=true, got %v", result["ok"])
	}

	// 3. Test POST /setup/trust-ca/{deviceIP}
	res, err = http.Post(ts.URL+"/setup/trust-ca/192.168.1.10", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("TrustCA: Expected status OK, got %v", res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("TrustCA: Failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("TrustCA: Expected ok=true, got %v", result["ok"])
	}
}

type mockSSH struct{}

func (m *mockSSH) Run(command string) (string, error) {
	if command == "cat /etc/hosts" {
		return "127.0.0.1 localhost", nil
	}
	return "", nil
}
func (m *mockSSH) UploadContent(content []byte, remotePath string) error { return nil }
