package soundtouchweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The clear goes to the speaker's own /removePreset, so the speaker reports
// the change back to AfterTouch itself -- the same path the store uses, and
// the one the preset sharing hangs off (issue 495).
func TestHandleRemovePresetCallsTheSpeaker(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	req := httptest.NewRequest("DELETE", "/api/control/devices/lib-device/preset/4", nil)
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": "4"})
	w := httptest.NewRecorder()

	app.HandleRemovePreset(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body, ok := captured["/removePreset"]
	if !ok {
		t.Fatal("speaker /removePreset was never called")
	}

	if !strings.Contains(body, `id="4"`) {
		t.Errorf("expected the requested slot in the body, got %q", body)
	}
}

func TestHandleRemovePresetRejectsSlotsOutsideOneToSix(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	for _, slot := range []string{"0", "7", "x"} {
		req := httptest.NewRequest("DELETE", "/api/control/devices/lib-device/preset/"+slot, nil)
		req = withChiParams(req, map[string]string{"id": "lib-device", "slot": slot})
		w := httptest.NewRecorder()

		app.HandleRemovePreset(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("slot %q: expected 400, got %d", slot, w.Code)
		}
	}

	if _, called := captured["/removePreset"]; called {
		t.Error("an invalid slot must never reach the speaker")
	}
}
