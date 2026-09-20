package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

// SourcesElsewherePayload answers "what could this speaker be given that the
// others already have?" (issue 754).
//
// Identity only: the stored source list carries credential material, and this
// API is unauthenticated (issue 663). See models.SourceIdentity.
type SourcesElsewherePayload struct {
	// Available is false where no service backs the player, so "we cannot see
	// the other speakers" stays tellable apart from "there is nothing".
	Available bool                    `json:"available"`
	Sources   []models.SourceIdentity `json:"sources"`
}

// HandleSourcesElsewhere lists the sources other speakers have and this one
// does not.
func (app *WebApp) HandleSourcesElsewhere(w http.ResponseWriter, r *http.Request) {
	device, exists := app.GetDevice(chi.URLParam(r, "id"))
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	payload := SourcesElsewherePayload{Sources: []models.SourceIdentity{}}

	if app.SourcesElsewhere != nil {
		id, account := storedPresetTarget(device, chi.URLParam(r, "id"))

		found, err := app.SourcesElsewhere(id, account)
		if err != nil {
			app.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload.Available = true

		if found != nil {
			payload.Sources = found
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: payload}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleAddSourceElsewhere gives this speaker a source one of the others has
// (issue 754).
//
// Only the addable tier gets this far, and the two kinds inside it are added
// in different places, which is not an implementation detail: a media server
// is registered on the speaker itself, because that is where a DLNA account
// lives, while TuneIn and its siblings are entries in the list AfterTouch
// serves. Neither carries another speaker's credential across.
//
// A music service is refused with the reason, since no amount of service-side
// work can link an account the target speaker has to link itself.
func (app *WebApp) HandleAddSourceElsewhere(w http.ResponseWriter, r *http.Request) {
	device, exists := app.GetDevice(chi.URLParam(r, "id"))
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		Type    string `json:"type"`
		Account string `json:"account"`
		Name    string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	sourceType := strings.ToUpper(strings.TrimSpace(req.Type))
	if sourceType == "" {
		app.sendError(w, "type is required", http.StatusBadRequest)
		return
	}

	switch models.SourceAvailability(sourceType) {
	case models.SourceAvailableByLinking:
		app.sendError(w, sourceDisplayName(sourceType)+
			" has to be linked on this speaker itself; its credential belongs to the speaker that acquired it and cannot be copied across.",
			http.StatusConflict)

		return
	case models.SourceAvailableLocalOnly:
		app.sendError(w, sourceDisplayName(sourceType)+" is a device-local source; there is nothing to add.", http.StatusConflict)

		return
	}

	if sourceType == "STORED_MUSIC" || sourceType == "LOCAL_MUSIC" {
		app.addMediaServerElsewhere(w, device, req.Account, req.Name)

		return
	}

	app.addCanonicalSourceElsewhere(w, device, chi.URLParam(r, "id"), sourceType)
}

func (app *WebApp) addMediaServerElsewhere(w http.ResponseWriter, device *webtypes.DeviceConnection, account, name string) {
	if account == "" {
		app.sendError(w, "account is required for a media server", http.StatusBadRequest)
		return
	}

	refreshed, err := app.registerStoredMusicAccount(device, account, name)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	app.sendAddedSource(w, true, refreshed)
}

func (app *WebApp) addCanonicalSourceElsewhere(w http.ResponseWriter, device *webtypes.DeviceConnection, fallbackID, sourceType string) {
	if app.AddCanonicalSource == nil {
		app.sendError(w, "This player has no AfterTouch service behind it, so it cannot add that source.", http.StatusNotImplemented)
		return
	}

	id, account := storedPresetTarget(device, fallbackID)

	added, err := app.AddCanonicalSource(id, account, sourceType)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusConflict)
		return
	}

	// The speaker fetches its sources from the service, so the write is the
	// change; the nudge only decides whether it shows up now or at the next
	// fetch, and must never fail the request.
	refreshed := false

	if info := device.Info(); info != nil && info.DeviceID != "" {
		refreshed = device.Client.NotifySourcesUpdated(info.DeviceID) == nil
	}

	app.sendAddedSource(w, added, refreshed)
}

func (app *WebApp) sendAddedSource(w http.ResponseWriter, added, refreshed bool) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]any{"added": added, "refreshed": refreshed},
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
