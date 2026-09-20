package soundtouchweb

import (
	"encoding/json"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

// StoredPresetsPayload is what the player receives for GET
// devices/{id}/stored-presets.
//
// It answers one question: does what AfterTouch stores for this speaker agree
// with what the speaker reports? A speaker has six buttons and reports at most
// six presets, so a stored list that is longer holds rows the speaker can
// never recall -- and those rows are what makes a sync look destructive and
// keeps the old list being served back (issue 697).
type StoredPresetsPayload struct {
	// Available is false where there is no service behind the player, so
	// "nothing to compare" stays tellable apart from "nothing wrong".
	Available bool                     `json:"available"`
	Rows      []models.StoredPresetRow `json:"rows"`
	// Unrecallable counts the rows that occupy no button the speaker has.
	Unrecallable int `json:"unrecallable"`
	// SpeakerCount is how many presets the speaker itself reports, for the
	// comparison the player shows.
	SpeakerCount int `json:"speaker_count"`
	// Disagrees is the one flag the player acts on.
	Disagrees bool `json:"disagrees"`
}

// HandleStoredPresets reports what AfterTouch has stored for a device,
// alongside what the speaker reports.
func (app *WebApp) HandleStoredPresets(w http.ResponseWriter, r *http.Request) {
	device, ok := app.deviceForStoredPresets(w, r)
	if !ok {
		return
	}

	payload := StoredPresetsPayload{Rows: []models.StoredPresetRow{}}

	if app.StoredPresets != nil {
		id, account := storedPresetTarget(device, chi.URLParam(r, "id"))

		rows, err := app.StoredPresets(id, account)
		if err != nil {
			app.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		payload.Available = true
		payload.Rows = rowsOrEmpty(rows)
	}

	app.describeStoredPresets(&payload, device)
	app.sendStoredPresets(w, payload)
}

// HandleRepairStoredPresets deletes stored rows the caller named by position.
//
// This is a write the speaker cannot carry: the rows are AfterTouch's own, and
// several of them name no button the speaker could be asked about. It is the
// one editor action that goes to the service rather than through the speaker.
func (app *WebApp) HandleRepairStoredPresets(w http.ResponseWriter, r *http.Request) {
	device, ok := app.deviceForStoredPresets(w, r)
	if !ok {
		return
	}

	if app.RepairStoredPresets == nil {
		app.sendError(w, "This player has no AfterTouch service behind it, so there is no stored list to repair", http.StatusNotImplemented)
		return
	}

	var req struct {
		Drop     []int `json:"drop"`
		Expected int   `json:"expected"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Drop) == 0 {
		app.sendError(w, "Name at least one row to remove", http.StatusBadRequest)
		return
	}

	id, account := storedPresetTarget(device, chi.URLParam(r, "id"))

	rows, err := app.RepairStoredPresets(id, account, req.Drop, req.Expected)
	if err != nil {
		// A refused repair is the caller's to resolve (a stale view, an
		// index that is not in the list), not a service failure.
		app.sendError(w, err.Error(), http.StatusConflict)

		return
	}

	payload := StoredPresetsPayload{Available: true, Rows: rowsOrEmpty(rows)}
	app.describeStoredPresets(&payload, device)
	app.sendStoredPresets(w, payload)
}

func (app *WebApp) deviceForStoredPresets(w http.ResponseWriter, r *http.Request) (*webtypes.DeviceConnection, bool) {
	device, exists := app.GetDevice(chi.URLParam(r, "id"))
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)

		return nil, false
	}

	return device, true
}

// storedPresetTarget names the device directory to read, and the account it
// should be read under.
//
// Both come from the speaker's own /info, which the player already holds:
//
//   - the device id, because the datastore files a speaker under the id it
//     reports, not under the key the player registered it as;
//   - margeAccountUUID, because a speaker can have directories under several
//     accounts after being re-paired, and only one of them is the list the
//     speaker is actually being served. Guessing from what is on disk can pick
//     a months-old directory from a previous pairing and report a
//     disagreement that says more about the leftover than about the speaker.
//
// An empty account leaves the choice to the service, which falls back to its
// on-disk guess.
func storedPresetTarget(device *webtypes.DeviceConnection, fallback string) (deviceID, account string) {
	info := device.Info()
	if info == nil {
		return fallback, ""
	}

	deviceID = info.DeviceID
	if deviceID == "" {
		deviceID = fallback
	}

	return deviceID, info.MargeAccountUUID
}

// describeStoredPresets fills in the comparison the player acts on. The
// speaker's own preset list comes from the status the player already holds, so
// this costs no extra speaker call.
func (app *WebApp) describeStoredPresets(payload *StoredPresetsPayload, device *webtypes.DeviceConnection) {
	for i := range payload.Rows {
		if !payload.Rows[i].OccupiesAButton() {
			payload.Unrecallable++
		}
	}

	if status := device.Status(); status != nil && status.Presets != nil {
		for i := range status.Presets.Preset {
			if status.Presets.Preset[i].ContentItem != nil {
				payload.SpeakerCount++
			}
		}
	}

	// Rows the speaker can never recall are a disagreement whatever it
	// reports. A stored list longer than the speaker's is one too, even when
	// every row looks valid on its own: the speaker has six buttons, and the
	// surplus is what a sync refuses to shrink.
	payload.Disagrees = payload.Available &&
		(payload.Unrecallable > 0 || len(payload.Rows) > payload.SpeakerCount)
}

func rowsOrEmpty(rows []models.StoredPresetRow) []models.StoredPresetRow {
	if rows == nil {
		return []models.StoredPresetRow{}
	}

	return rows
}

func (app *WebApp) sendStoredPresets(w http.ResponseWriter, payload StoredPresetsPayload) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: payload}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
