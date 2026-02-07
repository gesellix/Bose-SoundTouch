package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) HandleListDiscoveredDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.ds.ListAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
}

func (s *Server) HandleTriggerDiscovery(w http.ResponseWriter, r *http.Request) {
	go s.DiscoverDevices(r.Context())

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status": "Discovery started"}`))
}

func (s *Server) HandleGetDiscoveryStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"discovering": s.discovering})
}

func (s *Server) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"server_url": s.serverURL,
		"proxy_url":  s.proxyURL,
	})
}

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
	_ = json.NewEncoder(w).Encode(info)
}

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
	_ = json.NewEncoder(w).Encode(summary)
}

func (s *Server) HandleMigrateDevice(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"})

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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Migration started"})
}

func (s *Server) HandleEnsureRemoteServices(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"})

		return
	}

	if err := s.sm.EnsureRemoteServices(deviceIP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services ensured"})
}

func (s *Server) HandleBackupConfig(w http.ResponseWriter, r *http.Request) {
	deviceIP := chi.URLParam(r, "deviceIP")
	if deviceIP == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device IP is required"})

		return
	}

	if err := s.sm.BackupConfig(deviceIP); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()})

		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Backup created"})
}

func (s *Server) HandleGetProxySettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{
		"redact":   s.proxyRedact,
		"log_body": s.proxyLogBody,
	})
}

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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Proxy settings updated"})
}
