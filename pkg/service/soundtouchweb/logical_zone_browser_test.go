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
	if err := chromedp.Do(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(chromedp.CSS(".zone-card")),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('.device-card').length === 1 &&
			document.querySelector('.zone-card .device-name')?.textContent.trim() === 'Atrium' &&
			!document.querySelector('.zone-card .zone-member-badge')`),
		chromedp.Evaluate[chromedp.Void](`window.__zoneDocument = true`),
		chromedp.Click(chromedp.CSS(".zone-card")),
		chromedp.WaitVisible(chromedp.CSS(".zone-member-details > summary")),
		chromedp.Click(chromedp.CSS(".zone-member-details > summary")),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('.zone-logical-member').length === 2 &&
			document.querySelectorAll('.zone-logical-member [role="status"][aria-label]').length === 2 &&
			document.querySelector('.zone-member-details button[aria-label$="details for Breakfast Room"]') &&
			document.querySelector('.zone-member-details').textContent.includes('192.0.2.11')`),
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
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`
		document.querySelector('.zone-status-label')?.textContent.trim() === 'Standalone' &&
		!document.querySelector('.zone-member-details') &&
		!Array.from(document.querySelectorAll('button')).some(button =>
			button.textContent.trim() === 'Dissolve zone')`)); err != nil {
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
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`
		document.querySelector('.zone-member-details > summary') &&
		Array.from(document.querySelectorAll('button')).some(button =>
			button.textContent.trim() === 'Dissolve zone')`)); err != nil {
		t.Fatalf("external creation did not update open detail: %v", err)
	}
	if elapsed := time.Since(createStarted); elapsed > 5*time.Second {
		t.Fatalf("external creation took %s; periodic fallback masked the event path", elapsed)
	}

	if err := chromedp.Do(ctx,
		chromedp.Click(chromedp.CSS(".device-detail .back-btn")),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('.device-card').length === 1 &&
			document.querySelector('.zone-card')`),
	); err != nil {
		t.Fatalf("return to grouped dashboard: %v", err)
	}
	master.UpdateStatus(func(status *webtypes.DeviceStatus) { status.Zone = nil })
	app.BroadcastDeviceList()
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`window.__zoneDocument === true &&
		document.querySelectorAll('.device-card').length === 2 && !document.querySelector('.zone-card') &&
		Array.from(document.querySelectorAll('.device-name')).some(name => name.textContent.trim() === 'Breakfast Room')`)); err != nil {
		t.Fatalf("dissolution did not restore member dashboard card without reload: %v", err)
	}
	master.UpdateStatus(func(status *webtypes.DeviceStatus) { status.Zone = zone })
	app.BroadcastDeviceList()
	if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`window.__zoneDocument === true &&
		document.querySelectorAll('.device-card').length === 1 && document.querySelector('.zone-card')`)); err != nil {
		t.Fatalf("creation did not suppress duplicate member dashboard card without reload: %v", err)
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
