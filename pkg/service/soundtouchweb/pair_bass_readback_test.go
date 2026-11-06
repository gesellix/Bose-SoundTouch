package soundtouchweb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

func TestPairDetailRefreshesOnlyHiddenPeerBassAndRetainsFailedReadEvidence(t *testing.T) {
	var reads atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bass" {
			t.Errorf("unexpected peer endpoint %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		reads.Add(1)
		if fail.Load() {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`<bass deviceID="right-id"><targetbass>0</targetbass><actualbass>0</actualbass></bass>`))
	}))
	defer server.Close()
	group := testStereoGroup()
	left := projectionDevice("left.local", "left-id", "Left", true, group)
	right := projectionDevice("right.local", "right-id", "Right", true, group)
	right.Device.Client = client.NewClient(&client.Config{Host: server.URL})
	right.Device.ApplyFieldEvent(webtypes.FieldBass, func(s *webtypes.DeviceStatus) {
		s.Bass = &models.Bass{DeviceID: "right-id", ActualBass: 0}
	})
	app := NewWebApp()
	cleanupRegistry(t, app)
	app.AddDevice(left.ID, left.Device)
	app.AddDevice(right.ID, right.Device)
	request := func() deviceView {
		t.Helper()
		route := chi.NewRouteContext()
		route.URLParams.Add("id", left.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/control/devices/left.local", nil)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
		recorder := httptest.NewRecorder()
		app.HandleAPIDevice(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var result struct {
			Data deviceView `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	before := right.Device.Status().BassRevision
	view := request()
	if reads.Load() != 1 || right.Device.Status().BassRevision <= before {
		t.Fatal("unchanged peer value did not receive a fresh authoritative observation")
	}
	if len(view.SettingsTargets) != 2 || view.SettingsTargets[1].Status.BassRevision != right.Device.Status().BassRevision {
		t.Fatal("logical detail did not publish fresh hidden peer evidence")
	}
	before = right.Device.Status().BassRevision
	fail.Store(true)
	request()
	if reads.Load() != 2 || right.Device.Status().BassRevision != before {
		t.Fatal("a failed peer read advanced bass evidence")
	}
}
