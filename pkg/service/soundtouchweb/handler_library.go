package soundtouchweb

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

// defaultLibraryPageSize is how many items one browse asks the speaker for
// when the caller does not say.
//
// The speaker honours numItems well past this (measured on a SoundTouch 10
// against a 484-entry folder: 200, 400 and 1000 all came back correctly, the
// last one with every entry), so a larger page costs response size rather than
// correctness. 500 covers most real folders in one request while leaving the
// paging path in use for the libraries that need it (issue 583).
//
// A named constant because the right value depends on the library and on how
// the browser handles a long list, so this is a likely candidate for a setting
// later.
const defaultLibraryPageSize = 500

// sourcesUpdatedSettleDelay is how long a refresh waits between telling a
// speaker its sources changed and asking what it now has. Measured informally:
// a speaker acts on the notification within a second. Too short and the
// re-read returns the list we already had; too long and the UI feels stuck.
const sourcesUpdatedSettleDelay = 1500 * time.Millisecond

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

	// IsPresetable reports the speaker's own isPresetable attribute for this
	// item, which is what says whether it can be saved to a preset slot. It
	// was previously dropped here, so the UI had no way to know.
	//
	// Read false as "unknown", not "no": models.ContentItem.IsPresetable is a
	// plain bool, so an absent attribute and an explicit false are the same
	// value by the time it reaches us. Both media servers measured (a
	// FRITZ!Box UPnP server and this repo's example-dlna-server) reported
	// true on every directory and track, so a false in practice means the
	// attribute was missing.
	IsPresetable bool `json:"isPresetable"`
}

// libraryPage wraps a slice of libraryEntry as the Data payload.
type libraryPage struct {
	Entries    []libraryEntry `json:"entries"`
	TotalItems int            `json:"totalItems"`
}

// normalizeUDN strips the "uuid:" prefix that UPnP device descriptions include
// in the UDN field (e.g. "uuid:fa095ecc-e13e-40e7-8e6c-e0286d5bc000") so the
// result matches the bare UUID that a SoundTouch speaker uses as the
// STORED_MUSIC sourceAccount before the "/0" suffix is appended.
func normalizeUDN(s string) string {
	return strings.TrimPrefix(s, "uuid:")
}

// HandleDiscoverLibraryServers performs a LAN-wide SSDP sweep for DLNA media
// servers, plus a query to every paired speaker's own /listMediaServers, and
// returns the merged set as a JSON array. An optional ?timeout= query
// parameter (in seconds, integer) overrides the default 5-second SSDP budget.
// This handler is global (not device-scoped) and lives under
// /api/control/providers/library/servers.
//
// The two sources see different networks: our SSDP sweep runs from the
// AfterTouch service host, while each speaker's /listMediaServers reflects
// what that speaker sees on its own LAN segment. They can disagree when the
// service isn't co-located with the speaker (different subnet/VLAN), so a
// server invisible to one path may still be visible via the other.
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

	byUDN := make(map[string]libraryServer, len(servers))
	order := make([]string, 0, len(servers))

	for _, s := range servers {
		udn := normalizeUDN(s.UDN)
		byUDN[udn] = libraryServer{
			UDN:           udn,
			Name:          s.FriendlyName,
			Manufacturer:  s.Manufacturer,
			Model:         s.ModelName,
			CDSControlURL: s.CDSControlURL,
		}
		order = append(order, udn)
	}

	deviceFound := app.discoverDeviceMediaServers()
	for i := range deviceFound {
		found := &deviceFound[i]

		udn := normalizeUDN(found.ID)
		if _, exists := byUDN[udn]; exists {
			// Already found via SSDP, which carries CDSControlURL; keep that
			// entry since registration itself only needs the UDN and name.
			continue
		}

		byUDN[udn] = libraryServer{
			UDN:          udn,
			Name:         found.FriendlyName,
			Manufacturer: found.Manufacturer,
			Model:        found.ModelName,
		}
		order = append(order, udn)
	}

	out := make([]libraryServer, 0, len(order))
	for _, udn := range order {
		out = append(out, byUDN[udn])
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: out}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// discoverDeviceMediaServers queries every paired speaker's own
// /listMediaServers endpoint concurrently and returns the union of what they
// report, deduplicated by UDN. A speaker that is offline, times out, or runs
// firmware without the endpoint is skipped silently: this is a best-effort
// second discovery path, and one unreachable speaker must not fail or delay
// the overall response.
func (app *WebApp) discoverDeviceMediaServers() []models.MediaServerInfo {
	devices := app.DeviceSnapshot()

	type result struct {
		servers []models.MediaServerInfo
	}

	results := make(chan result, len(devices))

	var wg sync.WaitGroup

	for _, entry := range devices {
		deviceClient := entry.Device.Client
		if deviceClient == nil {
			continue
		}

		wg.Add(1)

		go func(c *client.Client) {
			defer wg.Done()

			resp, err := c.ListMediaServers()
			if err != nil || resp == nil {
				results <- result{}
				return
			}

			results <- result{servers: resp.MediaServers}
		}(deviceClient)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	seen := make(map[string]bool)

	out := make([]models.MediaServerInfo, 0, len(devices))

	for r := range results {
		for i := range r.servers {
			s := &r.servers[i]

			udn := normalizeUDN(s.ID)
			if udn == "" || seen[udn] {
				continue
			}

			seen[udn] = true

			out = append(out, *s)
		}
	}

	return out
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

	out := storedMusicServers(sources)

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: out}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleRefreshLibraryServers re-reads a speaker's STORED_MUSIC sources after
// asking it to re-read its own account list first.
//
// A media server sometimes disappears from one speaker's source list while
// other speakers still see it (issue 580). Rebooting that speaker brings it
// back, which says the registration itself survived and only the speaker's
// live view of it was lost. The same sourcesUpdated nudge that makes a newly
// registered server appear without a power cycle (see HandleAddLibraryServer)
// is the cheapest thing that can rebuild that view, so this handler offers it
// as an explicit gesture rather than making the user reboot.
//
// The nudge is best effort and its outcome is reported as "refreshed": the
// re-read happens either way, so a speaker that ignores the notification still
// answers with its current list rather than an error. Whether this actually
// restores a lost library is not confirmed; the reporter of issue 580 has the
// intermittent case we cannot reproduce here.
func (app *WebApp) HandleRefreshLibraryServers(w http.ResponseWriter, r *http.Request) {
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

	refreshed := false

	if boseDeviceID := app.boseDeviceID(device); boseDeviceID != "" {
		if err := device.Client.NotifySourcesUpdated(boseDeviceID); err == nil {
			refreshed = true
		} else {
			slog.Debug("library refresh: sourcesUpdated failed", "err", sanitizeLog(err.Error()))
		}
	}

	// Give the speaker a moment to act on the notification before asking what
	// it now has. Without this the re-read races the speaker's own work and
	// reports the list we were already showing.
	if refreshed {
		select {
		case <-time.After(sourcesUpdatedSettleDelay):
		case <-r.Context().Done():
			return
		}
	}

	sources, err := device.Client.GetSources()
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := storedMusicServers(sources)

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]interface{}{"servers": out, "refreshed": refreshed},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// boseDeviceID resolves the speaker's own device ID for notifications,
// preferring the cached DeviceInfo over a live /info round-trip.
func (app *WebApp) boseDeviceID(device *webtypes.DeviceConnection) string {
	if device.DeviceInfo != nil && device.DeviceInfo.DeviceID != "" {
		return device.DeviceInfo.DeviceID
	}

	if info, err := device.Client.GetDeviceInfo(); err == nil && info != nil {
		return info.DeviceID
	}

	return ""
}

// storedMusicServers maps a speaker's /sources response to the media servers
// registered on it.
func storedMusicServers(sources *models.Sources) []libraryServer {
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

	return out
}

// HandleAddLibraryServer registers a DLNA media server on a specific speaker
// using the speaker's setMusicServiceAccount endpoint. The request body must
// contain {udn, name}. The account sent to the speaker is "<udn>/0" as
// required by the STORED_MUSIC protocol. Error code 1024 from the speaker
// means the account is already registered and is treated as success.
//
// After a successful registration the handler fires a best-effort
// sourcesUpdated notification so the speaker re-fetches its account list and
// registers the new source without requiring a power-cycle. The notification
// outcome is reflected in the response field "refreshed" but never fails the
// request.
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

	account := normalizeUDN(req.UDN) + "/0"

	if err := device.Client.AddStoredMusicAccount(account, req.Name); err != nil {
		// Error code 1024 means the account is already registered on the speaker.
		// Treat it as success so callers can be idempotent.
		if !strings.Contains(err.Error(), "1024") {
			app.sendError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Resolve the Bose device ID for the sourcesUpdated nudge. Prefer the
	// cached DeviceInfo (no extra round-trip); fall back to a live /info
	// fetch only if the cached value is absent or empty.
	boseDeviceID := ""
	if device.DeviceInfo != nil && device.DeviceInfo.DeviceID != "" {
		boseDeviceID = device.DeviceInfo.DeviceID
	} else {
		if info, infoErr := device.Client.GetDeviceInfo(); infoErr == nil && info != nil {
			boseDeviceID = info.DeviceID
		}
	}

	// Send the sourcesUpdated nudge best-effort: the registration already
	// succeeded, so an error here must never fail the request.
	refreshed := false

	if boseDeviceID != "" {
		if nudgeErr := device.Client.NotifySourcesUpdated(boseDeviceID); nudgeErr == nil {
			refreshed = true
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]interface{}{"account": account, "refreshed": refreshed},
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
//   - count    (optional) number of items to return, defaults to
//     defaultLibraryPageSize
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

	count := defaultLibraryPageSize

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
		presetable := false

		// The item's own ContentItem, not the one inside mediaItemContainer:
		// that one repeats the parent container identically on every item.
		// models.NavigateItem keeps them apart.
		if item.ContentItem != nil {
			loc = item.ContentItem.Location
			presetable = item.ContentItem.IsPresetable
		}

		entries = append(entries, libraryEntry{
			Name:          item.GetDisplayName(),
			Type:          item.Type,
			Location:      loc,
			SourceAccount: account,
			Playable:      item.Playable == 1,
			IsDir:         item.Type == "dir",
			IsPresetable:  presetable,
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
