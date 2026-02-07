package handlers

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

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
