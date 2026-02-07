package handlers

import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
	"github.com/go-chi/chi/v5"
)

func (s *Server) HandleMargeSourceProviders(w http.ResponseWriter, r *http.Request) {
	etag := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.SourceProvidersToXML()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

func (s *Server) HandleMargeAccountFull(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	etag := strconv.FormatInt(s.ds.GetETagForAccount(account), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.AccountFullToXML(s.ds, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

func (s *Server) HandleMargePowerOn(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleMargeSoftwareUpdate(w http.ResponseWriter, r *http.Request) {
	etag := "default-embedded"
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header()["ETag"] = []string{etag}

	if len(swUpdateXML) > 0 {
		_, _ = w.Write(swUpdateXML)
	} else {
		_, _ = w.Write([]byte(marge.SoftwareUpdateToXML()))
	}
}

func (s *Server) HandleMargePresets(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	etag := strconv.FormatInt(s.ds.GetETagForPresets(account), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.PresetsToXML(s.ds, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

func (s *Server) HandleMargeUpdatePreset(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	device := chi.URLParam(r, "device")

	etag := strconv.FormatInt(s.ds.GetETagForPresets(account), 10)
	w.Header()["ETag"] = []string{etag}

	presetNumberStr := chi.URLParam(r, "presetNumber")

	presetNumber, err := strconv.Atoi(presetNumberStr)
	if err != nil {
		http.Error(w, "Invalid preset number", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	data, err := marge.UpdatePreset(s.ds, account, device, presetNumber, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(data)
}

func (s *Server) HandleMargeAddRecent(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	device := chi.URLParam(r, "device")

	etag := strconv.FormatInt(s.ds.GetETagForRecents(account), 10)
	w.Header()["ETag"] = []string{etag}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	data, err := marge.AddRecent(s.ds, account, device, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(data)
}

func (s *Server) HandleMargeAddDevice(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	data, err := marge.AddDeviceToAccount(s.ds, account, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(data)
}

func (s *Server) HandleMargeRemoveDevice(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := chi.URLParam(r, "device")
	if err := marge.RemoveDeviceFromAccount(s.ds, account, device); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok": true}`))
}

func (s *Server) HandleMargeProviderSettings(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(marge.ProviderSettingsToXML(account)))
}

func (s *Server) HandleMargeStreamingToken(w http.ResponseWriter, r *http.Request) {
	// Simple mock token for offline use.
	// In a real production environment, this would be a JWT or similar signed token.
	// Some speakers might expect a specific format; soundcork uses a distinctive prefix
	// to indicate it's a locally generated token.
	token := "soundcork-local-token-" + strconv.FormatInt(time.Now().Unix(), 10)
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) HandleMargeCustomerSupport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req models.CustomerSupportRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		// Log error but might still return 200 as Bose expects
		log.Printf("Failed to unmarshal CustomerSupportRequest: %v", err)
	}

	// Create a DeviceEvent for support data
	event := models.DeviceEvent{
		Type:     "customer-support-upload",
		Time:     time.Now().Format(time.RFC3339),
		MonoTime: time.Now().UnixNano() / int64(time.Millisecond),
		Data: map[string]interface{}{
			"firmware": req.Device.FirmwareVersion,
			"product":  req.Device.Product.ProductCode,
			"ip":       req.DiagnosticData.DeviceLandscape.IPAddress,
			"rssi":     req.DiagnosticData.DeviceLandscape.RSSI,
		},
	}
	s.ds.AddDeviceEvent(req.Device.ID, event)

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.WriteHeader(http.StatusOK)
}
