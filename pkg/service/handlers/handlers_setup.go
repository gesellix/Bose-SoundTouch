package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
)

// HandleListDiscoveredDevices returns a list of all discovered devices.
func (s *Server) HandleListDiscoveredDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.ds.ListAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(devices); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleAddManualDevice adds a device manually by IP.
func (s *Server) HandleAddManualDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if body.IP == "" {
		http.Error(w, "IP address is required", http.StatusBadRequest)
		return
	}

	// Try to get live info
	liveInfo, err := s.sm.GetLiveDeviceInfo(body.IP)
	if err != nil {
		// Even if we can't get live info, we might want to add it?
		// But usually we need at least the serial for proper account management.
		http.Error(w, "Failed to reach device at "+body.IP+": "+err.Error(), http.StatusBadGateway)
		return
	}

	// Reuse handleDiscoveredDevice logic via a fake models.DiscoveredDevice
	d := models.DiscoveredDevice{
		Name:            liveInfo.Name,
		Host:            body.IP,
		ModelID:         liveInfo.Type,
		SerialNo:        liveInfo.SerialNumber,
		DiscoveryMethod: "manual",
	}

	s.handleDiscoveredDevice(d)

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTriggerDiscovery triggers a new device discovery scan.
func (s *Server) HandleTriggerDiscovery(w http.ResponseWriter, _ *http.Request) {
	//nolint:contextcheck
	go s.DiscoverDevices(context.Background())

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status": "Discovery started"}`))
}

// HandleGetDiscoveryStatus returns the current discovery status.
func (s *Server) HandleGetDiscoveryStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"discovering": s.discovering}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetSettings returns the current service settings.
func (s *Server) HandleGetSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"server_url": s.serverURL,
		"proxy_url":  s.proxyURL,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetDeviceInfo returns live information for a device.
func (s *Server) HandleGetDeviceInfo(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		http.Error(w, "Device IP is required", http.StatusBadRequest)
		return
	}

	info, err := s.sm.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(info); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetMigrationSummary returns a summary of the migration plan for a device.
func (s *Server) HandleGetMigrationSummary(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		http.Error(w, "Device IP is required", http.StatusBadRequest)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	proxyURL := r.URL.Query().Get("proxy_url")

	options := make(map[string]string)

	for k, v := range r.URL.Query() {
		if len(v) > 0 && (k == "marge" || k == "stats" || k == "sw_update" || k == "bmx") {
			options[k] = v[0]
		}
	}

	summary, err := s.sm.GetMigrationSummary(deviceIP, targetURL, proxyURL, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(summary); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleMigrateDevice starts the migration process for a device.
func (s *Server) HandleMigrateDevice(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	targetURL := r.URL.Query().Get("target_url")
	proxyURL := r.URL.Query().Get("proxy_url")
	method := setup.MigrationMethod(r.URL.Query().Get("method"))

	options := make(map[string]string)

	for k, v := range r.URL.Query() {
		if len(v) > 0 && (k == "marge" || k == "stats" || k == "sw_update" || k == "bmx") {
			options[k] = v[0]
		}
	}

	if err := s.sm.MigrateSpeaker(deviceIP, targetURL, proxyURL, options, method); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Migration started"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTrustCACert injects the local Root CA into the device's shared trust store.
func (s *Server) HandleTrustCACert(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	if err := s.sm.TrustCACert(deviceIP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Root CA trusted"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleEnsureRemoteServices ensures that remote services are configured on a device.
func (s *Server) HandleEnsureRemoteServices(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	if err := s.sm.EnsureRemoteServices(deviceIP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services ensured"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleRemoveRemoteServices removes remote services configuration from a device.
func (s *Server) HandleRemoveRemoteServices(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	if err := s.sm.RemoveRemoteServices(deviceIP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services removed"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleBackupConfig creates a backup of the device configuration.
func (s *Server) HandleBackupConfig(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	if err := s.sm.BackupConfig(deviceIP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Backup created"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetProxySettings returns the current proxy settings.
func (s *Server) HandleGetProxySettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{
		"redact":   s.proxyRedact,
		"log_body": s.proxyLogBody,
		"record":   s.recordEnabled,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetCACert returns the Root CA certificate.
func (s *Server) HandleGetCACert(w http.ResponseWriter, _ *http.Request) {
	caCertPath := s.sm.Crypto.GetCACertPath()

	content, err := os.ReadFile(caCertPath)
	if err != nil {
		http.Error(w, "Failed to read CA certificate", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", "attachment; filename=soundtouch-ca.crt")
	_, _ = w.Write(content)
}

// HandleUpdateProxySettings updates the proxy settings.
func (s *Server) HandleUpdateProxySettings(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		Redact  bool `json:"redact"`
		LogBody bool `json:"log_body"`
		Record  bool `json:"record"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.proxyRedact = settings.Redact
	s.proxyLogBody = settings.LogBody
	s.recordEnabled = settings.Record

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Proxy settings updated"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTestHostsRedirection performs a preliminary check for /etc/hosts redirection.
func (s *Server) HandleTestHostsRedirection(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		http.Error(w, "Device IP is required", http.StatusBadRequest)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	if targetURL == "" {
		targetURL = s.serverURL
	}

	output, err := s.sm.TestHostsRedirection(deviceIP, targetURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Return 200 but ok: false so UI can show the output

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"message": err.Error(),
			"output":  output,
		}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Hosts redirection test successful",
		"output":  output,
	}); encodeErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleInitialSync fetches presets, recents and sources from the device and saves them to the datastore.
func (s *Server) HandleInitialSync(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		http.Error(w, "Missing deviceIP", http.StatusBadRequest)
		return
	}

	if err := s.sm.SyncDeviceData(deviceIP); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok": true}`))
}

// HandleTestConnection performs a connection check from the device to the server.
func (s *Server) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		http.Error(w, "Device IP is required", http.StatusBadRequest)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	if targetURL == "" {
		http.Error(w, "Target URL is required", http.StatusBadRequest)
		return
	}

	useExplicitCA := r.URL.Query().Get("use_explicit_ca") == "true"

	output, err := s.sm.TestConnection(deviceIP, targetURL, useExplicitCA)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Return 200 but ok: false so UI can show the output

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"message": err.Error(),
			"output":  output,
		}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Connection test successful",
		"output":  output,
	}); encodeErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
