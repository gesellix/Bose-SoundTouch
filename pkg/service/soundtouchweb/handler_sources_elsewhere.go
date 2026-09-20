package soundtouchweb

import (
	"encoding/json"
	"net/http"

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
