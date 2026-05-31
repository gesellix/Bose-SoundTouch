package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/service/tts"
	"github.com/go-chi/chi/v5"
)

// ttsSpeakRequest is the JSON body for POST /setup/tts/speak. Either DeviceID
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
	// Method selects how the clip is played on the speaker:
	//   "radio"   (default) — LOCAL_INTERNET_RADIO via /custom/v1/playback,
	//                          no app_key; replaces the current source.
	//   "speaker"           — POST /speaker notification; ducks and resumes
	//                          the current playback, supports volume, but
	//                          requires the speaker to accept the app_key
	//                          (validated via GET /v1/auth, which we answer 200).
	Method string `json:"method,omitempty"`
}

// HandleTTSSpeak synthesizes the requested text and plays it on the target
// speaker. Two playback methods (see ttsSpeakRequest.Method): the default
// LOCAL_INTERNET_RADIO path (like the "ding", no app_key) or the /speaker
// notification path (ducks/resumes and supports volume).
func (s *Server) HandleTTSSpeak(w http.ResponseWriter, r *http.Request) {
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

	c := client.NewClientFromHost(host)
	method := strings.ToLower(strings.TrimSpace(req.Method))

	resp := map[string]interface{}{"status": "ok", "host": host, "url": playURL}

	switch method {
	case "speaker":
		appKey := svc.AppKey()
		if appKey == "" {
			// The speaker validates the app_key via GET /v1/auth, which we
			// answer 200 regardless, so any non-empty value works.
			appKey = "aftertouch"
		}

		volume := svc.DefaultVolume()
		if req.Volume != nil {
			volume = *req.Volume
		}

		var playErr error
		if volume > 0 {
			playErr = c.PlayURL(playURL, appKey, "AfterTouch TTS", req.Text, "", volume)
		} else {
			playErr = c.PlayURL(playURL, appKey, "AfterTouch TTS", req.Text, "")
		}

		if playErr != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "play (speaker): "+playErr.Error()), http.StatusBadGateway)
			return
		}

		resp["method"] = "speaker"
	default: // "radio" or unset
		location := buildCustomPlaybackURL(svc.BaseURL(), playURL, "AfterTouch TTS: "+req.Text)
		if err := c.SelectLocalInternetRadio(location, "", "AfterTouch TTS", ""); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "play (radio): "+err.Error()), http.StatusBadGateway)
			return
		}

		resp["method"] = "radio"
		resp["location"] = location
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// buildCustomPlaybackURL wraps a target audio URL in the AfterTouch
// /custom/v1/playback proxy so the speaker plays it via LOCAL_INTERNET_RADIO
// (same mechanism as the "ding"). base is this service's public URL.
func buildCustomPlaybackURL(base, audioURL, name string) string {
	base = strings.TrimRight(base, "/")
	encoded := base64.URLEncoding.EncodeToString([]byte(audioURL))

	return base + "/custom/v1/playback/" + encoded + "?name=" + url.QueryEscape(name)
}

// HandleSpeakerAuth accepts the app_key the speaker presents when validating a
// /speaker notification. Real Bose validated against its cloud; as the cloud
// replacement we always accept (200) so the speaker doesn't report an invalid
// app key and refuse the notification.
func (s *Server) HandleSpeakerAuth(w http.ResponseWriter, r *http.Request) {
	// TEMP DEBUG: dump the full request so we can see how the speaker presents
	// the app_key (query param / header / body) and what a valid response might
	// need to look like. Remove once the /speaker auth contract is understood.
	if dump, err := httputil.DumpRequest(r, true); err == nil {
		log.Printf("[TTS][/v1/auth DEBUG] %s", dump)
	} else {
		log.Printf("[TTS][/v1/auth DEBUG] dump failed: %v; method=%s url=%s headers=%v", err, r.Method, r.URL.String(), r.Header)
	}

	w.WriteHeader(http.StatusOK)
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

// HandleTTSConfig reports the active TTS configuration (no secrets).
func (s *Server) HandleTTSConfig(w http.ResponseWriter, _ *http.Request) {
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
