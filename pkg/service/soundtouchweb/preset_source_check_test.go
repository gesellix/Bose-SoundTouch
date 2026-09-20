package soundtouchweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func sourceList(items ...models.SourceItem) *models.Sources {
	return &models.Sources{SourceItem: items}
}

func TestDescribeMissingSource(t *testing.T) {
	speaker := sourceList(
		models.SourceItem{Source: "TUNEIN", Status: models.SourceStatusReady},
		models.SourceItem{Source: "SPOTIFY", SourceAccount: "listener", Status: models.SourceStatusReady},
		models.SourceItem{
			Source: "STORED_MUSIC", SourceAccount: "uuid:aaaa/0",
			DisplayName: "MiniDLNA", Status: models.SourceStatusReady,
		},
	)

	tests := []struct {
		name    string
		source  string
		account string
		want    string
	}{
		{name: "source and account present", source: "SPOTIFY", account: "listener"},
		{name: "source present, no account needed", source: "TUNEIN"},
		// The speaker echoes the source name back as sourceAccount in recents;
		// treating that as an account would demand a credential that never
		// existed, and refuse a station the speaker plays happily.
		{name: "placeholder account", source: "TUNEIN", account: "TUNEIN"},
		{name: "case and spacing", source: "spotify", account: " listener "},
		{name: "library account present", source: "STORED_MUSIC", account: "uuid:aaaa/0"},

		{name: "source absent", source: "DEEZER", want: "no Deezer source"},
		{
			name: "account absent", source: "SPOTIFY", account: "someone-else",
			want: `no Spotify account "someone-else"`,
		},
		// The usual cause is a media server set up on another speaker, so the
		// refusal names what this one does have.
		{
			name:   "library account absent names what it has",
			source: "STORED_MUSIC", account: "uuid:bbbb/0",
			want: "It has: MiniDLNA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeMissingSource(speaker, tt.source, tt.account)

			if tt.want == "" {
				if got != "" {
					t.Fatalf("expected no complaint, got %q", got)
				}

				return
			}

			if !strings.Contains(got, tt.want) {
				t.Fatalf("got %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// Absent evidence is not evidence of absence: a speaker we could not ask, or
// one that reported nothing, must not have a preset refused on its behalf.
func TestDescribeMissingSourceNeverRefusesWithoutEvidence(t *testing.T) {
	for _, sources := range []*models.Sources{nil, {}, sourceList()} {
		if got := describeMissingSource(sources, "SPOTIFY", "listener"); got != "" {
			t.Errorf("refused with no source list: %q", got)
		}
	}
}

const speakerSourcesXML = `<sources deviceID="AABBCCDDEEFF">
	<sourceItem source="TUNEIN" status="READY">TuneIn</sourceItem>
	<sourceItem source="STORED_MUSIC" sourceAccount="uuid:live/0" status="READY">Late Arrival</sourceItem>
</sources>`

func TestStorePresetRefusesASourceTheSpeakerDoesNotHave(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, map[string]string{"/sources": speakerSourcesXML})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	body := `{"source":"STORED_MUSIC","sourceAccount":"uuid:other/0","location":"1$7$0","itemName":"Album on another speaker"}`
	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/4", strings.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": "4"})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "Late Arrival") {
		t.Errorf("the refusal should name what the speaker does have, got %s", w.Body.String())
	}

	if _, stored := captured["/storePreset"]; stored {
		t.Error("a preset the speaker cannot play must never reach it")
	}
}

// A cached source list can be minutes old. A service linked in the meantime
// would be reported as missing, so a failure against the cache is re-checked
// against the speaker before anything is refused.
func TestStorePresetRechecksTheSpeakerBeforeRefusing(t *testing.T) {
	speaker, captured := setupSpeakerMock(t, map[string]string{"/sources": speakerSourcesXML})
	defer speaker.Close()

	app := newLibraryTestApp(speaker.URL)

	device, _ := app.GetDevice("lib-device")
	device.SetStatus(&webtypes.DeviceStatus{
		IsConnected: true,
		// Stale: taken before the media server was added.
		Sources: sourceList(models.SourceItem{Source: "TUNEIN", Status: models.SourceStatusReady}),
	})

	body := `{"source":"STORED_MUSIC","sourceAccount":"uuid:live/0","location":"1$7$0","itemName":"Just added"}`
	req := httptest.NewRequest("POST", "/api/control/devices/lib-device/preset/4", strings.NewReader(body))
	req = withChiParams(req, map[string]string{"id": "lib-device", "slot": "4"})
	w := httptest.NewRecorder()

	app.HandleStorePresetContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected the live list to allow the store, got %d: %s", w.Code, w.Body.String())
	}

	if _, stored := captured["/storePreset"]; !stored {
		t.Error("speaker /storePreset was never called")
	}
}
