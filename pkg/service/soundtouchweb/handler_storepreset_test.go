package soundtouchweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
