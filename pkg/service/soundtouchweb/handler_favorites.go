package soundtouchweb

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/favorites"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

// FavoritesPayload distinguishes an unwired library from an empty selection.
type FavoritesPayload struct {
	Available bool              `json:"available"`
	Entries   []favorites.Entry `json:"entries"`
}

// HandleFavorites reads explicit selections, without probing any speaker.
func (app *WebApp) HandleFavorites(w http.ResponseWriter, _ *http.Request) {
	payload := FavoritesPayload{Available: app.Favorites != nil, Entries: []favorites.Entry{}}
	if app.Favorites != nil {
		entries, err := app.Favorites.List()
		if err != nil {
			log.Printf("Favorites read failed: %v", err)
			app.sendError(w, "Could not load favorites. Your saved library has not been replaced.", http.StatusInternalServerError)

			return
		}

		if entries != nil {
			payload.Entries = entries
		}
	}

	app.sendFavorites(w, payload)
}

// HandleAddFavorite is idempotent by content identity. It never writes a preset.
func (app *WebApp) HandleAddFavorite(w http.ResponseWriter, r *http.Request) {
	if !app.requireFavorites(w) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var item *models.ContentItem
	if err := decoder.Decode(&item); err != nil || item == nil {
		app.sendError(w, "Provide one valid content item.", http.StatusBadRequest)
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		app.sendError(w, "Provide exactly one content item.", http.StatusBadRequest)
		return
	}

	if err := favorites.Validate(*item); err != nil {
		app.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	entry, err := app.Favorites.Add(*item)
	if err != nil {
		log.Printf("Favorites save failed: %v", err)
		app.sendError(w, "Could not save the favorite. Refresh the library before retrying.", http.StatusInternalServerError)

		return
	}

	app.broadcastFavoritesChanged()
	app.sendFavorites(w, entry)
}

// HandleRemoveFavorite deletes one stable ID; retrying a missing ID is harmless.
func (app *WebApp) HandleRemoveFavorite(w http.ResponseWriter, r *http.Request) {
	if !app.requireFavorites(w) {
		return
	}

	id := chi.URLParam(r, "favoriteID")
	if _, err := hex.DecodeString(id); len(id) != 64 || err != nil {
		app.sendError(w, "Invalid favorite ID.", http.StatusBadRequest)
		return
	}

	if err := app.Favorites.Remove(id); err != nil {
		log.Printf("Favorites removal failed: %v", err)
		app.sendError(w, "Could not remove the favorite. Refresh the library before retrying.", http.StatusInternalServerError)

		return
	}

	app.broadcastFavoritesChanged()
	app.sendFavorites(w, struct{}{})
}

func (app *WebApp) requireFavorites(w http.ResponseWriter) bool {
	if app.Favorites == nil {
		app.sendError(w, "Favorites storage is not configured on this player.", http.StatusServiceUnavailable)
		return false
	}

	return true
}

func (app *WebApp) sendFavorites(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: data}); err != nil {
		log.Printf("Favorites response failed: %v", err)
	}
}

// This invalidation carries no library snapshot. The receiving browser reads
// the current file, so delayed delivery cannot roll it back to an older list.
// Store and registry locks have been released before browser I/O starts.
func (app *WebApp) broadcastFavoritesChanged() {
	for _, client := range app.globalWebSocketClients() {
		if err := app.withConnWrite(client, func(batch webSocketWriteBatch) error {
			return batch.writeJSON(client, webtypes.WebSocketMessage{Type: "favorites_changed"})
		}); err != nil {
			app.removeGlobalWebSocketClient(client)
		}
	}
}
