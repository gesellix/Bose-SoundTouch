package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/service/tts"
	"github.com/go-chi/chi/v5"
)

// ttsSpeakRequest is the JSON body for POST /mgmt/tts/speak. Either DeviceID
// (resolved to an IP via the datastore) or Host (an explicit IP/hostname) must
// be set. The remaining fields fall back to the service defaults when empty.
type ttsSpeakRequest struct {
	DeviceID string `json:"deviceId,omitempty"`
	Host     string `json:"host,omitempty"`
	Text     string `json:"text"`
	Language string `json:"language,omitempty"`
	Voice    string `json:"voice,omitempty"`
	Format   string `json:"format,omitempty"`
	Volume   *int   `json:"volume,omitempty"`
}

// HandleMgmtTTSSpeak synthesizes the requested text (or builds a direct URL),
// then tells the target speaker to play it via the /speaker endpoint.
func (s *Server) HandleMgmtTTSSpeak(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	svc := s.ttsSvc()
	if svc == nil {
		http.Error(w, `{"error":"tts not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req ttsSpeakRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Text) == "" {
		http.Error(w, `{"error":"text is required"}`, http.StatusBadRequest)
		return
	}

	host, err := s.resolveTTSHost(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	playURL, err := svc.Prepare(r.Context(), tts.Request{
		Text:     req.Text,
		Language: req.Language,
		Voice:    req.Voice,
		Format:   req.Format,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "synthesize: "+err.Error()), http.StatusBadGateway)
		return
	}

	volume := svc.DefaultVolume()
	if req.Volume != nil {
		volume = *req.Volume
	}

	c := client.NewClientFromHost(host)

	var playErr error
	if volume > 0 {
		playErr = c.PlayURL(playURL, svc.AppKey(), "AfterTouch TTS", req.Text, "", volume)
	} else {
		playErr = c.PlayURL(playURL, svc.AppKey(), "AfterTouch TTS", req.Text, "")
	}

	if playErr != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, "play: "+playErr.Error()), http.StatusBadGateway)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"host":   host,
		"url":    playURL,
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// resolveTTSHost returns the speaker IP/hostname to target. An explicit Host
// wins; otherwise DeviceID is looked up in the datastore.
func (s *Server) resolveTTSHost(req ttsSpeakRequest) (string, error) {
	if h := strings.TrimSpace(req.Host); h != "" {
		return h, nil
	}

	if strings.TrimSpace(req.DeviceID) == "" {
		return "", fmt.Errorf("either deviceId or host is required")
	}

	devices, err := s.ds.ListAllDevices()
	if err != nil {
		return "", fmt.Errorf("list devices: %w", err)
	}

	for i := range devices {
		if devices[i].DeviceID == req.DeviceID {
			if devices[i].IPAddress == "" {
				return "", fmt.Errorf("device %s has no known IP address", req.DeviceID)
			}

			return devices[i].IPAddress, nil
		}
	}

	return "", fmt.Errorf("device %s not found", req.DeviceID)
}

// HandleTTSMedia serves a synthesized clip by id for the speaker to fetch.
func (s *Server) HandleTTSMedia(w http.ResponseWriter, r *http.Request) {
	svc := s.ttsSvc()
	if svc == nil {
		http.NotFound(w, r)
		return
	}

	id := chi.URLParam(r, "id")

	audio, contentType, ok := svc.Clip(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(audio)
}

// HandleMgmtTTSConfig reports the active TTS configuration (no secrets).
func (s *Server) HandleMgmtTTSConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	svc := s.ttsSvc()

	resp := map[string]interface{}{"configured": svc != nil}
	if svc != nil {
		resp["provider"] = svc.ProviderName()
		resp["defaultLanguage"] = svc.DefaultLanguage()
		resp["defaultVoice"] = svc.DefaultVoice()
		resp["defaultVolume"] = svc.DefaultVolume()
		resp["appKeyConfigured"] = svc.AppKey() != ""
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
