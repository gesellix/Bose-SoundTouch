package handlers

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// --- Registry unit tests ---

func TestAuthProbeRegistry_RegisterAndGet(t *testing.T) {
	reg := newAuthProbeRegistry(5 * time.Second)
	reg.register("nonce1", "DEVICEID01", "192.0.2.10")

	p, ok := reg.get("nonce1")
	if !ok {
		t.Fatal("expected probe to be found")
	}

	if p.Nonce != "nonce1" {
		t.Errorf("nonce = %q, want nonce1", p.Nonce)
	}

	if p.DeviceID != "DEVICEID01" {
		t.Errorf("deviceID = %q, want DEVICEID01", p.DeviceID)
	}

	if p.TargetIP != "192.0.2.10" {
		t.Errorf("targetIP = %q, want 192.0.2.10", p.TargetIP)
	}
}

func TestAuthProbeRegistry_ObserveMatch(t *testing.T) {
	reg := newAuthProbeRegistry(5 * time.Second)
	reg.register("nonce1", "DEVICEID01", "192.0.2.10")

	matched := reg.observe("nonce1", "192.0.2.10")
	if !matched {
		t.Fatal("expected observe to match")
	}

	p, ok := reg.get("nonce1")
	if !ok {
		t.Fatal("expected probe to still be present")
	}

	if p.ObservedFrom != "192.0.2.10" {
		t.Errorf("observedFrom = %q, want 192.0.2.10", p.ObservedFrom)
	}

	if p.ObservedAt.IsZero() {
		t.Error("observedAt is zero, expected a timestamp")
	}
}

func TestAuthProbeRegistry_ObserveNoMatch(t *testing.T) {
	reg := newAuthProbeRegistry(5 * time.Second)
	reg.register("nonce1", "DEVICEID01", "192.0.2.10")

	matched := reg.observe("wrong-nonce", "192.0.2.10")
	if matched {
		t.Fatal("expected observe to not match for unknown nonce")
	}
}

func TestAuthProbeRegistry_ObserveExpired(t *testing.T) {
	reg := newAuthProbeRegistry(1 * time.Millisecond)
	reg.register("nonce1", "DEVICEID01", "192.0.2.10")

	time.Sleep(10 * time.Millisecond)

	matched := reg.observe("nonce1", "192.0.2.10")
	if matched {
		t.Fatal("expected observe to not match for expired nonce")
	}
}

func TestAuthProbeRegistry_Deregister(t *testing.T) {
	reg := newAuthProbeRegistry(5 * time.Second)
	reg.register("nonce1", "DEVICEID01", "192.0.2.10")
	reg.deregister("nonce1")

	_, ok := reg.get("nonce1")
	if ok {
		t.Fatal("expected probe to be gone after deregister")
	}
}

func TestAuthProbeRegistry_PrunesExpired(t *testing.T) {
	reg := newAuthProbeRegistry(1 * time.Millisecond)
	reg.register("old", "DEV01", "192.0.2.10")

	time.Sleep(10 * time.Millisecond)

	// Registering a new entry triggers pruning of expired ones.
	reg.register("new", "DEV02", "192.0.2.11")

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if _, found := reg.active["old"]; found {
		t.Error("expected expired entry 'old' to be pruned")
	}

	if _, found := reg.active["new"]; !found {
		t.Error("expected new entry to be present")
	}
}

// --- HandleSpeakerAuth tests ---

func probeAuthRouter(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()

	ds := datastore.NewDataStore(t.TempDir())
	server := NewServer(ds, nil, "http://localhost:8001", false, false, false)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth", server.HandleSpeakerAuth)

	return server, mux
}

func TestHandleSpeakerAuth_EmptyHeader(t *testing.T) {
	_, mux := probeAuthRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHandleSpeakerAuth_UnknownToken(t *testing.T) {
	_, mux := probeAuthRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth", nil)
	req.Header.Set("Apikeyheader", "aftertouch")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for unknown token", rec.Code)
	}
}

func TestHandleSpeakerAuth_MatchingNonce_Returns403(t *testing.T) {
	server, mux := probeAuthRouter(t)

	// Register a probe nonce.
	server.authProbes.register("atp_testfixednonce", "DEVICEID01", "192.0.2.10")

	req := httptest.NewRequest(http.MethodGet, "/v1/auth", nil)
	req.Header.Set("Apikeyheader", "atp_testfixednonce")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for matching probe nonce", rec.Code)
	}

	// Also confirm the probe was recorded.
	p, ok := server.authProbes.get("atp_testfixednonce")
	if !ok {
		t.Fatal("expected probe to still be in registry")
	}

	if p.ObservedAt.IsZero() {
		t.Error("expected ObservedAt to be set")
	}
}

func TestHandleSpeakerAuth_ExpiredNonce_Returns200(t *testing.T) {
	server, mux := probeAuthRouter(t)

	// Register with a very short TTL.
	server.authProbes = newAuthProbeRegistry(1 * time.Millisecond)
	server.authProbes.register("atp_expirednonce", "DEVICEID01", "192.0.2.10")

	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth", nil)
	req.Header.Set("Apikeyheader", "atp_expirednonce")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for expired nonce", rec.Code)
	}
}

// --- End-to-end loop test ---

// fakeSpeaker is an httptest server that:
//  1. On POST /speaker: parses the app_key from the PlayInfo XML and calls
//     GET /v1/auth on afterTouchURL with Apikeyheader set to that key.
//  2. Returns 200 for the /speaker request.
func fakeSpeaker(t *testing.T, afterTouchURL string) *httptest.Server {
	t.Helper()

	var callCount int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/speaker" {
			http.NotFound(w, r)
			return
		}

		atomic.AddInt32(&callCount, 1)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}

		var play models.PlayInfo
		if err := xml.Unmarshal(body, &play); err != nil {
			http.Error(w, "xml parse error", http.StatusInternalServerError)
			return
		}

		// Simulate the speaker calling back /v1/auth on AfterTouch.
		authReq, err := http.NewRequest(http.MethodGet, afterTouchURL+"/v1/auth", nil)
		if err != nil {
			http.Error(w, "build auth req error", http.StatusInternalServerError)
			return
		}

		authReq.Header.Set("Apikeyheader", play.AppKey)
		_, _ = http.DefaultClient.Do(authReq) //nolint:bodyclose

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<status>OK</status>"))
	}))
}

// probeRouter builds a minimal router with the probe and auth endpoints.
func probeRouter(t *testing.T, server *Server) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/setup/health/dns-path-probe", server.HandleDNSPathProbe)
	mux.HandleFunc("/v1/auth", server.HandleSpeakerAuth)

	return mux
}

func TestHandleDNSPathProbe_EndToEnd_Success(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// We need the AfterTouch test server URL to give to the fake speaker.
	// Use a two-step approach: create the server first, then wire up the URL.
	server := NewServer(ds, nil, "http://placeholder", false, false, false)
	server.authProbes = newAuthProbeRegistry(5 * time.Second)
	server.authProbeTimeoutOverride = 3 * time.Second

	atServer := httptest.NewServer(probeRouter(t, server))
	defer atServer.Close()

	// Update server to know its own URL (needed for the probe URL construction).
	server.serverURL = atServer.URL

	// Fake speaker that calls /v1/auth on the AfterTouch test server.
	speaker := fakeSpeaker(t, atServer.URL)
	defer speaker.Close()

	// Extract the speaker's host:port.
	speakerHost := strings.TrimPrefix(speaker.URL, "http://")

	// Register the speaker as a known device so resolveTTSHost accepts it.
	if err := ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICEID01",
		AccountID: "1000001",
		IPAddress: speakerHost,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	body := `{"deviceId":"DEVICEID01"}`
	req := httptest.NewRequest(http.MethodPost, "/setup/health/dns-path-probe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	http.HandlerFunc(server.HandleDNSPathProbe).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp dnsProbeSpeakerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, rec.Body.String())
	}

	if !resp.Success {
		t.Errorf("success = false, want true; reason: %s", resp.Reason)
	}

	if resp.ObservedFrom == "" {
		t.Error("observedFrom is empty, expected a source IP")
	}
}

func TestHandleDNSPathProbe_Timeout_NoCallback(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// A fake speaker that accepts /speaker but never calls /v1/auth back.
	silentSpeaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/speaker" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<status>OK</status>"))
			return
		}

		http.NotFound(w, r)
	}))
	defer silentSpeaker.Close()

	speakerHost := strings.TrimPrefix(silentSpeaker.URL, "http://")

	server := NewServer(ds, nil, "http://localhost:8001", false, false, false)
	server.authProbes = newAuthProbeRegistry(5 * time.Second)
	server.authProbeTimeoutOverride = 300 * time.Millisecond // fast timeout for tests

	if err := ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICEID01",
		AccountID: "1000001",
		IPAddress: speakerHost,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	body := `{"deviceId":"DEVICEID01"}`
	req := httptest.NewRequest(http.MethodPost, "/setup/health/dns-path-probe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	http.HandlerFunc(server.HandleDNSPathProbe).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp dnsProbeSpeakerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, rec.Body.String())
	}

	if resp.Success {
		t.Error("success = true, want false (speaker never called back)")
	}

	if resp.Remediation == "" {
		t.Error("remediation is empty, expected a hint")
	}
}

func TestHandleDNSPathProbe_SpeakerUnreachable(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Find a loopback port with nothing listening so the TCP dial fails fast
	// (connection refused). We must register it as a known device first.
	closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedAddr := strings.TrimPrefix(closedServer.URL, "http://")
	closedServer.Close() // immediately close so the port becomes unreachable

	if err := ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICEID01",
		AccountID: "1000001",
		IPAddress: closedAddr,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	server := NewServer(ds, nil, "http://localhost:8001", false, false, false)
	server.authProbes = newAuthProbeRegistry(5 * time.Second)
	server.authProbeTimeoutOverride = 300 * time.Millisecond

	body := `{"deviceId":"DEVICEID01"}`
	req := httptest.NewRequest(http.MethodPost, "/setup/health/dns-path-probe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	http.HandlerFunc(server.HandleDNSPathProbe).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp dnsProbeSpeakerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, rec.Body.String())
	}

	if resp.Success {
		t.Error("success = true, want false (speaker unreachable)")
	}

	if !strings.Contains(resp.Reason, "speaker unreachable") {
		t.Errorf("reason = %q, want it to mention 'speaker unreachable'", resp.Reason)
	}
}

func TestHandleDNSPathProbe_InvalidBody(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	server := NewServer(ds, nil, "http://localhost:8001", false, false, false)

	req := httptest.NewRequest(http.MethodPost, "/setup/health/dns-path-probe", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()

	http.HandlerFunc(server.HandleDNSPathProbe).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleDNSPathProbe_UnknownDevice(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	server := NewServer(ds, nil, "http://localhost:8001", false, false, false)

	body := `{"deviceId":"NOPE"}`
	req := httptest.NewRequest(http.MethodPost, "/setup/health/dns-path-probe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	http.HandlerFunc(server.HandleDNSPathProbe).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
