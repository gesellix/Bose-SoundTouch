//go:build browsertest

package soundtouchweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

const (
	zoneNavigationMaster     = "192.0.2.10"
	zoneNavigationMember     = "192.0.2.11"
	zoneNavigationIneligible = "192.0.2.12"
	zoneNavigationPairLeft   = "192.0.2.13"
	zoneNavigationPairRight  = "192.0.2.14"
	zoneNavigationCandidate  = "192.0.2.15"
)

func TestZoneMembersOpenFullDeviceDetailWithoutTopologyMutation(t *testing.T) {
	app := newZoneMemberNavigationApp(t)
	server, zoneMutations := newZoneMemberNavigationServer(t, app)
	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(".zone-card", chromedp.ByQuery),
		chromedp.Click(".zone-card", chromedp.ByQuery),
		chromedp.WaitVisible(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Click(".zone-member-details > summary", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open zone detail: %v", err)
	}

	openZoneMemberAndWait(t, ctx, "Breakfast Room", `Array.from(document.querySelectorAll('.stereo-action')).some(button =>
		button.textContent.trim() === 'Create stereo pair' && button.disabled === false)`)
	returnToZone(t, ctx, "Atrium")

	openZoneMemberAndWait(t, ctx, "Workshop", `!document.querySelector('.stereo-pair-section')`)
	returnToZone(t, ctx, "Atrium")

	openZoneMemberAndWait(t, ctx, "Living Pair", `document.querySelectorAll('.stereo-pair-member').length === 2`)
	returnToZone(t, ctx, "Atrium")

	if got := zoneMutations.Load(); got != 0 {
		t.Fatalf("device-detail navigation sent %d zone mutation request(s)", got)
	}
}

func TestZoneMemberNavigationFollowsSiblingsAndRetracesBack(t *testing.T) {
	app := newZoneMemberNavigationApp(t)
	server, zoneMutations := newZoneMemberNavigationServer(t, app)
	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(".zone-card", chromedp.ByQuery),
		chromedp.Click(".zone-card", chromedp.ByQuery),
		chromedp.WaitVisible(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Click(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Poll(`!document.querySelector('button.zone-member-open[aria-label="Open details for Atrium"]')`, nil),
	); err != nil {
		t.Fatalf("open zone detail without a self row button: %v", err)
	}

	openZoneMemberAndWait(t, ctx, "Breakfast Room", `document.querySelector('.zone-member-details > summary') !== null`)
	if err := chromedp.Run(ctx,
		chromedp.Click(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('button.zone-member-open[aria-label="Open details for Living Pair"]') &&
			!document.querySelector('button.zone-member-open[aria-label="Open details for Breakfast Room"]')`, nil),
	); err != nil {
		t.Fatalf("member page did not offer its siblings: %v", err)
	}

	// A healthy pair reached from a sibling must look like the same pair
	// reached from the master: its live entry, not a recovery prompt.
	openZoneMemberAndWait(t, ctx, "Living Pair", `document.querySelectorAll('.stereo-pair-member').length === 2 &&
		!document.body.textContent.includes('Continue cleanup') &&
		!Array.from(document.querySelectorAll('button')).some(button =>
			button.textContent.includes('Remove from AfterTouch'))`)

	for _, want := range []string{"Breakfast Room", "Atrium"} {
		if err := chromedp.Run(ctx,
			chromedp.Click(".device-detail .back-btn", chromedp.ByQuery),
			chromedp.Poll(fmt.Sprintf(`document.querySelector('.page-title')?.textContent.includes(%q)`, want), nil),
		); err != nil {
			t.Fatalf("back did not return to %q: %v", want, err)
		}
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(".device-detail .back-btn", chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.page-title')?.textContent.trim() === 'Devices'`, nil),
	); err != nil {
		t.Fatalf("back from the first detail page did not return to the device list: %v", err)
	}

	if got := zoneMutations.Load(); got != 0 {
		t.Fatalf("device-detail navigation sent %d zone mutation request(s)", got)
	}
}

func TestZoneMemberLeaveTargetsMemberWithoutReload(t *testing.T) {
	app := newZoneMemberNavigationApp(t)
	server, zoneMutations := newZoneMemberNavigationServer(t, app)
	leaveStarted := make(chan struct{})
	releaseLeave := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseLeave) })
	zoneMutations.beforeLeaveResponse = func() {
		close(leaveStarted)
		<-releaseLeave
	}
	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(".zone-card", chromedp.ByQuery),
		chromedp.Click(".zone-card", chromedp.ByQuery),
		chromedp.WaitVisible(".zone-member-details > summary", chromedp.ByQuery),
		chromedp.Click(".zone-member-details > summary", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open zone detail: %v", err)
	}

	openZoneMemberAndWait(t, ctx, "Breakfast Room", `Array.from(document.querySelectorAll('button')).some(button =>
		button.textContent.trim() === 'Leave zone')`)

	var sameDocument bool
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`window.__zoneLeaveDocumentMarker = true`, nil),
		chromedp.Click(`//button[normalize-space()="Leave zone"]`, chromedp.BySearch),
		chromedp.ActionFunc(func(ctx context.Context) error {
			select {
			case <-leaveStarted:
			case <-ctx.Done():
				return ctx.Err()
			}
			master, _ := app.GetDevice(zoneNavigationMaster)
			master.UpdateStatus(func(status *webtypes.DeviceStatus) {
				members := append([]models.Member(nil), status.Zone.Members...)
				members = append(members, models.Member{DeviceID: "CANDIDATE", IP: zoneNavigationCandidate})
				status.Zone = &models.ZoneInfo{Master: status.Zone.Master, Members: members}
			})
			app.BroadcastDeviceList()
			return nil
		}),
		chromedp.Poll(`!!document.querySelector('.zone-section button[aria-label="Open details for Library"]')`, nil),
		chromedp.ActionFunc(func(context.Context) error {
			releaseOnce.Do(func() { close(releaseLeave) })
			return nil
		}),
		chromedp.Poll(`document.querySelector('[role="alert"]')?.textContent.includes('leave rejected')`, nil),
		chromedp.Evaluate(`window.__zoneLeaveDocumentMarker === true`, &sameDocument),
	); err != nil {
		t.Fatalf("leave zone from member detail: %v", err)
	}

	if !sameDocument {
		t.Fatal("leave zone reloaded the browser document")
	}
	if got := zoneMutations.Load(); got != 1 {
		t.Fatalf("leave zone sent %d zone mutation request(s), want 1", got)
	}
	want := "POST /api/control/devices/" + zoneNavigationMember + "/zone/leave"
	if got := zoneMutations.Request(); got != want {
		t.Fatalf("leave zone request = %q, want %q", got, want)
	}
}

func openZoneMemberAndWait(t *testing.T, ctx context.Context, name, condition string) {
	t.Helper()

	selector := fmt.Sprintf(`button.zone-member-open[aria-label=%q]`, "Open details for "+name)
	if err := chromedp.Run(ctx,
		chromedp.Click(selector, chromedp.ByQuery),
		chromedp.Poll(fmt.Sprintf(`document.querySelector('.page-title')?.textContent.includes(%q) && (%s)`,
			name, condition), nil),
	); err != nil {
		t.Fatalf("open %q device detail: %v", name, err)
	}
}

func returnToZone(t *testing.T, ctx context.Context, zoneName string) {
	t.Helper()

	if err := chromedp.Run(ctx,
		chromedp.Click(".device-detail .back-btn", chromedp.ByQuery),
		chromedp.Poll(fmt.Sprintf(`document.querySelector('.page-title')?.textContent.includes(%q) &&
			document.querySelector('.zone-member-details > summary') !== null`, zoneName), nil),
		chromedp.Click(".zone-member-details > summary", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("return to %q zone detail: %v", zoneName, err)
	}
}

func newZoneMemberNavigationApp(t *testing.T) *WebApp {
	t.Helper()

	app := NewWebApp()
	pair := &models.Group{
		ID:             "pair-1",
		Name:           "Living Pair",
		MasterDeviceID: "PAIR-LEFT",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "PAIR-LEFT", Role: "LEFT", IPAddress: zoneNavigationPairLeft},
			{DeviceID: "PAIR-RIGHT", Role: "RIGHT", IPAddress: zoneNavigationPairRight},
		}},
	}
	zone := &models.ZoneInfo{
		Master: "ZONE-MASTER",
		Members: []models.Member{
			{DeviceID: "ZONE-MASTER", IP: zoneNavigationMaster},
			{DeviceID: "ZONE-MEMBER", IP: zoneNavigationMember},
			{DeviceID: "INELIGIBLE", IP: zoneNavigationIneligible},
			{DeviceID: "PAIR-LEFT", IP: zoneNavigationPairLeft},
			{DeviceID: "PAIR-RIGHT", IP: zoneNavigationPairRight},
		},
	}

	addZoneNavigationDevice(t, app, zoneNavigationMaster, "ZONE-MASTER", "Atrium", "SoundTouch 20", nil, zone)
	addZoneNavigationDevice(t, app, zoneNavigationMember, "ZONE-MEMBER", "Breakfast Room", "SoundTouch 10", nil, nil)
	addZoneNavigationDevice(t, app, zoneNavigationIneligible, "INELIGIBLE", "Workshop", "SoundTouch 20", nil, nil)
	addZoneNavigationDevice(t, app, zoneNavigationPairLeft, "PAIR-LEFT", "Living Left", "SoundTouch 10", pair, nil)
	addZoneNavigationDevice(t, app, zoneNavigationPairRight, "PAIR-RIGHT", "Living Right", "SoundTouch 10", pair, nil)
	addZoneNavigationDevice(t, app, zoneNavigationCandidate, "CANDIDATE", "Library", "SoundTouch 10", nil, nil)

	return app
}

func addZoneNavigationDevice(
	t *testing.T,
	app *WebApp,
	host, deviceID, name, model string,
	group *models.Group,
	zone *models.ZoneInfo,
) {
	t.Helper()

	connection := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
		DeviceID:  deviceID,
		Name:      name,
		Type:      model,
		IPAddress: host,
	})
	connection.SetStatus(&webtypes.DeviceStatus{
		NowPlaying:  &models.NowPlaying{Source: "STANDBY"},
		Volume:      &models.Volume{TargetVolume: 20, ActualVolume: 20},
		Group:       group,
		Zone:        zone,
		IsConnected: true,
	})
	if !app.AddDevice(host, connection) {
		t.Fatalf("add duplicate browser fixture %q", host)
	}
}

type zoneMutationRecorder struct {
	count               atomic.Int32
	request             atomic.Value
	beforeLeaveResponse func()
}

func (r *zoneMutationRecorder) Load() int32 {
	return r.count.Load()
}

func (r *zoneMutationRecorder) Request() string {
	request, _ := r.request.Load().(string)
	return request
}

func newZoneMemberNavigationServer(t *testing.T, app *WebApp) (*httptest.Server, *zoneMutationRecorder) {
	t.Helper()

	projection := app.deviceViewSnapshot()
	zone := projection[zoneNavigationMaster].Zone
	if zone == nil || len(zone.Members) != 4 {
		t.Fatalf("unexpected zone projection: %+v", zone)
	}

	mutations := &zoneMutationRecorder{}
	router := chi.NewRouter()
	app.MountWeb(router, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if r.Method == http.MethodPost && strings.Contains(path, "/zone/") {
			mutations.count.Add(1)
			mutations.request.Store(r.Method + " " + path)
		}

		switch {
		case r.Method == http.MethodPost &&
			path == "/api/control/devices/"+zoneNavigationMember+"/zone/leave":
			if mutations.beforeLeaveResponse != nil {
				mutations.beforeLeaveResponse()
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			writeZoneNavigationJSON(w, webtypes.APIResponse{Success: false, Error: "leave rejected"})
		case r.Method == http.MethodGet && path == "/api/control/devices/"+zoneNavigationMaster+"/zone":
			writeZoneNavigationJSON(w, webtypes.APIResponse{Success: true, Data: map[string]interface{}{
				"masterIp":            zone.MasterControlID,
				"masterHwId":          zone.MasterDeviceID,
				"masterName":          zone.Members[0].Name,
				"master":              zone.Members[0],
				"members":             zone.Members[1:],
				"physicalMemberCount": zone.PhysicalMemberCount,
				"isMaster":            true,
				"isSlave":             false,
				"isStandalone":        false,
			}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/zone") &&
			strings.HasPrefix(path, "/api/control/devices/"):
			writeZoneNavigationJSON(w, webtypes.APIResponse{Success: true, Data: map[string]interface{}{
				"masterIp":     zone.MasterControlID,
				"masterHwId":   zone.MasterDeviceID,
				"masterName":   zone.Members[0].Name,
				"members":      []interface{}{},
				"isMaster":     false,
				"isSlave":      true,
				"isStandalone": false,
			}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/zone/candidates"):
			writeZoneNavigationJSON(w, webtypes.APIResponse{Success: true, Data: map[string]interface{}{}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/stereo-pair"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/control/devices/"), "/stereo-pair")
			writeZoneNavigationJSON(w, webtypes.APIResponse{Success: true, Data: map[string]interface{}{
				"capable": id == zoneNavigationMember || id == zoneNavigationPairLeft,
			}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/recents"):
			writeZoneNavigationJSON(w, webtypes.APIResponse{Success: true, Data: []interface{}{}})
		default:
			router.ServeHTTP(w, r)
		}
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, mutations
}

func writeZoneNavigationJSON(w http.ResponseWriter, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
