package soundtouchweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/catalog"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
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
	// Slots pairs the two lists button by button, for the slots where they
	// differ. It is what lets a disagreement be settled one slot at a time
	// instead of by the whole-list import that "Sync Data" offers.
	Slots []StoredPresetSlot `json:"slots"`
}

// StoredPresetSlot is one button where the stored list and the speaker
// disagree: what each side holds, so the owner can pick a side per slot.
type StoredPresetSlot struct {
	Slot int `json:"slot"`
	// Ours is the content AfterTouch stores for this button, empty when it
	// stores nothing.
	Ours SlotContent `json:"ours"`
	// Theirs is what the speaker reports for the same button.
	Theirs SlotContent `json:"theirs"`
}

// SlotContent is one side of a slot disagreement, in the shape the store
// endpoint takes back, so the player can hand it straight to a write.
type SlotContent struct {
	Present       bool   `json:"present"`
	Source        string `json:"source,omitempty"`
	SourceAccount string `json:"sourceAccount,omitempty"`
	Location      string `json:"location,omitempty"`
	Type          string `json:"type,omitempty"`
	ItemName      string `json:"itemName,omitempty"`
	ContainerArt  string `json:"containerArt,omitempty"`
}

func (c SlotContent) identity() string {
	if !c.Present {
		return ""
	}

	return catalog.Identity(c.Source, c.SourceAccount, c.Location)
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

	payload.Slots = differingSlots(payload.Rows, device)

	// Rows the speaker can never recall are a disagreement whatever it
	// reports. A stored list longer than the speaker's is one too, even when
	// every row looks valid on its own: the speaker has six buttons, and the
	// surplus is what a sync refuses to shrink. And a button the two sides
	// fill differently is one even when the counts match.
	payload.Disagrees = payload.Available &&
		(payload.Unrecallable > 0 || len(payload.Rows) > payload.SpeakerCount || len(payload.Slots) > 0)
}

// differingSlots pairs the two lists button by button and keeps only the
// buttons where they hold different content.
//
// Content identity decides, not the name: the same station can be stored under
// two names, and two different stations can share one. A slot only one side
// fills is a difference too -- that is a preset the speaker has and AfterTouch
// would overwrite at its next fetch, or the other way round.
func differingSlots(rows []models.StoredPresetRow, device *webtypes.DeviceConnection) []StoredPresetSlot {
	ours := map[int]SlotContent{}

	for i := range rows {
		if rows[i].Verdict != models.StoredPresetOK {
			continue
		}

		ours[rows[i].Slot] = SlotContent{
			Present:       true,
			Source:        rows[i].Source,
			SourceAccount: rows[i].SourceAccount,
			Location:      rows[i].Location,
			ItemName:      rows[i].Name,
			ContainerArt:  rows[i].ContainerArt,
		}
	}

	theirs := map[int]SlotContent{}

	if status := device.Status(); status != nil && status.Presets != nil {
		for i := range status.Presets.Preset {
			preset := &status.Presets.Preset[i]
			if preset.ContentItem == nil {
				continue
			}

			theirs[preset.ID] = SlotContent{
				Present:       true,
				Source:        preset.ContentItem.Source,
				SourceAccount: preset.ContentItem.SourceAccount,
				Location:      preset.ContentItem.Location,
				Type:          preset.ContentItem.Type,
				ItemName:      preset.ContentItem.ItemName,
				ContainerArt:  preset.ContentItem.ContainerArt,
			}
		}
	}

	slots := []StoredPresetSlot{}

	for slot := 1; slot <= datastore.PresetButtons; slot++ {
		mine, yours := ours[slot], theirs[slot]

		if !mine.Present && !yours.Present {
			continue
		}

		if mine.identity() == yours.identity() {
			continue
		}

		slots = append(slots, StoredPresetSlot{Slot: slot, Ours: mine, Theirs: yours})
	}

	return slots
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

// HandleAdoptSpeakerPreset takes the speaker's side for one slot: what the
// speaker reports for that button becomes what AfterTouch stores (issue 697).
//
// This is the per-slot alternative to "Sync Data", which is all or nothing and
// refuses outright when it would shrink the stored list. Here the owner
// settles one button at a time, and what is displaced stays on the pick list,
// so the choice can be undone.
//
// The speaker is asked again rather than trusted from the cached status: the
// point of adopting is to take what the speaker actually has now, and a status
// can be minutes old.
func (app *WebApp) HandleAdoptSpeakerPreset(w http.ResponseWriter, r *http.Request) {
	device, ok := app.deviceForStoredPresets(w, r)
	if !ok {
		return
	}

	if app.AdoptSpeakerPreset == nil {
		app.sendError(w, "This player has no AfterTouch service behind it, so there is no stored list to change", http.StatusNotImplemented)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		Slot int `json:"slot"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Slot < 1 || req.Slot > datastore.PresetButtons {
		app.sendError(w, fmt.Sprintf("Preset slot must be between 1 and %d", datastore.PresetButtons), http.StatusBadRequest)
		return
	}

	presets, err := device.Client.GetPresets()
	if err != nil {
		app.sendError(w, "Could not ask the speaker what it has: "+err.Error(), http.StatusBadGateway)
		return
	}

	content := speakerPresetContent(presets, req.Slot)
	if content == nil {
		// Nothing to adopt. Emptying the stored slot instead would be a
		// different decision, and not the one that was asked for.
		app.sendError(w, fmt.Sprintf("The speaker holds nothing in preset %d", req.Slot), http.StatusConflict)

		return
	}

	id, account := storedPresetTarget(device, chi.URLParam(r, "id"))

	rows, err := app.AdoptSpeakerPreset(id, account, presetFromSpeaker(req.Slot, content))
	if err != nil {
		app.sendError(w, err.Error(), http.StatusConflict)
		return
	}

	payload := StoredPresetsPayload{Available: true, Rows: rowsOrEmpty(rows)}
	app.describeStoredPresets(&payload, device)
	app.sendStoredPresets(w, payload)
}

func speakerPresetContent(presets *models.Presets, slot int) *models.ContentItem {
	if presets == nil {
		return nil
	}

	for i := range presets.Preset {
		if presets.Preset[i].ID == slot {
			return presets.Preset[i].ContentItem
		}
	}

	return nil
}

// presetFromSpeaker builds the stored row from what the speaker reports.
// IsPresetable is carried across rather than forced to true: the speaker's
// verdict on whether it can recall its own content is the one that matters
// (GH-235).
func presetFromSpeaker(slot int, content *models.ContentItem) models.ServicePreset {
	preset := models.ServicePreset{
		ID:           strconv.Itoa(slot),
		ButtonNumber: strconv.Itoa(slot),
		ContainerArt: content.ContainerArt,
	}

	preset.Source = content.Source
	preset.SourceAccount = content.SourceAccount
	preset.Location = content.Location
	preset.Type = content.Type
	preset.ContentItemType = content.Type
	preset.Name = content.ItemName

	if content.IsPresetable {
		preset.IsPresetable = "true"
	} else {
		preset.IsPresetable = "false"
	}

	return preset
}
