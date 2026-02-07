package handlers

import (
	"encoding/json"
	"net/http"

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

// HandleTriggerDiscovery triggers a new device discovery scan.
func (s *Server) HandleTriggerDiscovery(w http.ResponseWriter, r *http.Request) {
	go s.DiscoverDevices(r.Context())

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

	options := make(map[string]string)

	for k, v := range r.URL.Query() {
		if len(v) > 0 && (k == "marge" || k == "stats" || k == "sw_update" || k == "bmx") {
			options[k] = v[0]
		}
	}

	if err := s.sm.MigrateSpeaker(deviceIP, targetURL, proxyURL, options); err != nil {
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
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleUpdateProxySettings updates the proxy settings.
func (s *Server) HandleUpdateProxySettings(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		Redact  bool `json:"redact"`
		LogBody bool `json:"log_body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.proxyRedact = settings.Redact
	s.proxyLogBody = settings.LogBody

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Proxy settings updated"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
