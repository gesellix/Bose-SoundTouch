package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

// libraryServer is the JSON DTO for a DLNA media server. The registered and
// ready fields reflect state on the specific speaker that was queried;
// HandleDiscoverLibraryServers leaves them false because it performs a
// LAN-wide sweep with no device context.
type libraryServer struct {
	UDN           string `json:"udn"`
	Name          string `json:"name"`
	Manufacturer  string `json:"manufacturer"`
	Model         string `json:"model"`
	CDSControlURL string `json:"cdsControlURL"`
	Registered    bool   `json:"registered"`
	Ready         bool   `json:"ready"`
}

// libraryEntry is the JSON DTO for a single item returned by the speaker's
// /navigate endpoint. IsDir is derived from the item type so the frontend
// can render directory entries differently without an extra string comparison.
type libraryEntry struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Location      string `json:"location"`
	SourceAccount string `json:"sourceAccount"`
	Playable      bool   `json:"playable"`
	IsDir         bool   `json:"isDir"`
}

// libraryPage wraps a slice of libraryEntry as the Data payload.
type libraryPage struct {
	Entries    []libraryEntry `json:"entries"`
	TotalItems int            `json:"totalItems"`
}

// HandleDiscoverLibraryServers performs a LAN-wide SSDP sweep for DLNA media
// servers and returns them as a JSON array. An optional ?timeout= query
// parameter (in seconds, integer) overrides the default 5-second budget.
// This handler is global (not device-scoped) and lives under
// /api/control/providers/library/servers.
func (app *WebApp) HandleDiscoverLibraryServers(w http.ResponseWriter, r *http.Request) {
	timeout := 5 * time.Second

	if raw := r.URL.Query().Get("timeout"); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
		}
	}

	servers, err := discovery.DiscoverMediaServers(r.Context(), timeout)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]libraryServer, 0, len(servers))
	for _, s := range servers {
		out = append(out, libraryServer{
			UDN:           s.UDN,
			Name:          s.FriendlyName,
			Manufacturer:  s.Manufacturer,
			Model:         s.ModelName,
			CDSControlURL: s.CDSControlURL,
		})
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: out}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleDeviceLibraryServers returns the STORED_MUSIC sources currently
// registered on a specific speaker. Each source corresponds to one DLNA
// server that has been paired with that device.
func (app *WebApp) HandleDeviceLibraryServers(w http.ResponseWriter, r *http.Request) {
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

	sources, err := device.Client.GetSources()
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]libraryServer, 0)

	for _, si := range sources.SourceItem {
		if si.Source != "STORED_MUSIC" {
			continue
		}

		udn := strings.TrimSuffix(si.SourceAccount, "/0")
		out = append(out, libraryServer{
			UDN:        udn,
			Name:       si.DisplayName,
			Registered: true,
			Ready:      si.Status == "READY",
		})
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: out}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleAddLibraryServer registers a DLNA media server on a specific speaker
// using the speaker's setMusicServiceAccount endpoint. The request body must
// contain {udn, name}. The account sent to the speaker is "<udn>/0" as
// required by the STORED_MUSIC protocol. Error code 1024 from the speaker
// means the account is already registered and is treated as success.
func (app *WebApp) HandleAddLibraryServer(w http.ResponseWriter, r *http.Request) {
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
		UDN  string `json:"udn"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.UDN == "" {
		app.sendError(w, "udn is required", http.StatusBadRequest)
		return
	}

	account := req.UDN + "/0"

	if err := device.Client.AddStoredMusicAccount(account, req.Name); err != nil {
		// Error code 1024 means the account is already registered on the speaker.
		// Treat it as success so callers can be idempotent.
		if !strings.Contains(err.Error(), "1024") {
			app.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]string{"account": account},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleRemoveLibraryServer unregisters a DLNA media server from a specific
// speaker. The {account} URL parameter is the full account string (e.g.
// "uuid:1234.../0") and must be URL-encoded by the caller.
func (app *WebApp) HandleRemoveLibraryServer(w http.ResponseWriter, r *http.Request) {
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

	rawAccount := chi.URLParam(r, "account")

	account, err := url.PathUnescape(rawAccount)
	if err != nil {
		account = rawAccount
	}

	if account == "" {
		app.sendError(w, "account is required", http.StatusBadRequest)
		return
	}

	if err := device.Client.RemoveStoredMusicAccount(account, ""); err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleLibraryBrowse browses STORED_MUSIC content via the speaker's own
// /navigate endpoint, which returns speaker-native location tokens. These
// tokens are what /select requires when playing a track; raw DLNA ContentID
// values are not accepted by the speaker.
//
// Query parameters:
//   - account  (required) the sourceAccount string, e.g. "uuid:.../0"
//   - location (optional) location token from a previous browse; empty means root
//   - type     (optional) type hint for the container item, defaults to "dir"
//   - start    (optional) 1-based start index, defaults to 1
//   - count    (optional) number of items to return, defaults to 200
func (app *WebApp) HandleLibraryBrowse(w http.ResponseWriter, r *http.Request) {
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

	account := r.URL.Query().Get("account")
	if account == "" {
		app.sendError(w, "account is required", http.StatusBadRequest)
		return
	}

	location := r.URL.Query().Get("location")
	itemType := r.URL.Query().Get("type")

	start := 1

	if raw := r.URL.Query().Get("start"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 1 {
			start = v
		}
	}

	count := 200

	if raw := r.URL.Query().Get("count"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 1 {
			count = v
		}
	}

	var (
		resp   *models.NavigateResponse
		navErr error
	)

	if location == "" {
		resp, navErr = device.Client.Navigate("STORED_MUSIC", account, start, count)
	} else {
		if itemType == "" {
			itemType = "dir"
		}

		container := &models.ContentItem{
			Source:        "STORED_MUSIC",
			SourceAccount: account,
			Location:      location,
			Type:          itemType,
		}
		resp, navErr = device.Client.NavigateContainer("STORED_MUSIC", account, start, count, container)
	}

	if navErr != nil {
		app.sendError(w, navErr.Error(), http.StatusInternalServerError)
		return
	}

	entries := make([]libraryEntry, 0, len(resp.Items))

	for _, item := range resp.Items {
		loc := ""
		if item.ContentItem != nil {
			loc = item.ContentItem.Location
		}

		entries = append(entries, libraryEntry{
			Name:          item.GetDisplayName(),
			Type:          item.Type,
			Location:      loc,
			SourceAccount: account,
			Playable:      item.Playable == 1,
			IsDir:         item.Type == "dir",
		})
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data: libraryPage{
			Entries:    entries,
			TotalItems: resp.TotalItems,
		},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandlePlayLibrary plays a STORED_MUSIC track on a specific speaker using
// the speaker's /select endpoint. The request body must contain:
//   - account  (required) sourceAccount, e.g. "uuid:.../0"
//   - location (required) the speaker-native location token from /navigate
//   - type     (optional) content type, defaults to "track"
//   - name     (optional) display name logged with the playback request
func (app *WebApp) HandlePlayLibrary(w http.ResponseWriter, r *http.Request) {
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
		Account  string `json:"account"`
		Location string `json:"location"`
		Type     string `json:"type"`
		Name     string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Account == "" {
		app.sendError(w, "account is required", http.StatusBadRequest)
		return
	}

	if req.Location == "" {
		app.sendError(w, "location is required", http.StatusBadRequest)
		return
	}

	itemType := req.Type
	if itemType == "" {
		itemType = "track"
	}

	ci := &models.ContentItem{
		Source:        "STORED_MUSIC",
		SourceAccount: req.Account,
		Location:      req.Location,
		Type:          itemType,
		ItemName:      req.Name,
		IsPresetable:  true,
	}

	logPlaybackRequest("library", deviceID, ci.Source, ci.SourceAccount, ci.Location, ci.ItemName)

	if err := device.Client.SelectContentItem(ci); err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Playing " + req.Name},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
