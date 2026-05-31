package soundtouchweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// HandleAPISpeakText synthesizes and plays text on a device. The Web UI talks
// to speakers directly for most controls, but TTS synthesis (Google Cloud) and
// the Bose app_key live in the AfterTouch service, so this proxies to the
// service's /mgmt/tts/speak endpoint, targeting the device by its IP/host.
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

	if app.ServiceURL == "" {
		app.sendError(w,
			"TTS requires the AfterTouch service. Start soundtouch-web with --service-url <https://your-aftertouch-host>.",
			http.StatusBadRequest)

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

	payload := map[string]interface{}{
		"host": device.Client.Host(),
		"text": req.Text,
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

	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, app.ServiceURL+"/mgmt/tts/speak", bytes.NewReader(body))
	if err != nil {
		app.sendError(w, "Failed to build TTS request", http.StatusInternalServerError)
		return
	}

	upstream.Header.Set("Content-Type", "application/json")

	if app.MgmtUsername != "" || app.MgmtPassword != "" {
		upstream.SetBasicAuth(app.MgmtUsername, app.MgmtPassword)
	}

	resp, err := http.DefaultClient.Do(upstream)
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
