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
