package soundtouchweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

// hostOnly reduces a base URL or host:port to a bare host (IP or hostname).
// device.Client.Host() returns a full base URL like "http://192.168.0.2:8090",
// but the AfterTouch service matches the TTS target against bare datastore IPs,
// so we strip the scheme and port before sending it. Inputs that are already
// bare ("192.168.0.2") are returned unchanged.
func hostOnly(raw string) string {
	raw = strings.TrimSpace(raw)
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

// HandleAPISpeakText synthesizes and plays text on a device. The Web UI talks
// to speakers directly for most controls, but TTS synthesis (Google Cloud) and
// the Bose app_key live in the AfterTouch service, so this proxies to the
// service's /api/setup/tts/speak endpoint, targeting the device by its IP/host.
func (app *WebApp) HandleAPISpeakText(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		Text     string `json:"text"`
		Language string `json:"language,omitempty"`
		Voice    string `json:"voice,omitempty"`
		Volume   *int   `json:"volume,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Text) == "" {
		app.sendError(w, "text is required", http.StatusBadRequest)
		return
	}

	// The TTS request is made server-side by soundtouch-web, so its target must
	// be the operator-configured service URL — never a client-supplied value
	// (that would let any LAN caller use this endpoint as an SSRF proxy). This
	// differs from Play URL, where the URL is handed to the speaker, not fetched
	// by soundtouch-web.
	serviceURL := strings.TrimRight(app.ServiceURL, "/")
	if serviceURL == "" {
		app.sendError(w,
			"TTS requires the AfterTouch service URL. Start soundtouch-web with --service-url <https://your-aftertouch-host>.",
			http.StatusBadRequest)

		return
	}

	// Identify the target speaker for the service. Prefer the DeviceID (the
	// canonical, unambiguous key the service matches in its datastore). Also
	// send a bare-IP host as a fallback: device.Client.Host() is a full base
	// URL (http://ip:8090), which the service's exact-match SSRF guard
	// (resolveTTSHost) would reject, so strip it down to host-only.
	payload := map[string]interface{}{
		"text": req.Text,
	}
	if device.DeviceInfo != nil && device.DeviceInfo.DeviceID != "" {
		payload["deviceId"] = device.DeviceInfo.DeviceID
	}

	if h := hostOnly(device.Client.Host()); h != "" {
		payload["host"] = h
	}

	if req.Language != "" {
		payload["language"] = req.Language
	}

	if req.Voice != "" {
		payload["voice"] = req.Voice
	}

	if req.Volume != nil {
		payload["volume"] = *req.Volume
	}

	body, err := json.Marshal(payload)
	if err != nil {
		app.sendError(w, "Failed to build TTS request", http.StatusInternalServerError)
		return
	}

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, serviceURL+"/api/setup/tts/speak", bytes.NewReader(body))
	if err != nil {
		app.sendError(w, "Failed to build TTS request", http.StatusInternalServerError)
		return
	}

	upstream.Header.Set("Content-Type", "application/json")

	resp, err := app.serviceHTTPClient().Do(upstream)
	if err != nil {
		app.sendError(w, fmt.Sprintf("TTS service request failed: %v", err), http.StatusBadGateway)
		return
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))

	if resp.StatusCode != http.StatusOK {
		app.sendError(w, fmt.Sprintf("TTS service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))), http.StatusBadGateway)
		return
	}

	app.sendControlResponse(w, nil, fmt.Sprintf("Speaking: %q", req.Text))
}
