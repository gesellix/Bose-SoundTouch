package soundtouchweb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// cannedNavigateResponse is an XML navigateResponse the fake speaker returns
// for /navigate in browse tests: one directory and one track, so we can assert
// both are mapped correctly.
//
// The shape is taken from real captures (issue 700) rather than written by
// hand, because the hand-written version disagreed with hardware on four
// counts and the wrong one was load-bearing: it claimed isPresetable="false"
// on the directory, which was the only reason to doubt that saving a folder to
// a preset works at all. Captured from a SoundTouch 10 (FW 27.0.6) against two
// independent media servers, which agreed on all of it:
//
//   - directories report Playable="1", not "0"
//   - isPresetable is "true" on directories and tracks alike
//   - the ContentItem carries NO type attribute; the kind is the <type>
//     element on the item
//   - every item carries a <mediaItemContainer> whose ContentItem repeats the
//     parent container, so each item has two ContentItems
//
// Names and identifiers are placeholders; the structure is verbatim.
// Note: totalItems is an XML element, not an attribute, per models.NavigateResponse.
const cannedNavigateResponse = `<?xml version="1.0" encoding="UTF-8" ?>
<navigateResponse source="STORED_MUSIC" sourceAccount="uuid:test-udn/0">
  <totalItems>2</totalItems>
  <items>
    <item Playable="1">
      <name>Albums</name>
      <type>dir</type>
      <mediaItemContainer offset="0">
        <ContentItem source="STORED_MUSIC" location="0" sourceAccount="uuid:test-udn/0" isPresetable="true">
          <itemName>uuid:test-udn/0</itemName>
        </ContentItem>
      </mediaItemContainer>
      <ContentItem source="STORED_MUSIC" location="4:cont2:150:0:0:" sourceAccount="uuid:test-udn/0" isPresetable="true">
        <itemName>Albums</itemName>
      </ContentItem>
    </item>
    <item Playable="1">
      <name>Great Song</name>
      <type>track</type>
      <mediaItemContainer offset="1">
        <ContentItem source="STORED_MUSIC" location="4:cont2:150:0:0:" sourceAccount="uuid:test-udn/0" isPresetable="true">
          <itemName>Albums</itemName>
        </ContentItem>
      </mediaItemContainer>
      <ContentItem source="STORED_MUSIC" location="5:audio5:part13:3171:5 TRACK" sourceAccount="uuid:test-udn/0" isPresetable="true">
        <itemName>Great Song</itemName>
      </ContentItem>
    </item>
  </items>
</navigateResponse>`

// cannedSourcesResponse is a minimal /sources XML containing one STORED_MUSIC
// account for use in HandleDeviceLibraryServers tests.
const cannedSourcesResponse = `<?xml version="1.0" encoding="UTF-8" ?>
<sources deviceID="AABBCCDDEEFF">
  <sourceItem source="STORED_MUSIC" sourceAccount="uuid:nas-udn/0" status="READY" isLocal="false" multiroomallowed="true">My NAS</sourceItem>
  <sourceItem source="BLUETOOTH" sourceAccount="" status="READY" isLocal="true" multiroomallowed="false">Bluetooth</sourceItem>
</sources>`

// setupSpeakerMock creates an httptest.Server that captures request bodies for
// paths listed in captureMap, writes canned XML responses from responseMap,
// and returns HTTP 200 for everything else. Call speaker.Close() when done.
func setupSpeakerMock(t *testing.T, responseMap map[string]string) (*httptest.Server, map[string]string) {
	t.Helper()

	captured := map[string]string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, err := io.ReadAll(r.Body); err == nil {
			captured[r.URL.Path] = string(body)
		}

		if resp, ok := responseMap[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(resp))
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	return srv, captured
}

// newLibraryTestApp builds a WebApp with a single device whose Client points
// at the given speaker URL. The device is registered under "lib-device" with
// a non-empty DeviceID so HandleAddLibraryServer can resolve the Bose ID from
// the cached DeviceInfo without a /info fallback.
func newLibraryTestApp(speakerURL string) *WebApp {
	app := NewWebApp()

	c := client.NewClient(&client.Config{Host: speakerURL})
	info := &models.DeviceInfo{Name: "Library Test Speaker", DeviceID: "AABBCCDDEEFF"}
	conn := webtypes.NewDeviceConnection(c, info)
	conn.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
	app.AddDevice("lib-device", conn)

	return app
}

// ---- HandlePlayLibrary --------------------------------------------------

// TestHandlePlayLibrary_XMLShape verifies that the /select XML the handler
// posts to the speaker carries source="STORED_MUSIC", the given sourceAccount,
// location, and type="track".
func TestHandlePlayLibrary_XMLShape(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{
		"account":  "uuid:test-udn/0",
		"location": "5:audio5:part13:3171:5 TRACK",
		"type":     "track",
		"name":     "Great Song"
	}`)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/library/play", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandlePlayLibrary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success=true, got false (error=%s)", resp.Error)
	}

	selectXML := captured["/select"]
	if selectXML == "" {
		t.Fatal("speaker /select was never called")
	}

	for _, want := range []string{
		`source="STORED_MUSIC"`,
		`sourceAccount="uuid:test-udn/0"`,
		`location="5:audio5:part13:3171:5 TRACK"`,
		`type="track"`,
	} {
		if !strings.Contains(selectXML, want) {
			t.Errorf("select XML should contain %q, got:\n%s", want, selectXML)
		}
	}
}

// TestHandlePlayLibrary_DefaultsTypeToTrack checks that omitting "type" in
// the request body still sends type="track" in the /select XML.
func TestHandlePlayLibrary_DefaultsTypeToTrack(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{
		"account":  "uuid:test-udn/0",
		"location": "5:audio5:part13:3171:5 TRACK"
	}`)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/library/play", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandlePlayLibrary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if want := `type="track"`; !strings.Contains(captured["/select"], want) {
		t.Errorf("select XML should contain %q, got:\n%s", want, captured["/select"])
	}
}

// TestHandlePlayLibrary_MissingFields checks that missing required fields
// result in 400.
func TestHandlePlayLibrary_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing account", `{"location":"5:audio5:part13:3171:5 TRACK"}`},
		{"missing location", `{"account":"uuid:test-udn/0"}`},
		{"both missing", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			speaker, _ := setupSpeakerMock(t, nil)
			defer speaker.Close()

			app := newLibraryTestApp(speaker.URL)

			req := httptest.NewRequest("POST", "/api/control/devices/lib-device/library/play",
				strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = withChiParams(req, map[string]string{"id": "lib-device"})
			w := httptest.NewRecorder()

			app.HandlePlayLibrary(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}

			var resp webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if resp.Success {
				t.Error("expected success=false")
			}
		})
	}
}

// TestHandlePlayLibrary_UnknownDevice checks that requesting an unregistered
// device returns 404.
func TestHandlePlayLibrary_UnknownDevice(t *testing.T) {
	app := NewWebApp()

	body := strings.NewReader(`{"account":"uuid:test-udn/0","location":"5:audio5:part13:3171:5 TRACK"}`)
	req := httptest.NewRequest("POST", "/api/control/devices/ghost/library/play", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "ghost"})
	w := httptest.NewRecorder()

	app.HandlePlayLibrary(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- HandleLibraryBrowse -----------------------------------------------

// TestHandleLibraryBrowse_RootMapsEntries verifies that a root browse
// (no location) calls /navigate and maps both a directory and a track
// entry correctly.
func TestHandleLibraryBrowse_RootMapsEntries(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, map[string]string{
		"/navigate": cannedNavigateResponse,
	})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("GET",
		"/api/control/devices/lib-device/library/browse?account=uuid:test-udn/0",
		nil)
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleLibraryBrowse(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success=true, error=%s", resp.Error)
	}

	// Decode the page from the generic Data interface{}.
	pageBytes, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("re-marshal data: %v", err)
	}

	var page libraryPage
	if err := json.Unmarshal(pageBytes, &page); err != nil {
		t.Fatalf("unmarshal page: %v", err)
	}

	if page.TotalItems != 2 {
		t.Errorf("expected totalItems=2, got %d", page.TotalItems)
	}

	if len(page.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(page.Entries))
	}

	dir := page.Entries[0]

	if dir.Name != "Albums" {
		t.Errorf("dir name: expected 'Albums', got %q", dir.Name)
	}

	if dir.Type != "dir" {
		t.Errorf("dir type: expected 'dir', got %q", dir.Type)
	}

	if !dir.IsDir {
		t.Error("dir.IsDir should be true")
	}

	// Hardware reports Playable="1" on directories: the speaker will happily
	// play a whole folder, which is what makes a folder preset work.
	if !dir.Playable {
		t.Error("dir.Playable should be true, as hardware reports Playable=1 for directories")
	}

	// Dropped before issue 700, so the UI could not tell whether a folder was
	// savable to a preset.
	if !dir.IsPresetable {
		t.Error("dir.IsPresetable should be true")
	}

	// The item's own ContentItem, not the parent repeated inside
	// mediaItemContainer (location "0" here).
	if dir.Location != "4:cont2:150:0:0:" {
		t.Errorf("dir location: expected '4:cont2:150:0:0:', got %q", dir.Location)
	}

	track := page.Entries[1]

	if track.Name != "Great Song" {
		t.Errorf("track name: expected 'Great Song', got %q", track.Name)
	}

	if track.Type != "track" {
		t.Errorf("track type: expected 'track', got %q", track.Type)
	}

	if track.IsDir {
		t.Error("track.IsDir should be false")
	}

	if !track.Playable {
		t.Error("track.Playable should be true")
	}

	if track.Location != "5:audio5:part13:3171:5 TRACK" {
		t.Errorf("track location: expected '5:audio5:part13:3171:5 TRACK', got %q", track.Location)
	}

	if track.SourceAccount != "uuid:test-udn/0" {
		t.Errorf("track sourceAccount: expected 'uuid:test-udn/0', got %q", track.SourceAccount)
	}
}

// TestHandleLibraryBrowse_MissingAccount checks that omitting ?account=
// returns 400.
func TestHandleLibraryBrowse_MissingAccount(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("GET",
		"/api/control/devices/lib-device/library/browse",
		nil)
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleLibraryBrowse(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Success {
		t.Error("expected success=false")
	}
}

// TestHandleLibraryBrowse_UnknownDevice checks that a missing device returns 404.
func TestHandleLibraryBrowse_UnknownDevice(t *testing.T) {
	app := NewWebApp()

	req := httptest.NewRequest("GET",
		"/api/control/devices/ghost/library/browse?account=uuid:x/0",
		nil)
	req = withChiParams(req, map[string]string{"id": "ghost"})
	w := httptest.NewRecorder()

	app.HandleLibraryBrowse(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- HandleDeviceLibraryServers ----------------------------------------

// TestHandleDeviceLibraryServers_FiltersStoredMusic checks that only
// STORED_MUSIC sources are returned and that the UDN is stripped of the "/0"
// suffix.
func TestHandleDeviceLibraryServers_FiltersStoredMusic(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, map[string]string{
		"/sources": cannedSourcesResponse,
	})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("GET",
		"/api/control/devices/lib-device/library/servers",
		nil)
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleDeviceLibraryServers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success=true, error=%s", resp.Error)
	}

	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	var servers []libraryServer
	if err := json.Unmarshal(raw, &servers); err != nil {
		t.Fatalf("unmarshal servers: %v", err)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 STORED_MUSIC server, got %d", len(servers))
	}

	s := servers[0]

	if s.UDN != "uuid:nas-udn" {
		t.Errorf("UDN: expected 'uuid:nas-udn', got %q", s.UDN)
	}

	if s.Name != "My NAS" {
		t.Errorf("Name: expected 'My NAS', got %q", s.Name)
	}

	if !s.Registered {
		t.Error("Registered should be true")
	}

	if !s.Ready {
		t.Error("Ready should be true for READY status")
	}
}

// TestHandleDeviceLibraryServers_UnknownDevice checks that an unknown device
// returns 404.
func TestHandleDeviceLibraryServers_UnknownDevice(t *testing.T) {
	app := NewWebApp()

	req := httptest.NewRequest("GET",
		"/api/control/devices/ghost/library/servers",
		nil)
	req = withChiParams(req, map[string]string{"id": "ghost"})
	w := httptest.NewRecorder()

	app.HandleDeviceLibraryServers(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ---- HandleAddLibraryServer --------------------------------------------

// TestDiscoverLibraryServers_UDNNormalization verifies the mapping logic used
// inside HandleDiscoverLibraryServers: a MediaServer with a "uuid:"-prefixed
// UDN must produce a libraryServer DTO with the bare UUID (no prefix), because
// SoundTouch STORED_MUSIC sourceAccounts use the bare form.
// This exercises normalizeUDN indirectly through the same code path used in
// the handler loop; HandleDiscoverLibraryServers itself cannot be called in a
// unit test because it invokes the real SSDP stack.
func TestDiscoverLibraryServers_UDNNormalization(t *testing.T) {
	prefixedUDN := "uuid:fa095ecc-e13e-40e7-8e6c-e0286d5bc000"
	want := "fa095ecc-e13e-40e7-8e6c-e0286d5bc000"

	got := libraryServer{
		UDN: normalizeUDN(prefixedUDN),
	}

	if got.UDN != want {
		t.Errorf("libraryServer UDN after normalizeUDN = %q, want %q", got.UDN, want)
	}
}

// TestNormalizeUDN verifies that normalizeUDN strips the "uuid:" prefix and
// is a no-op when the prefix is absent.
func TestNormalizeUDN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"uuid:fa095ecc-e13e-40e7-8e6c-e0286d5bc000", "fa095ecc-e13e-40e7-8e6c-e0286d5bc000"},
		{"fa095ecc-e13e-40e7-8e6c-e0286d5bc000", "fa095ecc-e13e-40e7-8e6c-e0286d5bc000"},
		{"uuid:nas-udn", "nas-udn"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeUDN(tt.input)
		if got != tt.want {
			t.Errorf("normalizeUDN(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestHandleAddLibraryServer_AccountFormat verifies that the speaker receives
// a setMusicServiceAccount call with the account set to "<bare-uuid>/0", i.e.
// any "uuid:" prefix is stripped before the "/0" suffix is appended.
// It also asserts that a POST /notification (sourcesUpdated nudge) is sent
// after a successful registration and that the response carries refreshed=true.
func TestHandleAddLibraryServer_AccountFormat(t *testing.T) {
	tests := []struct {
		name        string
		requestUDN  string
		wantAccount string
	}{
		{
			name:        "bare UDN",
			requestUDN:  "nas-udn",
			wantAccount: "nas-udn/0",
		},
		{
			name:        "uuid-prefixed UDN is normalised",
			requestUDN:  "uuid:nas-udn",
			wantAccount: "nas-udn/0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The client parses the response XML and checks for the success sentinel.
			// Also handle /notification so NotifySourcesUpdated succeeds.
			speaker, captured := setupSpeakerMock(t, map[string]string{
				"/setMusicServiceAccount": `<status>/setMusicServiceAccount</status>`,
				"/notification":           `<status>/notification</status>`,
			})
			defer speaker.Close()

			app := newLibraryTestApp(speaker.URL)

			bodyStr := `{"udn":"` + tt.requestUDN + `","name":"My NAS"}`
			req := httptest.NewRequest("POST",
				"/api/control/devices/lib-device/library/servers",
				strings.NewReader(bodyStr))
			req.Header.Set("Content-Type", "application/json")
			req = withChiParams(req, map[string]string{"id": "lib-device"})
			w := httptest.NewRecorder()

			app.HandleAddLibraryServer(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var resp webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}

			if !resp.Success {
				t.Fatalf("expected success=true, error=%s", resp.Error)
			}

			// The speaker should have been called at the setMusicServiceAccount endpoint.
			setXML := captured["/setMusicServiceAccount"]
			if setXML == "" {
				t.Fatal("speaker /setMusicServiceAccount was never called")
			}

			if !strings.Contains(setXML, tt.wantAccount) {
				t.Errorf("setMusicServiceAccount XML should contain %q, got:\n%s", tt.wantAccount, setXML)
			}

			// A sourcesUpdated nudge must have been POSTed to /notification.
			notifXML := captured["/notification"]
			if notifXML == "" {
				t.Fatal("speaker /notification was never called (sourcesUpdated nudge missing)")
			}

			if !strings.Contains(notifXML, "sourcesUpdated") {
				t.Errorf("/notification body should contain 'sourcesUpdated', got:\n%s", notifXML)
			}

			// The response must carry the account and refreshed=true.
			data, ok := resp.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("resp.Data is not a map: %T", resp.Data)
			}

			if got, _ := data["account"].(string); got != tt.wantAccount {
				t.Errorf("response account = %q, want %q", got, tt.wantAccount)
			}

			if refreshed, _ := data["refreshed"].(bool); !refreshed {
				t.Errorf("response refreshed should be true, got %v", data["refreshed"])
			}
		})
	}
}

// TestHandleAddLibraryServer_NudgeSentAfterAlreadyRegistered verifies that
// the sourcesUpdated nudge is also fired for the 1024 (already-registered)
// idempotent path, since the source still needs to be re-registered on the
// speaker.
func TestHandleAddLibraryServer_NudgeSentAfterAlreadyRegistered(t *testing.T) {
	alreadyRegistered := `<errors deviceID="AABBCCDDEEFF">
		<error value="1024">1024: Account already exists</error>
	</errors>`

	var notifCalled int

	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/setMusicServiceAccount":
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(alreadyRegistered))
		case "/notification":
			notifCalled++
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<status>/notification</status>`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{"udn":"uuid:nas-udn","name":"My NAS"}`)
	req := httptest.NewRequest("POST",
		"/api/control/devices/lib-device/library/servers",
		body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleAddLibraryServer(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success=true when error contains 1024, got error=%s", resp.Error)
	}

	if notifCalled == 0 {
		t.Error("expected /notification to be called for already-registered path, but it was not")
	}
}

// TestHandleAddLibraryServer_MissingUDN checks that omitting udn returns 400.
func TestHandleAddLibraryServer_MissingUDN(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{"name":"My NAS"}`)
	req := httptest.NewRequest("POST",
		"/api/control/devices/lib-device/library/servers",
		body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleAddLibraryServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestHandleAddLibraryServer_AlreadyRegistered verifies that a speaker error
// response whose text contains "1024" is treated as success by the handler.
// The client surfaces the ErrorsResponse chardata as the error string; to
// make it contain "1024" we put that token in the message text and return
// HTTP 400 so the client takes the error-parse path.
func TestHandleAddLibraryServer_AlreadyRegistered(t *testing.T) {
	// Return HTTP 400 with an <errors> body so the client wraps it as an
	// ErrorsResponse whose .Error() text contains "1024".
	alreadyRegistered := `<errors deviceID="AABBCCDDEEFF">
		<error value="1024">1024: Account already exists</error>
	</errors>`

	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setMusicServiceAccount" {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(alreadyRegistered))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{"udn":"uuid:nas-udn","name":"My NAS"}`)
	req := httptest.NewRequest("POST",
		"/api/control/devices/lib-device/library/servers",
		body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleAddLibraryServer(w, req)

	// The error text includes "1024" so the handler should absorb it and return 200.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when error contains 1024, got %d: %s", w.Code, w.Body.String())
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success=true when error contains 1024, got error=%s", resp.Error)
	}
}

// ---- discoverDeviceMediaServers (speaker-side /listMediaServers) --------

// cannedListMediaServersResponse is a minimal /listMediaServers XML response
// with one server, used to exercise the speaker-side discovery merge path
// that complements our own SSDP sweep.
const cannedListMediaServersResponse = `<?xml version="1.0" encoding="UTF-8" ?>
<ListMediaServersResponse>
  <media_server id="uuid:fa095ecc-uuid" ip="198.51.100.10" manufacturer="AVM" model_name="FRITZ!Mediaserver" friendly_name="FRITZ!Mediaserver"/>
</ListMediaServersResponse>`

// newMediaServerTestDevice registers a device backed by speakerURL under id,
// mirroring newLibraryTestApp's setup but for tests that need more than one
// device on the same WebApp.
func newMediaServerTestDevice(app *WebApp, id, speakerURL string) {
	c := client.NewClient(&client.Config{Host: speakerURL})
	info := &models.DeviceInfo{DeviceID: id}
	app.AddDevice(id, webtypes.NewDeviceConnection(c, info))
}

// TestDiscoverDeviceMediaServers_QueriesEveryPairedSpeaker verifies that
// discoverDeviceMediaServers calls /listMediaServers on every paired device
// and returns the union of what they report. This is the path that lets
// discovery see a server the AfterTouch service's own SSDP sweep might miss
// because it isn't co-located with the speaker's LAN segment.
func TestDiscoverDeviceMediaServers_QueriesEveryPairedSpeaker(t *testing.T) {
	speakerA, _ := setupSpeakerMock(t, map[string]string{
		"/listMediaServers": cannedListMediaServersResponse,
	})
	defer speakerA.Close()

	speakerB, _ := setupSpeakerMock(t, map[string]string{
		"/listMediaServers": `<?xml version="1.0" encoding="UTF-8" ?>
<ListMediaServersResponse>
  <media_server id="uuid:other-udn" ip="198.51.100.20" manufacturer="Synology" model_name="DS220" friendly_name="NAS"/>
</ListMediaServersResponse>`,
	})
	defer speakerB.Close()

	app := NewWebApp()
	newMediaServerTestDevice(app, "dev-a", speakerA.URL)
	newMediaServerTestDevice(app, "dev-b", speakerB.URL)

	got := app.discoverDeviceMediaServers()

	if len(got) != 2 {
		t.Fatalf("expected 2 servers across both speakers, got %d: %+v", len(got), got)
	}

	udns := map[string]bool{}
	for _, s := range got {
		udns[normalizeUDN(s.ID)] = true
	}

	if !udns["fa095ecc-uuid"] || !udns["other-udn"] {
		t.Errorf("expected both UDNs present, got %+v", udns)
	}
}

// TestDiscoverDeviceMediaServers_DedupesAcrossSpeakers verifies that the same
// server reported by two speakers (e.g. two boxes on the same LAN both seeing
// one NAS) is returned only once, keyed by normalized UDN, even when the two
// speakers report the UDN in different forms (with/without "uuid:" prefix).
func TestDiscoverDeviceMediaServers_DedupesAcrossSpeakers(t *testing.T) {
	speakerA, _ := setupSpeakerMock(t, map[string]string{
		"/listMediaServers": cannedListMediaServersResponse,
	})
	defer speakerA.Close()

	speakerB, _ := setupSpeakerMock(t, map[string]string{
		"/listMediaServers": `<?xml version="1.0" encoding="UTF-8" ?>
<ListMediaServersResponse>
  <media_server id="fa095ecc-uuid" ip="198.51.100.10" manufacturer="AVM" model_name="FRITZ!Mediaserver" friendly_name="FRITZ!Mediaserver"/>
</ListMediaServersResponse>`,
	})
	defer speakerB.Close()

	app := NewWebApp()
	newMediaServerTestDevice(app, "dev-a", speakerA.URL)
	newMediaServerTestDevice(app, "dev-b", speakerB.URL)

	got := app.discoverDeviceMediaServers()

	if len(got) != 1 {
		t.Fatalf("expected 1 deduplicated server, got %d: %+v", len(got), got)
	}
}

// TestDiscoverDeviceMediaServers_UnreachableSpeakerSkippedSilently verifies
// that a speaker whose /listMediaServers call fails (offline, old firmware
// without the endpoint) does not prevent results from other, reachable
// speakers, and does not error the overall call.
func TestDiscoverDeviceMediaServers_UnreachableSpeakerSkippedSilently(t *testing.T) {
	speakerA, _ := setupSpeakerMock(t, map[string]string{
		"/listMediaServers": cannedListMediaServersResponse,
	})
	defer speakerA.Close()

	speakerB, _ := setupSpeakerMock(t, nil)
	speakerB.Close() // closed before use: every request to it fails outright.

	app := NewWebApp()
	newMediaServerTestDevice(app, "dev-a", speakerA.URL)
	newMediaServerTestDevice(app, "dev-b", speakerB.URL)

	got := app.discoverDeviceMediaServers()

	if len(got) != 1 {
		t.Fatalf("expected 1 server from the reachable speaker, got %d: %+v", len(got), got)
	}

	if normalizeUDN(got[0].ID) != "fa095ecc-uuid" {
		t.Errorf("expected the reachable speaker's server, got %+v", got[0])
	}
}

// TestHandleLibraryBrowse_PageSize checks the two halves of the paging
// contract (issue 583): the default page size the handler asks the speaker
// for, and that an explicit start and count are passed through so the player
// can fetch the next slice.
func TestHandleLibraryBrowse_PageSize(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantStart string
		wantNum   string
	}{
		{"defaults", "account=uuid:test-udn/0", "<startItem>1</startItem>", "<numItems>500</numItems>"},
		{"explicit page", "account=uuid:test-udn/0&start=501&count=250", "<startItem>501</startItem>", "<numItems>250</numItems>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			speaker, captured := setupSpeakerMock(t, map[string]string{"/navigate": cannedNavigateResponse})
			defer speaker.Close()

			app := newLibraryTestApp(speaker.URL)

			req := httptest.NewRequest("GET", "/api/control/devices/lib-device/library/browse?"+tt.query, nil)
			req = withChiParams(req, map[string]string{"id": "lib-device"})
			w := httptest.NewRecorder()

			app.HandleLibraryBrowse(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			navigateXML := captured["/navigate"]
			for _, want := range []string{tt.wantStart, tt.wantNum} {
				if !strings.Contains(navigateXML, want) {
					t.Errorf("navigate XML should contain %q, got:\n%s", want, navigateXML)
				}
			}
		})
	}
}

// TestHandleLibraryBrowse_ReportsTotalItems pins the field the player pages
// against: without it there is no way to know a page is partial.
func TestHandleLibraryBrowse_ReportsTotalItems(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, map[string]string{"/navigate": cannedNavigateResponse})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("GET", "/api/control/devices/lib-device/library/browse?account=uuid:test-udn/0", nil)
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleLibraryBrowse(w, req)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TotalItems int `json:"totalItems"`
			Entries    []struct {
				Name string `json:"name"`
			} `json:"entries"`
		} `json:"data"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success || resp.Data.TotalItems != 2 || len(resp.Data.Entries) != 2 {
		t.Errorf("got success=%v totalItems=%d entries=%d, want true/2/2",
			resp.Success, resp.Data.TotalItems, len(resp.Data.Entries))
	}
}

// cannedSourcesWithoutLibrary is the same speaker answering with no
// STORED_MUSIC source at all, which is what issue 580 reports: the media
// server disappears from one speaker while others still see it.
const cannedSourcesWithoutLibrary = `<?xml version="1.0" encoding="UTF-8" ?>
<sources deviceID="AABBCCDDEEFF">
  <sourceItem source="BLUETOOTH" sourceAccount="" status="READY" isLocal="true" multiroomallowed="false">Bluetooth</sourceItem>
</sources>`

// TestHandleRefreshLibraryServers_NudgesThenReads verifies the order the
// refresh depends on: the speaker is told its sources changed, and only then
// asked what it has. Reading first would report the stale list the user is
// already looking at.
func TestHandleRefreshLibraryServers_NudgesThenReads(t *testing.T) {
	var mu sync.Mutex
	var calls []string

	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()

		if r.URL.Path == "/sources" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(cannedSourcesResponse))

			return
		}

		// /notification answers with the posted status echoed back.
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?><status>/notification</status>`))
	}))
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/library/servers/refresh", nil)
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleRefreshLibraryServers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Refreshed bool `json:"refreshed"`
			Servers   []struct {
				UDN   string `json:"udn"`
				Name  string `json:"name"`
				Ready bool   `json:"ready"`
			} `json:"servers"`
		} `json:"data"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success || !resp.Data.Refreshed {
		t.Errorf("got success=%v refreshed=%v, want both true", resp.Success, resp.Data.Refreshed)
	}

	if len(resp.Data.Servers) != 1 || resp.Data.Servers[0].UDN != "uuid:nas-udn" || !resp.Data.Servers[0].Ready {
		t.Errorf("servers = %+v, want the one READY STORED_MUSIC source", resp.Data.Servers)
	}

	mu.Lock()
	defer mu.Unlock()

	notifyAt, sourcesAt := -1, -1

	for i, p := range calls {
		if p == "/notification" && notifyAt < 0 {
			notifyAt = i
		}

		if p == "/sources" && sourcesAt < 0 {
			sourcesAt = i
		}
	}

	if notifyAt < 0 || sourcesAt < 0 || notifyAt > sourcesAt {
		t.Errorf("calls = %v, want /notification before /sources", calls)
	}
}

// TestHandleRefreshLibraryServers_ReportsAnEmptyList covers the case the
// reporter of issue 580 sees: the speaker genuinely has no media server left.
// The refresh must answer with an empty list rather than an error, so the UI
// can say so and point at "Find servers".
func TestHandleRefreshLibraryServers_ReportsAnEmptyList(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, map[string]string{"/sources": cannedSourcesWithoutLibrary})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/library/servers/refresh", nil)
	req = withChiParams(req, map[string]string{"id": "lib-device"})
	w := httptest.NewRecorder()

	app.HandleRefreshLibraryServers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Servers []any `json:"servers"`
		} `json:"data"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success || len(resp.Data.Servers) != 0 {
		t.Errorf("got success=%v servers=%v, want true and an empty list", resp.Success, resp.Data.Servers)
	}
}

// TestHandleRefreshLibraryServers_UnknownDevice keeps the 404 distinct from an
// empty list: "no such speaker" and "this speaker has no media server" are
// different answers.
func TestHandleRefreshLibraryServers_UnknownDevice(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("POST", "/api/control/devices/nope/library/servers/refresh", nil)
	req = withChiParams(req, map[string]string{"id": "nope"})
	w := httptest.NewRecorder()

	app.HandleRefreshLibraryServers(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
