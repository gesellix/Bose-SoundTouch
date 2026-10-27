package soundtouchweb

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func TestRecentsInvalidationScopesHistoryToCurrentConnection(t *testing.T) {
	app := NewWebApp()
	master := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "MASTER"})
	member := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "MEMBER"})
	t.Cleanup(master.Close)
	t.Cleanup(member.Close)
	app.AddDevice("master", master)
	app.AddDevice("member", member)

	if !app.invalidateRecents("master", master, "MASTER") {
		t.Fatal("current master notification rejected")
	}
	if master.Status().RecentsRevision != 1 || member.Status().RecentsRevision != 0 {
		t.Fatal("invalidation crossed physical targets")
	}
	if app.invalidateRecents("master", master, "MEMBER") || app.invalidateRecents("missing", master, "") {
		t.Fatal("foreign notification accepted")
	}
	if !app.invalidateRecents("member", member, "") {
		t.Fatal("payload-free member notification rejected")
	}
	member.UpdateStatus(func(s *webtypes.DeviceStatus) { s.Volume = &models.Volume{ActualVolume: 12} })
	if member.Status().RecentsRevision != 1 || master.Status().RecentsRevision != 1 {
		t.Fatal("unrelated status changed Recents revision")
	}

	replacement := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "MASTER"})
	t.Cleanup(replacement.Close)
	app.devicesMu.Lock()
	app.devices["master"] = replacement
	app.devicesMu.Unlock()
	if app.invalidateRecents("master", master, "MASTER") || replacement.Status().RecentsRevision != 0 {
		t.Fatal("old connection invalidated replacement")
	}
	replacement.Close()
	if app.invalidateRecents("master", replacement, "") {
		t.Fatal("closed connection accepted")
	}
}

func TestRecentsInvalidationPublishesCurrentDeviceSnapshot(t *testing.T) {
	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "MASTER"})
	t.Cleanup(conn.Close)
	app.AddDevice("master", conn)
	remote, peer, cleanup := dialTestWebSocket(t, app)
	defer cleanup()
	t.Cleanup(func() { app.removeGlobalWebSocketClient(remote) })
	if err := app.registerGlobalWebSocket(remote); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	readRevision := func() uint64 {
		t.Helper()
		for {
			var frame struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := peer.ReadJSON(&frame); err != nil {
				t.Fatal(err)
			}
			if frame.Type != "devices" {
				continue
			}
			var devices map[string]struct {
				Status *webtypes.DeviceStatus `json:"status"`
			}
			if err := json.Unmarshal(frame.Data, &devices); err != nil {
				t.Fatal(err)
			}
			return devices["master"].Status.RecentsRevision
		}
	}
	if got := readRevision(); got != 0 {
		t.Fatalf("initial revision=%d", got)
	}
	if !app.invalidateRecents("master", conn, "") {
		t.Fatal("notification rejected")
	}
	if got := readRevision(); got != 1 {
		t.Fatalf("published revision=%d, want1", got)
	}
}

func TestRecentsInvalidationConcurrentStatusUpdates(t *testing.T) {
	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "MASTER"})
	t.Cleanup(conn.Close)
	app.AddDevice("master", conn)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for range 20 {
			app.invalidateRecents("master", conn, "")
		}
	}()
	go func() {
		defer workers.Done()
		for level := range 20 {
			conn.UpdateStatus(func(s *webtypes.DeviceStatus) { s.Volume = &models.Volume{ActualVolume: level} })
		}
	}()
	workers.Wait()
	if conn.Status().RecentsRevision != 20 || conn.Status().Volume.ActualVolume != 19 {
		t.Fatal("concurrent status update lost Recents or volume")
	}
}
