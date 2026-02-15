package handlers

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/go-chi/chi/v5"
)

// HandleUsageStats handles Marge usage stats uploads.
func (s *Server) HandleUsageStats(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var stats models.UsageStats
	// Try XML first (Bose devices often use XML)
	if err := xml.Unmarshal(body, &stats); err != nil {
		// Fallback to JSON
		if err := json.Unmarshal(body, &stats); err != nil {
			http.Error(w, "Invalid stats format", http.StatusBadRequest)
			return
		}
	}

	if err := s.ds.SaveUsageStats(stats); err != nil {
		http.Error(w, "Failed to save usage stats", http.StatusInternalServerError)
		return
	}

	// Create a DeviceEvent from the usage stats
	event := models.DeviceEvent{
		Type:     stats.EventType,
		Time:     stats.Timestamp,
		MonoTime: time.Now().UnixNano() / int64(time.Millisecond),
		Data:     stats.Parameters,
	}
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339)
	}

	s.ds.AddDeviceEvent(stats.DeviceID, event)

	w.WriteHeader(http.StatusOK)
}

// HandleAppEvents handles events from the Bose SoundTouch app (stapp/scmudc).
func (s *Server) HandleAppEvents(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req models.DeviceEventsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid app events format", http.StatusBadRequest)
		return
	}

	deviceID := req.Envelope.UniqueID
	if deviceID == "" {
		deviceID = chi.URLParam(r, "deviceId")
	}

	for _, e := range req.Payload.Events {
		event := models.DeviceEvent{
			Type:     e.Type,
			Time:     e.Time,
			MonoTime: req.Envelope.MonoTime,
			Data:     e.Data,
		}
		if event.Time == "" {
			event.Time = time.Now().Format(time.RFC3339)
		}

		s.ds.AddDeviceEvent(deviceID, event)
	}

	w.WriteHeader(http.StatusOK)
}

// HandleErrorStats handles Marge error stats uploads.
func (s *Server) HandleErrorStats(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var stats models.ErrorStats
	if err := xml.Unmarshal(body, &stats); err != nil {
		if err := json.Unmarshal(body, &stats); err != nil {
			http.Error(w, "Invalid error stats format", http.StatusBadRequest)
			return
		}
	}

	if err := s.ds.SaveErrorStats(stats); err != nil {
		http.Error(w, "Failed to save error stats", http.StatusInternalServerError)
		return
	}

	// Create a DeviceEvent from the error stats
	event := models.DeviceEvent{
		Type:     "device-error",
		Time:     stats.Timestamp,
		MonoTime: time.Now().UnixNano() / int64(time.Millisecond),
		Data: map[string]interface{}{
			"errorCode":    stats.ErrorCode,
			"errorMessage": stats.ErrorMessage,
			"details":      stats.Details,
		},
	}
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339)
	}

	s.ds.AddDeviceEvent(stats.DeviceID, event)

	w.WriteHeader(http.StatusOK)
}
