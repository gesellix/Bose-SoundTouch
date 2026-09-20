package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func decodeSourcesElsewhere(t *testing.T, w *httptest.ResponseRecorder) SourcesElsewherePayload {
	t.Helper()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data SourcesElsewherePayload `json:"data"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	return resp.Data
}

func TestHandleSourcesElsewhere(t *testing.T) {
	app := speakerWithPresetsOn(t, 0, "ACCOUNT01")

	var askedDevice, askedAccount string

	app.SourcesElsewhere = func(deviceID, account string) ([]models.SourceIdentity, error) {
		askedDevice, askedAccount = deviceID, account

		return []models.SourceIdentity{
			{
				Type: "STORED_MUSIC", Account: "uuid:theirs/0", DisplayName: "fritz",
				Availability: models.SourceAvailableToAdd, Devices: []string{"DEVICEID02"},
			},
			{
				Type: "SPOTIFY", Account: "listener", DisplayName: "Spotify",
				Availability: models.SourceAvailableByLinking, Devices: []string{"DEVICEID02"},
			},
		}, nil
	}

	w := httptest.NewRecorder()
	app.HandleSourcesElsewhere(w, storedPresetsRequest("GET", "/sources-elsewhere", ""))

	payload := decodeSourcesElsewhere(t, w)

	if !payload.Available || len(payload.Sources) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	// The same targeting as the stored list: the speaker's own device id, and
	// the account it reports, so a leftover directory is not read instead.
	if askedDevice != "DEVICEID01" || askedAccount != "ACCOUNT01" {
		t.Errorf("asked for device %q account %q", askedDevice, askedAccount)
	}

	// The response is a projection. Nothing credential-shaped may appear in
	// it, whatever the stored source carried (issue 663: this API is
	// unauthenticated).
	body := strings.ToLower(w.Body.String())
	for _, forbidden := range []string{"secret", "credential", "\"bs-"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response contains %q: %s", forbidden, w.Body.String())
		}
	}
}

func TestHandleSourcesElsewhereWithoutAService(t *testing.T) {
	app := speakerWithPresetsOn(t, 0, "")

	w := httptest.NewRecorder()
	app.HandleSourcesElsewhere(w, storedPresetsRequest("GET", "/sources-elsewhere", ""))

	payload := decodeSourcesElsewhere(t, w)
	if payload.Available {
		t.Error("expected available=false with no service behind the player")
	}

	if payload.Sources == nil {
		t.Error("sources must serialise as [] rather than null")
	}
}

// Adding a media server goes to the speaker, because that is where a DLNA
// account lives; the Library page and this path are the same call.
func TestAddSourceElsewhereRegistersAMediaServerOnTheSpeaker(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, map[string]string{
		"/setMusicServiceAccount": `<status>/setMusicServiceAccount</status>`,
	})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	w := httptest.NewRecorder()
	app.HandleAddSourceElsewhere(w, libraryDeviceRequest(
		`{"type":"STORED_MUSIC","account":"uuid:theirs/0","name":"fritz"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, called := captured["/setMusicServiceAccount"]; !called {
		t.Fatalf("the speaker was never asked to register the server: %v", keysOf(captured))
	}
}

// TuneIn and its siblings are entries in the list AfterTouch serves, so they
// are added service-side, from the service's own definition.
func TestAddSourceElsewhereAddsAMintedSourceThroughTheService(t *testing.T) {
	speaker, _ := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	var askedType string

	app.AddCanonicalSource = func(_, _, sourceType string) (bool, error) {
		askedType = sourceType

		return true, nil
	}

	w := httptest.NewRecorder()
	app.HandleAddSourceElsewhere(w, libraryDeviceRequest(`{"type":"TUNEIN"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if askedType != "TUNEIN" {
		t.Errorf("service asked for %q", askedType)
	}
}

// No amount of service-side work can link an account the target speaker has
// to link itself, so the refusal says that rather than failing obscurely.
func TestAddSourceElsewhereRefusesWhatItCannotCarryAcross(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, nil)
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)
	app.AddCanonicalSource = func(string, string, string) (bool, error) {
		t.Error("a music service must never reach the canonical add")

		return false, nil
	}

	for _, tt := range []struct{ body, want string }{
		{`{"type":"SPOTIFY","account":"listener"}`, "linked on this speaker itself"},
		{`{"type":"AUX"}`, "device-local"},
	} {
		w := httptest.NewRecorder()
		app.HandleAddSourceElsewhere(w, libraryDeviceRequest(tt.body))

		if w.Code != http.StatusConflict {
			t.Fatalf("%s: expected 409, got %d: %s", tt.body, w.Code, w.Body.String())
		}

		if !strings.Contains(w.Body.String(), tt.want) {
			t.Errorf("%s: expected the reason %q, got %s", tt.body, tt.want, w.Body.String())
		}
	}

	if _, called := captured["/setMusicServiceAccount"]; called {
		t.Error("a refused source must never reach the speaker")
	}
}

func libraryDeviceRequest(body string) *http.Request {
	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/sources-elsewhere/add", strings.NewReader(body))

	return withChiParams(req, map[string]string{"id": "lib-device"})
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
