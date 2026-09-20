package soundtouchweb

import (
	"encoding/json"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/service/catalog"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// CatalogPayload is what the player receives from GET /api/control/catalog.
//
// Available says whether there is a catalog at all, rather than leaving the
// player to guess from an empty list: a standalone soundtouch-player has no
// datastore behind it, and an operator can switch the catalog off. Those both
// mean "no pick list here", which is a different thing to tell the owner than
// "nothing seen yet".
type CatalogPayload struct {
	Available bool            `json:"available"`
	Entries   []catalog.Entry `json:"entries"`
}

// HandleCatalog returns the preset/source entries this service has seen
// (issue 754), newest sighting first.
func (app *WebApp) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	payload := CatalogPayload{Entries: []catalog.Entry{}}

	if app.CatalogEntries != nil {
		payload.Available = true

		if entries := app.CatalogEntries(); entries != nil {
			payload.Entries = entries
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: payload}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
