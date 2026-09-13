//go:build browsertest

package soundtouchweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestZoneMemberDetailKeepsNewerStatusAcrossDelayedLoad(t *testing.T) {
	app := newZoneMemberNavigationApp(t)
	server, _ := newZoneMemberNavigationServer(t, app)
	ctx := newHeadlessChromeContext(t)

	var result struct {
		PendingTrack string `json:"pendingTrack"`
		LoadedTrack  string `json:"loadedTrack"`
		Revision     int    `json:"revision"`
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.Evaluate(`window.__zoneMemberFenceResult = null;
        import('/app/static/js/app.js').then(({ completeSelectedMemberLoad, mergeSelectedMemberStatus }) => {
            const selected = {
                controlId: 'member', originId: 'zone', device: null, loading: true,
            };
            const pending = mergeSelectedMemberStatus(selected, 'member', {
                epoch: 3, revision: 7, nowPlaying: { Track: 'new event' },
            });
            const loaded = completeSelectedMemberLoad(pending, 'member', 'zone', {
                info: { name: 'Member' },
                status: { epoch: 3, revision: 6, nowPlaying: { Track: 'old REST' } },
            });
            window.__zoneMemberFenceResult = {
                pendingTrack: pending.pendingStatus?.nowPlaying?.Track,
                loadedTrack: loaded.device?.status?.nowPlaying?.Track,
                revision: loaded.device?.status?.revision,
            };
        })`, nil),
		chromedp.Poll(`window.__zoneMemberFenceResult !== null`, nil),
		chromedp.Evaluate(`window.__zoneMemberFenceResult`, &result),
	); err != nil {
		t.Fatalf("evaluate selected-member status fence: %v", err)
	}
	if result.PendingTrack != "new event" || result.LoadedTrack != "new event" || result.Revision != 7 {
		t.Fatalf("selected-member status fence = %+v, want newer event revision 7", result)
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

func newZoneMemberNavigationServer(t *testing.T, app *WebApp) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	projection := app.deviceViewSnapshot()
	zone := projection[zoneNavigationMaster].Zone
	if zone == nil || len(zone.Members) != 4 {
		t.Fatalf("unexpected zone projection: %+v", zone)
	}

	mutations := &atomic.Int32{}
	router := chi.NewRouter()
	app.MountWeb(router, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if r.Method == http.MethodPost && strings.Contains(path, "/zone/") {
			mutations.Add(1)
		}

		switch {
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
