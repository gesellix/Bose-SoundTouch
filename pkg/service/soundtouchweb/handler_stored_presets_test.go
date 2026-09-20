package soundtouchweb

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func decodeStoredPresets(t *testing.T, w *httptest.ResponseRecorder) StoredPresetsPayload {
	t.Helper()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data StoredPresetsPayload `json:"data"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	return resp.Data
}

// speakerWithPresets registers a device reporting the given number of filled
// preset buttons, which is the half of the comparison the speaker owns.
func speakerWithPresets(t *testing.T, filled int) *WebApp {
	t.Helper()

	return speakerWithPresetsOn(t, filled, "")
}

func speakerWithPresetsOn(t *testing.T, filled int, margeAccount string) *WebApp {
	t.Helper()

	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
		DeviceID: "DEVICEID01", Name: "Speaker", MargeAccountUUID: margeAccount,
	})

	presets := &models.Presets{}
	for i := 1; i <= filled; i++ {
		presets.Preset = append(presets.Preset, models.Preset{ID: i, ContentItem: &models.ContentItem{Source: "TUNEIN"}})
	}

	conn.SetStatus(&webtypes.DeviceStatus{IsConnected: true, Presets: presets})
	app.AddDevice("speaker", conn)

	return app
}

func storedPresetsRequest(method, path string, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))

	return withChiParams(req, map[string]string{"id": "speaker"})
}

// The issue 697 shape: eight stored rows, six of them on real buttons, and a
// speaker reporting six.
func TestHandleStoredPresetsReportsTheDisagreement(t *testing.T) {
	app := speakerWithPresets(t, 6)

	var askedFor string

	app.StoredPresets = func(deviceID, _ string) ([]models.StoredPresetRow, error) {
		askedFor = deviceID

		return []models.StoredPresetRow{
			{Index: 0, Button: "1", Slot: 1, Verdict: models.StoredPresetOK},
			{Index: 1, Button: "7", Slot: 7, Verdict: models.StoredPresetOutOfRange},
			{Index: 2, Button: "", Verdict: models.StoredPresetNoSlot},
		}, nil
	}

	w := httptest.NewRecorder()
	app.HandleStoredPresets(w, storedPresetsRequest("GET", "/stored-presets/", ""))

	payload := decodeStoredPresets(t, w)

	// The datastore files a speaker under the id it reports, not under the key
	// the player happens to have registered it as.
	if askedFor != "DEVICEID01" {
		t.Errorf("asked the service for %q, want the speaker's own device id", askedFor)
	}

	if !payload.Available || !payload.Disagrees {
		t.Errorf("expected an available, disagreeing payload, got %+v", payload)
	}

	if payload.Unrecallable != 2 {
		t.Errorf("Unrecallable = %d, want the two rows on no real button", payload.Unrecallable)
	}

	if payload.SpeakerCount != 6 {
		t.Errorf("SpeakerCount = %d, want what the speaker reports", payload.SpeakerCount)
	}
}

// A healthy install must not be told anything is wrong, or the warning stops
// meaning anything.
func TestHandleStoredPresetsStaysQuietWhenTheListsAgree(t *testing.T) {
	app := speakerWithPresets(t, 2)
	app.StoredPresets = func(string, string) ([]models.StoredPresetRow, error) {
		return []models.StoredPresetRow{
			{Index: 0, Button: "1", Slot: 1, Verdict: models.StoredPresetOK},
			{Index: 1, Button: "2", Slot: 2, Verdict: models.StoredPresetOK},
		}, nil
	}

	w := httptest.NewRecorder()
	app.HandleStoredPresets(w, storedPresetsRequest("GET", "/stored-presets/", ""))

	if payload := decodeStoredPresets(t, w); payload.Disagrees {
		t.Errorf("expected no disagreement, got %+v", payload)
	}
}

// Every row valid on its own, but more of them than the speaker has: this is
// the case a sync refuses to shrink, so it counts as a disagreement too.
func TestHandleStoredPresetsFlagsASurplusOfValidRows(t *testing.T) {
	app := speakerWithPresets(t, 1)
	app.StoredPresets = func(string, string) ([]models.StoredPresetRow, error) {
		return []models.StoredPresetRow{
			{Index: 0, Button: "1", Slot: 1, Verdict: models.StoredPresetOK},
			{Index: 1, Button: "2", Slot: 2, Verdict: models.StoredPresetOK},
		}, nil
	}

	w := httptest.NewRecorder()
	app.HandleStoredPresets(w, storedPresetsRequest("GET", "/stored-presets/", ""))

	payload := decodeStoredPresets(t, w)
	if !payload.Disagrees {
		t.Error("a stored list longer than the speaker's must count as a disagreement")
	}

	if payload.Unrecallable != 0 {
		t.Errorf("Unrecallable = %d, want 0: every row is on a real button", payload.Unrecallable)
	}
}

func TestHandleStoredPresetsWithoutAServiceReportsUnavailable(t *testing.T) {
	app := speakerWithPresets(t, 6)

	w := httptest.NewRecorder()
	app.HandleStoredPresets(w, storedPresetsRequest("GET", "/stored-presets/", ""))

	payload := decodeStoredPresets(t, w)
	if payload.Available || payload.Disagrees {
		t.Errorf("expected an unavailable, quiet payload, got %+v", payload)
	}

	if payload.Rows == nil {
		t.Error("rows must serialise as [] rather than null")
	}
}

func TestHandleRepairStoredPresetsPassesTheRowsAndTheGuard(t *testing.T) {
	app := speakerWithPresets(t, 1)
	app.StoredPresets = func(string, string) ([]models.StoredPresetRow, error) {
		return []models.StoredPresetRow{{Index: 0, Button: "1", Slot: 1, Verdict: models.StoredPresetOK}}, nil
	}

	var gotDrop []int
	var gotExpected int

	app.RepairStoredPresets = func(_, _ string, drop []int, expected int) ([]models.StoredPresetRow, error) {
		gotDrop, gotExpected = drop, expected

		return []models.StoredPresetRow{{Index: 0, Button: "1", Slot: 1, Verdict: models.StoredPresetOK}}, nil
	}

	w := httptest.NewRecorder()
	app.HandleRepairStoredPresets(w, storedPresetsRequest("POST", "/stored-presets/repair", `{"drop":[1,2],"expected":3}`))

	payload := decodeStoredPresets(t, w)
	if payload.Disagrees {
		t.Errorf("expected the repaired list to agree, got %+v", payload)
	}

	if len(gotDrop) != 2 || gotDrop[0] != 1 || gotDrop[1] != 2 || gotExpected != 3 {
		t.Errorf("repair called with drop=%v expected=%d", gotDrop, gotExpected)
	}
}

// A stale view is the caller's to resolve, not a service failure: the player
// reloads and offers the repair again.
func TestHandleRepairStoredPresetsReportsARefusalAsAConflict(t *testing.T) {
	app := speakerWithPresets(t, 1)
	app.RepairStoredPresets = func(string, string, []int, int) ([]models.StoredPresetRow, error) {
		return nil, errors.New("the stored list now holds 6 rows, not 8; reload and try again")
	}

	w := httptest.NewRecorder()
	app.HandleRepairStoredPresets(w, storedPresetsRequest("POST", "/stored-presets/repair", `{"drop":[7],"expected":8}`))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "reload") {
		t.Errorf("the refusal should reach the caller verbatim, got %s", w.Body.String())
	}
}

func TestHandleRepairStoredPresetsRejectsAnEmptyRequest(t *testing.T) {
	app := speakerWithPresets(t, 1)

	var called bool

	app.RepairStoredPresets = func(string, string, []int, int) ([]models.StoredPresetRow, error) {
		called = true

		return nil, nil
	}

	w := httptest.NewRecorder()
	app.HandleRepairStoredPresets(w, storedPresetsRequest("POST", "/stored-presets/repair", `{"drop":[],"expected":8}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	if called {
		t.Error("a repair naming no rows must never reach the service")
	}
}

// A speaker re-paired at some point keeps its old directory, and reading that
// one reports a disagreement that says more about the leftover than about the
// speaker -- and a repair would edit a file nothing is being served from. The
// speaker's own margeAccountUUID is the signal that settles it, and the player
// already holds it.
func TestStoredPresetsAreReadUnderTheAccountTheSpeakerReports(t *testing.T) {
	app := speakerWithPresetsOn(t, 6, "6919733")

	var askedAccount string

	app.StoredPresets = func(_, account string) ([]models.StoredPresetRow, error) {
		askedAccount = account

		return nil, nil
	}

	w := httptest.NewRecorder()
	app.HandleStoredPresets(w, storedPresetsRequest("GET", "/stored-presets/", ""))
	decodeStoredPresets(t, w)

	if askedAccount != "6919733" {
		t.Errorf("asked under account %q, want the one the speaker reports", askedAccount)
	}

	var repairedAccount string

	app.RepairStoredPresets = func(_, account string, _ []int, _ int) ([]models.StoredPresetRow, error) {
		repairedAccount = account

		return nil, nil
	}

	w = httptest.NewRecorder()
	app.HandleRepairStoredPresets(w, storedPresetsRequest("POST", "/stored-presets/repair", `{"drop":[7],"expected":8}`))

	if repairedAccount != "6919733" {
		t.Errorf("repaired under account %q, want the same account the view was read from", repairedAccount)
	}
}

// A speaker that reports no account at all (or one we hold nothing for) leaves
// the choice to the service rather than failing.
func TestStoredPresetsFallBackWhenTheSpeakerNamesNoAccount(t *testing.T) {
	app := speakerWithPresetsOn(t, 6, "")

	var asked bool

	app.StoredPresets = func(_, account string) ([]models.StoredPresetRow, error) {
		asked = true

		if account != "" {
			t.Errorf("account = %q, want it left to the service", account)
		}

		return nil, nil
	}

	w := httptest.NewRecorder()
	app.HandleStoredPresets(w, storedPresetsRequest("GET", "/stored-presets/", ""))
	decodeStoredPresets(t, w)

	if !asked {
		t.Error("expected the service to be asked anyway")
	}
}
