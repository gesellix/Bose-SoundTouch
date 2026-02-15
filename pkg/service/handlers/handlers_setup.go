package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
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
	s.mergeOverlappingDevices()

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

// HandleRemoveDevice removes a device from the datastore.
func (s *Server) HandleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	deviceId := chi.URLParam(r, "deviceId")
	if deviceId == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	// Find which account this device belongs to.
	devices, err := s.ds.ListAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var found bool

	for i := range devices {
		if devices[i].DeviceID == deviceId {
			err = s.ds.RemoveDevice(devices[i].AccountID, devices[i].DeviceID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			found = true

			break
		}
	}

	if !found {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetSettings returns the current service settings.
func (s *Server) HandleGetSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	serverURL, proxyURL, httpsServerURL := s.serverURL, s.proxyURL, s.httpsServerURL
	discoveryInterval := s.discoveryInterval.String()
	discoveryEnabled := s.discoveryEnabled
	s.mu.RUnlock()

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"server_url":         serverURL,
		"proxy_url":          proxyURL,
		"https_server_url":   httpsServerURL,
		"discovery_interval": discoveryInterval,
		"discovery_enabled":  discoveryEnabled,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleUpdateSettings updates the service settings.
func (s *Server) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		ServerURL         string `json:"server_url"`
		ProxyURL          string `json:"proxy_url"`
		DiscoveryInterval string `json:"discovery_interval"`
		DiscoveryEnabled  bool   `json:"discovery_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	interval, err := time.ParseDuration(settings.DiscoveryInterval)
	if err != nil && settings.DiscoveryInterval != "" {
		http.Error(w, "Invalid discovery interval: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.serverURL = settings.ServerURL

	s.proxyURL = settings.ProxyURL
	if settings.DiscoveryInterval != "" {
		s.discoveryInterval = interval
	}

	s.discoveryEnabled = settings.DiscoveryEnabled

	if s.sm != nil {
		s.sm.ServerURL = settings.ServerURL
	}

	// Persist to datastore
	// Access fields directly since we already hold the lock
	currentRedact := s.proxyRedact
	currentLogBody := s.proxyLogBody
	currentRecord := s.recordEnabled
	currentHTTPS := s.httpsServerURL

	log.Printf("Saving updated settings to %s/settings.json", s.ds.DataDir)
	err = s.ds.SaveSettings(datastore.Settings{
		ServerURL:          s.serverURL,
		ProxyURL:           s.proxyURL,
		HTTPServerURL:      currentHTTPS,
		RedactLogs:         currentRedact,
		LogBodies:          currentLogBody,
		RecordInteractions: currentRecord,
		DiscoveryInterval:  s.discoveryInterval.String(),
		DiscoveryEnabled:   s.discoveryEnabled,
	})
	s.mu.Unlock()

	if err != nil {
		http.Error(w, "Failed to save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Settings updated"}); err != nil {
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

	output, err := s.sm.MigrateSpeaker(deviceIP, targetURL, proxyURL, options, method)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Migration started", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleRevertMigration reverts the migration for a device.
func (s *Server) HandleRevertMigration(w http.ResponseWriter, r *http.Request) {
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

	output, err := s.sm.RevertMigration(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Revert started", "output": output}); err != nil {
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

	output, err := s.sm.TrustCACert(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Root CA trusted", "output": output}); err != nil {
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

	output, err := s.sm.EnsureRemoteServices(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services enabled", "output": output}); err != nil {
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

	output, err := s.sm.RemoveRemoteServices(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services removed", "output": output}); err != nil {
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

	output, err := s.sm.BackupConfig(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Config backed up", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetProxySettings returns the current proxy settings.
func (s *Server) HandleGetProxySettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	redact, logBody, record := s.GetProxySettings()

	if err := json.NewEncoder(w).Encode(map[string]bool{
		"redact":   redact,
		"log_body": logBody,
		"record":   record,
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

	s.mu.Lock()
	s.proxyRedact = settings.Redact
	s.proxyLogBody = settings.LogBody
	s.recordEnabled = settings.Record

	// Persist to datastore
	// Access fields directly since we already hold the lock
	serverURL, proxyURL, httpsServerURL := s.serverURL, s.proxyURL, s.httpsServerURL
	discoveryInterval := s.discoveryInterval.String()
	discoveryEnabled := s.discoveryEnabled

	log.Printf("Saving updated proxy settings to %s/settings.json", s.ds.DataDir)
	err := s.ds.SaveSettings(datastore.Settings{
		ServerURL:          serverURL,
		ProxyURL:           proxyURL,
		HTTPServerURL:      httpsServerURL,
		RedactLogs:         s.proxyRedact,
		LogBodies:          s.proxyLogBody,
		RecordInteractions: s.recordEnabled,
		DiscoveryInterval:  discoveryInterval,
		DiscoveryEnabled:   discoveryEnabled,
	})
	s.mu.Unlock()

	if err != nil {
		http.Error(w, "Failed to save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

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

// HandleRebootDevice reboots a device.
func (s *Server) HandleRebootDevice(w http.ResponseWriter, r *http.Request) {
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

	output, err := s.sm.Reboot(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Reboot started", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
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

// HandleGetVersionInfo returns version information for the service.
func (s *Server) HandleGetVersionInfo(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{
		"version": s.Version,
		"commit":  s.Commit,
		"date":    s.Date,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetInteractionStats returns statistics about recorded interactions.
func (s *Server) HandleGetInteractionStats(w http.ResponseWriter, _ *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	stats, err := s.recorder.GetInteractionStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleListInteractions returns a list of recorded interactions.
func (s *Server) HandleListInteractions(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	session := r.URL.Query().Get("session")
	category := r.URL.Query().Get("category")
	since := r.URL.Query().Get("since")

	interactions, err := s.recorder.ListInteractions(session, category, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(interactions); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetInteractionContent returns the raw content of a recorded interaction.
func (s *Server) HandleGetInteractionContent(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	file := r.URL.Query().Get("file")
	if file == "" {
		http.Error(w, "File parameter is required", http.StatusBadRequest)
		return
	}

	content, err := s.recorder.GetInteractionContent(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(content)
}

// HandleDeleteSession deletes a recorded interaction session.
func (s *Server) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	session := chi.URLParam(r, "session")
	if session == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	if err := s.recorder.DeleteSession(session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok": true}`))
}

// HandleCleanupSessions deletes all but the most recent N sessions.
func (s *Server) HandleCleanupSessions(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	keep := 10

	keepStr := r.URL.Query().Get("keep")
	if keepStr != "" {
		if k, err := strconv.Atoi(keepStr); err == nil {
			keep = k
		}
	}

	if err := s.recorder.CleanupSessions(keep); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok": true}`))
}
