//go:build browsertest

package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

func TestOpenZoneDetailTracksExternalTopologyChanges(t *testing.T) {
	const (
		masterID = "192.0.2.10"
		memberID = "192.0.2.11"
	)
	zone := &models.ZoneInfo{
		Master: "master-device",
		Members: []models.Member{
			{DeviceID: "master-device", IP: masterID},
			{DeviceID: "member-device", IP: memberID},
		},
	}
	app := NewWebApp()
	for _, entry := range []DeviceEntry{
		projectionDeviceWithZone(masterID, "master-device", "Atrium", true, nil, zone),
		projectionDeviceWithZone(memberID, "member-device", "Breakfast Room", true, nil, nil),
	} {
		app.AddDevice(entry.ID, entry.Device)
	}

	router := chi.NewRouter()
	app.MountWeb(router, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		switch {
		case r.Method == http.MethodGet && path == "/api/control/devices/"+masterID+"/zone":
			writeLogicalZoneBrowserState(w, app, masterID)
		case r.Method == http.MethodGet && path == "/api/control/devices/"+masterID+"/zone/candidates":
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{
				Success: true,
				Data:    map[string]interface{}{},
			})
		default:
			router.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(".zone-card", chromedp.ByQuery),
		chromedp.Click(".zone-card", chromedp.ByQuery),
		chromedp.WaitVisible(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Click(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('.zone-logical-member').length === 2 &&
			document.querySelectorAll('.zone-logical-member [role="status"][aria-label]').length === 2`, nil),
	); err != nil {
		t.Fatalf("open logical zone detail: %v", err)
	}

	master, _ := app.GetDevice(masterID)
	generation := master.BeginZoneRefresh()
	if !master.ApplyPolledZone(generation, "master-device", &models.ZoneInfo{
		Master:  "master-device",
		Members: []models.Member{{DeviceID: "master-device", IP: masterID}},
	}) {
		t.Fatal("external dissolution did not change cached topology")
	}
	app.BroadcastDeviceList()

	dissolveStarted := time.Now()
	if err := chromedp.Run(ctx, chromedp.Poll(`
		document.querySelector('.zone-status-label')?.textContent.trim() === 'Standalone' &&
		!document.querySelector('.zone-member-details') &&
		!Array.from(document.querySelectorAll('button')).some(button =>
			button.textContent.trim() === 'Dissolve zone')`, nil)); err != nil {
		t.Fatalf("external dissolution did not update open detail: %v", err)
	}
	if elapsed := time.Since(dissolveStarted); elapsed > 5*time.Second {
		t.Fatalf("external dissolution took %s; periodic fallback masked the event path", elapsed)
	}

	generation = master.BeginZoneRefresh()
	if !master.ApplyPolledZone(generation, "master-device", zone) {
		t.Fatal("external creation did not change cached topology")
	}
	app.BroadcastDeviceList()

	createStarted := time.Now()
	if err := chromedp.Run(ctx, chromedp.Poll(`
		document.querySelector('.zone-member-details > summary') &&
		Array.from(document.querySelectorAll('button')).some(button =>
			button.textContent.trim() === 'Dissolve zone')`, nil)); err != nil {
		t.Fatalf("external creation did not update open detail: %v", err)
	}
	if elapsed := time.Since(createStarted); elapsed > 5*time.Second {
		t.Fatalf("external creation took %s; periodic fallback masked the event path", elapsed)
	}
}

func writeLogicalZoneBrowserState(w http.ResponseWriter, app *WebApp, masterID string) {
	w.Header().Set("Content-Type", "application/json")
	projection := app.deviceViewSnapshot()[masterID].Zone
	if projection == nil {
		_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]interface{}{
			"members":      []interface{}{},
			"isMaster":     false,
			"isSlave":      false,
			"isStandalone": true,
		}})
		return
	}

	master := projection.Members[0]
	_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]interface{}{
		"masterIp":            projection.MasterControlID,
		"masterHwId":          projection.MasterDeviceID,
		"masterName":          master.Name,
		"master":              master,
		"members":             projection.Members[1:],
		"physicalMemberCount": projection.PhysicalMemberCount,
		"isMaster":            true,
		"isSlave":             false,
		"isStandalone":        false,
	}})
}
