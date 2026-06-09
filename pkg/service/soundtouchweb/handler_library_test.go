package soundtouchweb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// cannedNavigateResponse is a minimal XML navigateResponse the fake speaker
// returns for /navigate in browse tests. It contains one directory and one
// track so we can assert both are mapped correctly.
// Note: totalItems is an XML element, not an attribute, per models.NavigateResponse.
const cannedNavigateResponse = `<?xml version="1.0" encoding="UTF-8" ?>
<navigateResponse source="STORED_MUSIC" sourceAccount="uuid:test-udn/0">
  <totalItems>2</totalItems>
  <items>
    <item Playable="0">
      <name>Albums</name>
      <type>dir</type>
      <ContentItem source="STORED_MUSIC" type="dir" location="4:cont2:150:0:0:" sourceAccount="uuid:test-udn/0" isPresetable="false">
        <itemName>Albums</itemName>
      </ContentItem>
    </item>
    <item Playable="1">
      <name>Great Song</name>
      <type>track</type>
      <ContentItem source="STORED_MUSIC" type="track" location="5:audio5:part13:3171:5 TRACK" sourceAccount="uuid:test-udn/0" isPresetable="true">
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

	if dir.Playable {
		t.Error("dir.Playable should be false")
	}

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
