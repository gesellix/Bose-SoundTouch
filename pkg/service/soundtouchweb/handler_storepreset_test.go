package soundtouchweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/dlna/dlnatest"
)

// The /storePreset body shape below is not invented: it was measured against a
// SoundTouch 10 (FW 27.0.6) for issue 700. The speaker accepts an arbitrary
// ContentItem, so a Library row can be saved to a slot without playing it
// first, and it accepts a STORED_MUSIC folder whose ContentItem carries *no*
// type attribute at all, which is exactly how the speaker stores one itself.
// Recall then plays the folder from offset 0.

// TestHandleStorePresetContent_XMLShape asserts the ContentItem the handler
// posts to /storePreset: the named content, marked presetable, with the folder
// case carrying no type attribute.
func TestHandleStorePresetContent_XMLShape(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{
		"source":        "STORED_MUSIC",
		"sourceAccount": "uuid:test-udn/0",
		"location":      "1$7$0",
		"type":          "",
		"itemName":      "Some Album"
	}`)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/6", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": "6"})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	storeXML := captured["/storePreset"]
	if storeXML == "" {
		t.Fatal("speaker /storePreset was never called")
	}

	for _, want := range []string{
		`id="6"`,
		`source="STORED_MUSIC"`,
		`sourceAccount="uuid:test-udn/0"`,
		`location="1$7$0"`,
		`isPresetable="true"`,
		`<itemName>Some Album</itemName>`,
	} {
		if !strings.Contains(storeXML, want) {
			t.Errorf("storePreset XML should contain %q, got:\n%s", want, storeXML)
		}
	}

	// A container carries no type. models.ContentItem always marshals the
	// attribute, so it goes out empty rather than absent; the speaker accepted
	// that form for the folder above (issue 700 hardware run).
	if want := `type=""`; !strings.Contains(storeXML, want) {
		t.Errorf("storePreset XML should contain %q for a folder, got:\n%s", want, storeXML)
	}
}

// TestHandleStorePresetContent_KeepsTrackType checks the non-container case
// still carries the type, so a single track is recalled as a track.
func TestHandleStorePresetContent_KeepsTrackType(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{
		"source":        "STORED_MUSIC",
		"sourceAccount": "uuid:test-udn/0",
		"location":      "5:audio5:part13:3171:5 TRACK",
		"type":          "track",
		"itemName":      "Great Song"
	}`)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/2", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": "2"})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if want := `type="track"`; !strings.Contains(captured["/storePreset"], want) {
		t.Errorf("storePreset XML should contain %q, got:\n%s", want, captured["/storePreset"])
	}
}

// TestHandleStorePresetContent_DropsPlaceholderAccount covers the pattern
// where a speaker echoes the source name back as the account when there is no
// credential (see the SourceAccount placeholder fix, PR 376): storing that
// value would persist a fake account, so it is dropped.
func TestHandleStorePresetContent_DropsPlaceholderAccount(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := strings.NewReader(`{
		"source":        "TUNEIN",
		"sourceAccount": "TUNEIN",
		"location":      "/v1/playback/station/s12345",
		"itemName":      "Some Station"
	}`)

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/1", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": "1"})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if strings.Contains(captured["/storePreset"], `sourceAccount="TUNEIN"`) {
		t.Errorf("placeholder sourceAccount should be dropped, got:\n%s", captured["/storePreset"])
	}
}

// TestHandleStorePresetContent_Rejects verifies the input guards run before
// any speaker call: an out-of-range slot and the two required fields.
func TestHandleStorePresetContent_Rejects(t *testing.T) {
	tests := []struct {
		name string
		slot string
		body string
	}{
		{"slot zero", "0", `{"source":"STORED_MUSIC","location":"1$7$0"}`},
		{"slot seven", "7", `{"source":"STORED_MUSIC","location":"1$7$0"}`},
		{"slot not a number", "x", `{"source":"STORED_MUSIC","location":"1$7$0"}`},
		{"no source", "1", `{"location":"1$7$0"}`},
		{"no location", "1", `{"source":"STORED_MUSIC"}`},
		{"unparseable body", "1", `not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			speaker, captured := setupSpeakerMock(t, nil)
			defer speaker.Close()

			app := newLibraryTestApp(speaker.URL)

			req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/"+tt.slot, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = withChiParams(req, map[string]string{"id": "lib-device", "slot": tt.slot})
			w := httptest.NewRecorder()

			app.HandleStorePresetContent(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}

			// 400 must mean the command never reached the speaker: the player's
			// checkedReq treats 4xx as definitive proof of exactly that.
			if _, called := captured["/storePreset"]; called {
				t.Error("speaker /storePreset must not be called on a rejected request")
			}
		})
	}
}

// TestHandleStorePresetContent_UnknownDevice keeps the 404 distinct from the
// validation failures above.
func TestHandleStorePresetContent_UnknownDevice(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("POST", "/api/control/devices/nope/preset/1",
		strings.NewReader(`{"source":"STORED_MUSIC","location":"1$7$0"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "nope", "slot": "1"})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// The art lookup below exists because the speaker cannot answer the question:
// its /navigate response carries no artwork (verified on hardware), so a
// folder saved from the Library used to land in a preset slot with no art at
// all, while the same folder playing showed its cover, which the speaker
// resolves from the media server itself at play time.
//
// The mapping the lookup relies on was measured on a SoundTouch 10 (FW
// 27.0.6): a STORED_MUSIC location IS the DLNA object ID, except that a
// playable item's location carries a " TRACK" suffix.

// libraryArtTestApp wires a speaker whose /listMediaServers points at the
// given DLNA server, which is how the art lookup finds a ContentDirectory
// without an SSDP sweep.
func libraryArtTestApp(t *testing.T, udn, dlnaURL string) (*WebApp, map[string]string) {
	t.Helper()

	listing := `<?xml version="1.0" encoding="UTF-8" ?>` +
		`<ListMediaServersResponse>` +
		`<media_server id="` + strings.TrimPrefix(udn, "uuid:") + `" ip="192.0.2.10"` +
		` friendly_name="Test MediaServer" location="` + dlnaURL + `/rootDesc.xml" />` +
		`</ListMediaServersResponse>`

	speaker, captured := setupSpeakerMock(t, map[string]string{"/listMediaServers": listing})
	t.Cleanup(speaker.Close)

	return newLibraryTestApp(speaker.URL), captured
}

// storePresetContent posts one save request and fails the test unless the
// handler answered 200.
func storePresetContent(t *testing.T, app *WebApp, slot, body string) {
	t.Helper()

	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/"+slot, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": slot})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// artTree is a one-album tree whose container advertises its own cover, the
// case that matters here: MiniDLNA reports albumArtURI on every musicAlbum
// container, and that is the only artwork a folder has.
func artTree() *dlnatest.Tree {
	return &dlnatest.Tree{Containers: []*dlnatest.Container{{
		ID:         "1$6$7$2",
		ParentID:   "1$6$7",
		Title:      "Some Album",
		Class:      "object.container.album.musicAlbum",
		ArtPayload: []byte{0x89, 'P', 'N', 'G'},
		ArtMime:    "image/png",
		Children: []*dlnatest.Item{{
			ID:         "1$6$7$2$5",
			ParentID:   "1$6$7$2",
			Title:      "Great Song",
			Class:      "object.item.audioItem.musicTrack",
			MimeType:   "audio/mpeg",
			ArtPayload: []byte{0x89, 'P', 'N', 'G'},
			ArtMime:    "image/jpeg",
		}},
	}}}
}

// TestHandleStorePresetContent_FillsFolderArtFromTheMediaServer stores a
// folder and expects the container's own cover to be saved with it.
func TestHandleStorePresetContent_FillsFolderArtFromTheMediaServer(t *testing.T) {
	const udn = "uuid:4d696e69-444c-164e-9d41-2497ed3da6ad"

	media, _ := dlnatest.NewHTTPTest(dlnatest.WithUDN(udn), dlnatest.WithTree(artTree()))
	defer media.Close()

	app, captured := libraryArtTestApp(t, udn, media.URL)

	storePresetContent(t, app, "6", `{
		"source":        "STORED_MUSIC",
		"sourceAccount": "4d696e69-444c-164e-9d41-2497ed3da6ad/0",
		"location":      "1$6$7$2",
		"type":          "",
		"itemName":      "Some Album"
	}`)

	if want := `<containerArt>` + media.URL + `/AlbumArt/1%246%247%242.png</containerArt>`; !strings.Contains(captured["/storePreset"], want) {
		t.Errorf("storePreset XML should contain %q, got:\n%s", want, captured["/storePreset"])
	}
}

// TestHandleStorePresetContent_FillsTrackArtFromTheMediaServer covers the
// " TRACK" suffix: the speaker's location for a playable item is the object ID
// plus that word, so the lookup has to strip it to ask about the right object.
func TestHandleStorePresetContent_FillsTrackArtFromTheMediaServer(t *testing.T) {
	const udn = "uuid:4d696e69-444c-164e-9d41-2497ed3da6ad"

	media, _ := dlnatest.NewHTTPTest(dlnatest.WithUDN(udn), dlnatest.WithTree(artTree()))
	defer media.Close()

	app, captured := libraryArtTestApp(t, udn, media.URL)

	storePresetContent(t, app, "5", `{
		"source":        "STORED_MUSIC",
		"sourceAccount": "4d696e69-444c-164e-9d41-2497ed3da6ad/0",
		"location":      "1$6$7$2$5 TRACK",
		"type":          "track",
		"itemName":      "Great Song"
	}`)

	if want := `/AlbumArt/1%246%247%242%245.jpg</containerArt>`; !strings.Contains(captured["/storePreset"], want) {
		t.Errorf("storePreset XML should contain the track's own art %q, got:\n%s", want, captured["/storePreset"])
	}
}

// TestHandleStorePresetContent_KeepsGivenArt leaves a caller-supplied
// containerArt alone: whoever already knows the artwork (a provider result,
// say) is a better source than our own lookup.
func TestHandleStorePresetContent_KeepsGivenArt(t *testing.T) {
	const udn = "uuid:4d696e69-444c-164e-9d41-2497ed3da6ad"

	media, _ := dlnatest.NewHTTPTest(dlnatest.WithUDN(udn), dlnatest.WithTree(artTree()))
	defer media.Close()

	app, captured := libraryArtTestApp(t, udn, media.URL)

	storePresetContent(t, app, "6", `{
		"source":        "STORED_MUSIC",
		"sourceAccount": "4d696e69-444c-164e-9d41-2497ed3da6ad/0",
		"location":      "1$6$7$2",
		"containerArt":  "http://192.0.2.20/cover.jpg",
		"itemName":      "Some Album"
	}`)

	if want := `<containerArt>http://192.0.2.20/cover.jpg</containerArt>`; !strings.Contains(captured["/storePreset"], want) {
		t.Errorf("storePreset XML should keep the given art %q, got:\n%s", want, captured["/storePreset"])
	}
}

// TestHandleStorePresetContent_StoresWithoutArtWhenLookupFails is the
// failsafe: artwork is decoration, so an unknown server (or any other lookup
// failure) must still store the preset.
func TestHandleStorePresetContent_StoresWithoutArtWhenLookupFails(t *testing.T) {
	media, _ := dlnatest.NewHTTPTest(dlnatest.WithUDN("uuid:some-other-server"), dlnatest.WithTree(artTree()))
	defer media.Close()

	app, captured := libraryArtTestApp(t, "uuid:some-other-server", media.URL)

	storePresetContent(t, app, "6", `{
		"source":        "STORED_MUSIC",
		"sourceAccount": "4d696e69-444c-164e-9d41-2497ed3da6ad/0",
		"location":      "1$6$7$2",
		"itemName":      "Some Album"
	}`)

	if storeXML := captured["/storePreset"]; !strings.Contains(storeXML, `location="1$6$7$2"`) {
		t.Errorf("the preset must be stored even without art, got:\n%s", storeXML)
	}

	if strings.Contains(captured["/storePreset"], "containerArt") {
		t.Errorf("no art was resolvable, so none should be stored, got:\n%s", captured["/storePreset"])
	}
}

// TestStoredMusicArtLookupResolvesTheServerOnce pins the cache: repeated saves
// must not re-resolve the media server, which costs the speaker a
// /listMediaServers call and the server a description fetch every time.
func TestStoredMusicArtLookupResolvesTheServerOnce(t *testing.T) {
	const udn = "uuid:4d696e69-444c-164e-9d41-2497ed3da6ad"

	media, _ := dlnatest.NewHTTPTest(dlnatest.WithUDN(udn), dlnatest.WithTree(artTree()))
	defer media.Close()

	var mu sync.Mutex
	listings := 0

	listing := `<?xml version="1.0" encoding="UTF-8" ?>` +
		`<ListMediaServersResponse>` +
		`<media_server id="4d696e69-444c-164e-9d41-2497ed3da6ad" location="` + media.URL + `/rootDesc.xml" />` +
		`</ListMediaServersResponse>`

	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/listMediaServers" {
			mu.Lock()
			listings++
			mu.Unlock()

			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(listing))

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := `{
		"source":        "STORED_MUSIC",
		"sourceAccount": "4d696e69-444c-164e-9d41-2497ed3da6ad/0",
		"location":      "1$6$7$2",
		"itemName":      "Some Album"
	}`

	storePresetContent(t, app, "6", body)
	storePresetContent(t, app, "5", body)

	mu.Lock()
	defer mu.Unlock()

	if listings != 1 {
		t.Errorf("listMediaServers calls = %d, want 1 (the resolved server is cached)", listings)
	}
}
