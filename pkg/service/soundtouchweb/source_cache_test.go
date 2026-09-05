package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// TestUpdateDeviceStatusDoesNotRefreshNowPlayingRevisionOnFailure: a poll round
// where /now_playing failed but /volume succeeded must merge the volume and
// leave now-playing authority untouched. A source selection waiting for
// confirmation reads NowPlayingRevision, so advancing it on a failed read
// would confirm a selection nothing actually verified.
func TestUpdateDeviceStatusDoesNotRefreshNowPlayingRevisionOnFailure(t *testing.T) {
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/volume" {
			_, _ = w.Write([]byte(`<volume><targetvolume>35</targetvolume><actualvolume>35</actualvolume><muteenabled>false</muteenabled></volume>`))

			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer speaker.Close()

	conn := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.URL}), nil)
	conn.SetStatus(&webtypes.DeviceStatus{NowPlaying: &models.NowPlaying{Source: "SPOTIFY"}})
	baseline := conn.Status()

	NewWebApp().UpdateDeviceStatus("speaker", conn)
	updated := conn.Status()

	if updated.Revision <= baseline.Revision || updated.Volume == nil || updated.Volume.ActualVolume != 35 {
		t.Fatalf("unrelated successful field was not merged: %+v", updated)
	}
	if updated.NowPlaying.Source != "SPOTIFY" || updated.NowPlayingRevision != baseline.NowPlayingRevision {
		t.Fatalf("failed now-playing read advanced source authority: %+v", updated)
	}
}

// TestUpdateDeviceStatusMarksSourcesStaleOnRepeatedFailedReads: unlike every
// other field, a failed /sources read is still recorded. The last known
// inventory stays visible but goes unusable once reads keep failing, because
// offering source buttons the speaker no longer confirms is worse than
// offering none.
func TestUpdateDeviceStatusMarksSourcesStaleOnRepeatedFailedReads(t *testing.T) {
	sourcesOK := true
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sources" {
			if !sourcesOK {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)

				return
			}

			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<sources><sourceItem source="AUX" sourceAccount="AUX1" status="READY" isLocal="true">Aux 1</sourceItem></sources>`))

			return
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<volume><targetvolume>35</targetvolume><actualvolume>35</actualvolume><muteenabled>false</muteenabled></volume>`))
	}))
	defer speaker.Close()

	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.URL}), nil)

	app.UpdateDeviceStatus("speaker", conn)

	fresh := conn.Status()
	if fresh.SourcesStale || fresh.Sources == nil || len(fresh.Sources.SourceItem) != 1 {
		t.Fatalf("successful source read was not merged as actionable: %+v", fresh)
	}

	// One failure is not enough: a single transient hiccup must not disable
	// the whole source list.
	sourcesOK = false
	app.UpdateDeviceStatus("speaker", conn)

	if single := conn.Status(); single.SourcesStale {
		t.Fatalf("one failed source read marked the inventory stale: %+v", single)
	}

	app.UpdateDeviceStatus("speaker", conn)

	stale := conn.Status()
	if !stale.SourcesStale {
		t.Fatalf("consecutive failed source reads did not mark the inventory stale: %+v", stale)
	}
	if stale.Sources != fresh.Sources {
		t.Fatalf("failed source read discarded the last known inventory: %+v", stale)
	}

	sourcesOK = true
	app.UpdateDeviceStatus("speaker", conn)

	if recovered := conn.Status(); recovered.SourcesStale {
		t.Fatalf("successful source read did not clear staleness: %+v", recovered)
	}
}

// TestHandleAPIDevicePublishesCanonicalReadback: the player's bounded readback
// after a source selection reads GET /devices/{id}, so that response must
// carry the same revisions the connection now holds -- not a snapshot taken
// before its own refresh.
func TestHandleAPIDevicePublishesCanonicalReadback(t *testing.T) {
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responses := map[string]string{
			"/now_playing": `<nowPlaying source="AUX" sourceAccount="AUX1"><track>Confirmed track</track><playStatus>PLAY_STATE</playStatus></nowPlaying>`,
			"/volume":      `<volume><targetvolume>35</targetvolume><actualvolume>35</actualvolume><muteenabled>false</muteenabled></volume>`,
			"/presets":     `<presets></presets>`,
			"/sources":     `<sources><sourceItem source="AUX" sourceAccount="AUX1" status="READY" isLocal="true">Aux 1</sourceItem></sources>`,
			"/bass":        `<bass><targetbass>0</targetbass><actualbass>0</actualbass></bass>`,
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	defer speaker.Close()

	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: speaker.URL}),
		&models.DeviceInfo{Name: "Speaker"},
	)
	conn.SetStatus(&webtypes.DeviceStatus{NowPlaying: &models.NowPlaying{Source: "STANDBY"}})
	baselineRevision := conn.Status().NowPlayingRevision
	conn.WebSocket = &client.WebSocketClient{}
	app.AddDevice("speaker", conn)

	req := httptest.NewRequest(http.MethodGet, "/api/control/devices/speaker", nil)
	req = withChiParams(req, map[string]string{"id": "speaker"})
	w := httptest.NewRecorder()
	app.HandleAPIDevice(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET device status = %d: %s", w.Code, w.Body.String())
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Status webtypes.DeviceStatus `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode device readback: %v", err)
	}

	canonical := conn.Status()
	if !response.Success || canonical.NowPlaying.Source != "AUX" || canonical.NowPlaying.Track != "Confirmed track" {
		t.Fatalf("canonical readback not merged: response=%+v status=%+v", response, canonical)
	}
	if response.Data.Status.NowPlaying.Source != canonical.NowPlaying.Source ||
		response.Data.Status.Revision != canonical.Revision ||
		response.Data.Status.NowPlayingRevision != canonical.NowPlayingRevision ||
		canonical.NowPlayingRevision <= baselineRevision {
		t.Fatalf("response did not publish canonical status: response=%+v status=%+v", response.Data.Status, canonical)
	}
}
