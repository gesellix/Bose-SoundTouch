package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
	//   "speaker" (default) — POST /speaker notification; ducks and resumes
	//                          the current playback, supports volume. Requires
	//                          the speaker to accept the app_key (validated via
	//                          GET /v1/auth, which we answer 200).
	//   "radio"             — LOCAL_INTERNET_RADIO via /custom/v1/playback,
	//                          no app_key; replaces the current source.
	Method string `json:"method,omitempty"`
}

// HandleTTSSpeak synthesizes the requested text and plays it on the target
// speaker. Two playback methods (see ttsSpeakRequest.Method): the default
// /speaker notification path (ducks/resumes and supports volume) or the
// LOCAL_INTERNET_RADIO path (like the "ding", no app_key, replaces the source).
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
	case "radio":
		location := buildCustomPlaybackURL(svc.BaseURL(), playURL, "AfterTouch TTS: "+req.Text)
		if err := c.SelectLocalInternetRadio(location, "", "AfterTouch TTS", ""); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, "play (radio): "+err.Error()), http.StatusBadGateway)
			return
		}

		resp["method"] = "radio"
		resp["location"] = location
	default: // "speaker" or unset — ducks and resumes the current playback
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

// HandleSpeakerAuth accepts the app_key a speaker presents when validating a
// /speaker notification. The speaker issues GET /v1/auth to the (now-dead) Bose
// host audionotification.api.bosecm.com — which AfterTouch's DNS interception
// points at us — with the key in an "Apikeyheader" header. An empty 200 is
// sufficient; real Bose validated against its cloud, but as the cloud
// replacement we always accept, so the speaker doesn't report an invalid app
// key (HandleInvalidAppKeyCb) and refuse the notification.
//
// When an active DNS-path probe is running (HandleDNSPathProbe), the probe
// registers its nonce as the app_key. If the nonce matches, this handler
// returns 403 so the speaker treats the key as invalid and refuses the
// notification (no audio plays). The callback arrival itself already proved
// the speaker resolved a Bose host through AfterTouch's DNS.
func (s *Server) HandleSpeakerAuth(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Apikeyheader")
	if token != "" && s.authProbes != nil {
		if s.authProbes.observe(token, clientHostFromRemoteAddr(r.RemoteAddr)) {
			// Active DNS-path probe: the callback arrival already proved the
			// speaker resolved a Bose host through AfterTouch. Returning 403
			// makes the speaker treat the key as invalid and refuse the
			// notification, so no audio plays.
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	// Normal path: accept any app_key so real TTS/URL notifications proceed.
	w.WriteHeader(http.StatusOK)
}

// resolveTTSHost returns the speaker IP to target, resolved from the datastore
// so the result is always a known device address — never a value taken straight
// from the request. This prevents the endpoint from being used as an SSRF proxy
// to arbitrary hosts (the resolved IP flows into client.NewClientFromHost ->
// baseURL -> the outbound request). Match by DeviceID, or by Host equal to a
// known device's IP; either way the returned string is the datastore's
// IPAddress, not the caller-supplied value.
//
// The incoming Host is normalized to a bare host (scheme and port stripped)
// before the match so callers that pass a base URL (e.g. "http://ip:8090",
// which is what a client's Host() returns) still resolve. This only makes the
// needle comparable to the datastore's bare IPs; the result is still always a
// datastore IP, so the SSRF guarantee is unchanged.
func (s *Server) resolveTTSHost(req ttsSpeakRequest) (string, error) {
	deviceID := strings.TrimSpace(req.DeviceID)
	host := ttsHostOnly(strings.TrimSpace(req.Host))

	if deviceID == "" && host == "" {
		return "", fmt.Errorf("either deviceId or host is required")
	}

	devices, err := s.ds.ListAllDevices()
	if err != nil {
		return "", fmt.Errorf("list devices: %w", err)
	}

	for i := range devices {
		ip := devices[i].IPAddress
		if ip == "" {
			continue
		}

		if (deviceID != "" && devices[i].DeviceID == deviceID) || (host != "" && ip == host) {
			return ip, nil
		}
	}

	if deviceID != "" {
		return "", fmt.Errorf("device %s not found (or has no known IP)", deviceID)
	}

	return "", fmt.Errorf("host %s is not a known device", host)
}

// ttsHostOnly reduces a base URL or host:port to a bare host so it can be
// compared against the datastore's bare IPs. Inputs that are already bare are
// returned unchanged. The empty string maps to the empty string.
func ttsHostOnly(raw string) string {
	if raw == "" {
		return ""
	}

	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Hostname()
	}

	if host, _, err := net.SplitHostPort(raw); err == nil {
		return host
	}

	return raw
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
