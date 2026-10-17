package soundtouchweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// These convergence cases restore the volume behavior from 21daae17 through
// the current per-field generation and topology APIs.
func TestVolumeReadbackConvergence(t *testing.T) {
	for _, test := range []struct {
		name                                                                          string
		queue                                                                         []int
		invalidateFirst, invalidateEvery, ignoreWrites, unrelated, topologyDuringWait bool
		wantConfirmed                                                                 bool
		wantGets                                                                      int
		wantActual                                                                    *int
	}{
		{name: "lagging readback", queue: []int{10, 10, 40}, wantConfirmed: true, wantGets: 3, wantActual: intPointer(40)},
		{name: "matching event invalidates first read", invalidateFirst: true, wantConfirmed: true, wantGets: 2, wantActual: intPointer(40)},
		{name: "bounded repeated invalidation", invalidateEvery: true, wantGets: 3},
		{name: "matching cache cannot confirm fresh mismatch", invalidateFirst: true, ignoreWrites: true, wantGets: 3, wantActual: intPointer(10)},
		{name: "unrelated field event preserves cache", unrelated: true, wantConfirmed: true, wantGets: 1, wantActual: intPointer(40)},
		{name: "topology changes during retry wait", ignoreWrites: true, topologyDuringWait: true, wantGets: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			speaker := newVolumeTestSpeaker(t, 10, "")
			speaker.volumeQueue = test.queue
			speaker.ignoreWrites = test.ignoreWrites
			app := NewWebApp()
			conn := addVolumeTestDevice(app, "speaker", "SPEAKER", "Office", "SoundTouch 20", speaker, 40, nil, nil)
			topology, _ := conn.SnapshotGroupTopology()
			speaker.onVolumeGet = func(get int) {
				if test.invalidateEvery || (test.invalidateFirst && get == 1) {
					conn.ApplyVolumeEvent(&models.Volume{TargetVolume: 40, ActualVolume: 40}, time.Now())
				}
				if test.unrelated {
					conn.ApplyFieldEvent(webtypes.FieldNowPlaying, func(status *webtypes.DeviceStatus) {
						status.NowPlaying = &models.NowPlaying{Source: "RADIO"}
					})
				}
			}
			waits := 0
			app.volumeReadbackRetryWait = func(delay time.Duration) {
				waits++
				if delay != 50*time.Millisecond {
					t.Errorf("retry delay = %v", delay)
				}
				if test.topologyDuringWait {
					conn.ApplyGroupEvent(nil, time.Now())
				}
			}
			member := zoneVolumeMemberResult{}
			confirmed, merged := app.applyVolumeTarget(&member, "speaker", conn, topology, nil, 40)
			if confirmed != test.wantConfirmed || (member.Status == volumeConfirmed) != test.wantConfirmed {
				t.Fatalf("confirmation = %v result = %+v", confirmed, member)
			}
			if test.wantActual == nil {
				if member.Actual != nil || merged {
					t.Fatalf("obsolete readback merged: %+v", member)
				}
			} else if member.Actual == nil || *member.Actual != *test.wantActual || !merged {
				t.Fatalf("actual = %+v merged=%v", member, merged)
			}
			if confirmed && member.Error != "" {
				t.Fatalf("confirmed readback retained an old error: %+v", member)
			}
			if _, posts, gets := speaker.operations(); fmt.Sprint(posts) != "[40]" || gets != test.wantGets {
				t.Fatalf("operations = posts %v gets %d", posts, gets)
			}
			if waits > 2 || (test.topologyDuringWait && waits != 1) {
				t.Fatalf("readback retry waits = %d", waits)
			}
			if test.unrelated && conn.Status().NowPlaying.Source != "RADIO" {
				t.Fatal("volume merge discarded unrelated event")
			}
		})
	}
}

func TestZoneVolumeFreshParallelBatchCapture(t *testing.T) {
	zone, zoneXML := testZone()
	master := newVolumeTestSpeaker(t, 20, zoneXML)
	member := newVolumeTestSpeaker(t, 35, "")
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	for _, speaker := range []*volumeTestSpeaker{master, member} {
		speaker.onVolumeGet = func(get int) {
			if get == 1 {
				entered <- struct{}{}
				<-release
			}
		}
	}
	app := NewWebApp()
	// Deliberately stale cache values must not determine the baseline or delta.
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", master, 95, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Member", "SoundTouch 20", member, 70, nil, nil)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		app.HandleZoneVolume(response, withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{"id": "192.0.2.10", "volume": "40"}))
		close(done)
	}()
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("baseline reads did not run in parallel")
		}
	}
	for _, speaker := range []*volumeTestSpeaker{master, member} {
		if _, posts, _ := speaker.operations(); len(posts) != 0 {
			t.Fatalf("write began before batch capture completed: %v", posts)
		}
	}
	releaseOnce.Do(func() { close(release) })
	awaitVolumeTestDone(t, done)
	var payload struct {
		Data zoneVolumeResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || payload.Data.Baseline != 35 || payload.Data.Delta != 5 || payload.Data.Partial {
		t.Fatalf("fresh baseline result = %d %s", response.Code, response.Body.String())
	}
	if _, posts, gets := master.operations(); fmt.Sprint(posts) != "[25]" || gets != 2 {
		t.Fatalf("master operations = posts %v gets %d", posts, gets)
	}
	if _, posts, gets := member.operations(); fmt.Sprint(posts) != "[40]" || gets != 2 {
		t.Fatalf("member operations = posts %v gets %d", posts, gets)
	}
}

func TestZoneVolumeSerializesGroupAndMemberBatches(t *testing.T) {
	for _, memberOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("second member=%v", memberOnly), func(t *testing.T) {
			zone, zoneXML := testZone()
			master := newVolumeTestSpeaker(t, 20, zoneXML)
			member := newVolumeTestSpeaker(t, 20, "")
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			master.onVolumePost = func() {
				_, posts, _ := master.operations()
				if len(posts) == 1 {
					close(entered)
					<-release
				}
			}
			app := NewWebApp()
			addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", master, 20, nil, zone)
			addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Member", "SoundTouch 20", member, 20, nil, nil)
			request := func(level string) *http.Request {
				return withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{"id": "192.0.2.10", "volume": level})
			}
			first, second := httptest.NewRecorder(), httptest.NewRecorder()
			firstDone, secondDone := make(chan struct{}), make(chan struct{})
			go func() { app.HandleZoneVolume(first, request("25")); close(firstDone) }()
			awaitVolumeTestDone(t, entered)
			go func() {
				if memberOnly {
					app.HandleZoneMemberVolume(second, zoneMemberVolumeTestRequest("192.0.2.10", "MEMBER", 30))
				} else {
					app.HandleZoneVolume(second, request("30"))
				}
				close(secondDone)
			}()
			select {
			case <-secondDone:
				t.Fatal("second batch completed before first batch reconciliation")
			case <-time.After(50 * time.Millisecond):
			}
			if _, posts, gets := master.operations(); fmt.Sprint(posts) != "[25]" || gets != 1 {
				t.Fatalf("second batch captured volume before first completed: posts=%v gets=%d", posts, gets)
			}
			if _, posts, _ := member.operations(); len(posts) > 1 {
				t.Fatalf("member write overlapped group batch: %v", posts)
			}
			releaseOnce.Do(func() { close(release) })
			awaitVolumeTestDone(t, firstDone)
			awaitVolumeTestDone(t, secondDone)
			if first.Code != 200 || second.Code != 200 {
				t.Fatalf("batch responses: %d %s / %d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
			}
			if _, posts, _ := member.operations(); fmt.Sprint(posts) != "[25 30]" {
				t.Fatalf("serialized member writes = %v", posts)
			}
			if !memberOnly {
				var payload struct {
					Data zoneVolumeResult `json:"data"`
				}
				if err := json.Unmarshal(second.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Data.Baseline != 25 || payload.Data.Delta != 5 {
					t.Fatalf("second batch = %+v", payload.Data)
				}
			}
		})
	}
}

func TestVolumePhysicalSerializationIncludesDirectRoute(t *testing.T) {
	zone, zoneXML := testZone()
	master := newVolumeTestSpeaker(t, 20, zoneXML)
	member := newVolumeTestSpeaker(t, 20, "")
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	member.onVolumeGet = func(get int) {
		if get == 1 {
			close(entered)
			<-release
		}
	}
	app := NewWebApp()
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", master, 20, nil, zone)
	conn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Member", "SoundTouch 20", member, 20, nil, nil)
	conn.WebSocket = &client.WebSocketClient{}
	first, second := httptest.NewRecorder(), httptest.NewRecorder()
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	go func() {
		app.HandleZoneMemberVolume(first, zoneMemberVolumeTestRequest("192.0.2.10", "MEMBER", 40))
		close(firstDone)
	}()
	awaitVolumeTestDone(t, entered)
	go func() {
		app.HandleDirectVolumeControl(second, withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{"id": "192.0.2.20", "volume": "45"}))
		close(secondDone)
	}()
	select {
	case <-secondDone:
		t.Fatal("direct route completed during member reconciliation")
	case <-time.After(50 * time.Millisecond):
	}
	if _, posts, gets := member.operations(); fmt.Sprint(posts) != "[40]" || gets != 1 {
		t.Fatalf("direct write interleaved with member readback: posts %v gets %d", posts, gets)
	}
	releaseOnce.Do(func() { close(release) })
	awaitVolumeTestDone(t, firstDone)
	awaitVolumeTestDone(t, secondDone)
	if first.Code != 200 || second.Code != 200 || conn.Status().Volume.ActualVolume != 45 {
		t.Fatalf("serialized results = %d %s / %d %s, cache=%+v", first.Code, first.Body.String(), second.Code, second.Body.String(), conn.Status().Volume)
	}
	if _, posts, gets := member.operations(); fmt.Sprint(posts) != "[40 45]" || gets != 2 {
		t.Fatalf("physical operations = posts %v gets %d", posts, gets)
	}
}

func awaitVolumeTestDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("volume operation did not reach expected barrier")
	}
}

func TestZoneVolumeWritesLogicalStereoMasterOnce(t *testing.T) {
	zone := &models.ZoneInfo{Master: "MASTER", Members: []models.Member{
		{DeviceID: "MASTER", IP: "192.0.2.10"},
		{DeviceID: "RIGHT", IP: "192.0.2.21"},
	}}
	group := &models.Group{ID: "pair", MasterDeviceID: "RIGHT", Status: "GROUP_OK",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "LEFT", Role: "LEFT", IPAddress: "192.0.2.20"},
			{DeviceID: "RIGHT", Role: "RIGHT", IPAddress: "192.0.2.21"},
		}},
	}
	master := newVolumeTestSpeaker(t, 20, `<zone master="MASTER"><member ipaddress="192.0.2.10">MASTER</member><member ipaddress="192.0.2.21">RIGHT</member></zone>`)
	left, right := newVolumeTestSpeaker(t, 30, ""), newVolumeTestSpeaker(t, 30, "")
	app := NewWebApp()
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", master, 20, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "LEFT", "Pair", "SoundTouch 10", left, 30, group, nil)
	addVolumeTestDevice(app, "192.0.2.21", "RIGHT", "Pair", "SoundTouch 10", right, 30, group, nil)
	response := httptest.NewRecorder()
	app.HandleZoneVolume(response, withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{"id": "192.0.2.10", "volume": "40"}))
	if response.Code != 200 {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if _, posts, gets := master.operations(); fmt.Sprint(posts) != "[30]" || gets != 2 {
		t.Fatalf("zone master operations = %v / %d", posts, gets)
	}
	if _, posts, gets := right.operations(); fmt.Sprint(posts) != "[40]" || gets != 2 {
		t.Fatalf("pair master operations = %v / %d", posts, gets)
	}
	if _, posts, gets := left.operations(); len(posts) != 0 || gets != 0 {
		t.Fatalf("pair slave operations = %v / %d", posts, gets)
	}
}

func TestVerifiedDirectVolumeRejectsSlaveAndUnconfirmedPair(t *testing.T) {
	for _, slave := range []bool{false, true} {
		t.Run(fmt.Sprintf("slave=%v", slave), func(t *testing.T) {
			speaker := newVolumeTestSpeaker(t, 20, "")
			app := NewWebApp()
			conn := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.server.URL}), &models.DeviceInfo{DeviceID: "SPEAKER", Type: "SoundTouch 10"})
			if slave {
				conn.ApplyGroupEvent(&models.Group{ID: "pair", MasterDeviceID: "OTHER"}, time.Now())
			}
			conn.WebSocket = &client.WebSocketClient{}
			app.AddDevice("speaker", conn)
			response := httptest.NewRecorder()
			app.HandleDirectVolumeControl(response, withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{"id": "speaker", "volume": "40"}))
			if response.Code != http.StatusConflict {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if _, posts, gets := speaker.operations(); len(posts) != 0 || gets != 0 {
				t.Fatalf("unwritable target performed I/O: %v / %d", posts, gets)
			}
		})
	}
}

func TestZoneVolumeRepeatedMovesPreservePhysicalOffsets(t *testing.T) {
	for _, offset := range []int{5, 0} {
		t.Run(fmt.Sprintf("offset=%d", offset), func(t *testing.T) {
			zone, zoneXML := testZone()
			master := newVolumeTestSpeaker(t, 20, zoneXML)
			member := newVolumeTestSpeaker(t, 20+offset, "")
			app := NewWebApp()
			masterConn := addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", master, 60, nil, zone)
			memberConn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Member", "SoundTouch 20", member, 60, nil, nil)
			previousMax := 20 + offset
			for index, requested := range []int{35, 18, 43, 12, 23} {
				response := httptest.NewRecorder()
				app.HandleZoneVolume(response, withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{
					"id": "192.0.2.10", "volume": fmt.Sprint(requested),
				}))
				var payload struct {
					Success bool             `json:"success"`
					Data    zoneVolumeResult `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 || !payload.Success || payload.Data.Partial ||
					payload.Data.Requested != requested || payload.Data.Baseline != previousMax ||
					payload.Data.Delta != requested-previousMax || len(payload.Data.Members) != 2 {
					t.Fatalf("move %d response = %d %s", index, response.Code, response.Body.String())
				}
				masterActual, masterPosts, masterGets := master.operations()
				memberActual, memberPosts, memberGets := member.operations()
				if masterActual != requested-offset || memberActual != requested || memberActual-masterActual != offset {
					t.Fatalf("move %d physical levels = %d/%d, requested=%d offset=%d", index, masterActual, memberActual, requested, offset)
				}
				if len(masterPosts) != index+1 || len(memberPosts) != index+1 ||
					masterPosts[index] != masterActual || memberPosts[index] != memberActual ||
					masterGets != 2*(index+1) || memberGets != 2*(index+1) {
					t.Fatalf("move %d operations = master %v/%d member %v/%d", index, masterPosts, masterGets, memberPosts, memberGets)
				}
				for _, result := range payload.Data.Members {
					want := memberActual
					if result.DeviceID == "MASTER" {
						want = masterActual
					}
					if result.Status != volumeConfirmed || result.Actual == nil || *result.Actual != want || result.Target == nil || *result.Target != want {
						t.Fatalf("move %d member result = %+v, want=%d", index, result, want)
					}
				}
				if got := masterConn.Status().Volume; got == nil || got.TargetVolume != masterActual || got.ActualVolume != masterActual {
					t.Fatalf("move %d master cache = %+v", index, got)
				}
				if got := memberConn.Status().Volume; got == nil || got.TargetVolume != memberActual || got.ActualVolume != memberActual {
					t.Fatalf("move %d member cache = %+v", index, got)
				}
				view := app.deviceViewSnapshot()["192.0.2.10"]
				if view.Zone == nil || view.Zone.Volume == nil || *view.Zone.Volume != requested {
					t.Fatalf("move %d projected max = %+v", index, view.Zone)
				}
				previousMax = requested
			}
		})
	}
}

func TestQueuedDirectVolumeRechecksPhysicalAdmission(t *testing.T) {
	for _, replaceRegistry := range []bool{false, true} {
		t.Run(fmt.Sprintf("registry replacement=%v", replaceRegistry), func(t *testing.T) {
			zone, zoneXML := testZone()
			master := newVolumeTestSpeaker(t, 20, zoneXML)
			member := newVolumeTestSpeaker(t, 20, "")
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			member.onVolumeGet = func(get int) {
				if get == 1 {
					close(entered)
					<-release
				}
			}
			app := NewWebApp()
			addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", master, 20, nil, zone)
			conn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Member", "SoundTouch 20", member, 20, nil, nil)
			first, second := httptest.NewRecorder(), httptest.NewRecorder()
			firstDone, secondDone := make(chan struct{}), make(chan struct{})
			go func() {
				app.HandleZoneMemberVolume(first, zoneMemberVolumeTestRequest("192.0.2.10", "MEMBER", 40))
				close(firstDone)
			}()
			awaitVolumeTestDone(t, entered)
			// The direct handler's resolved connection is captured before waiting
			// for the operation mutex, even if discovery replaces its registry slot.
			go func() { app.handleVerifiedVolumeControl(second, "192.0.2.20", conn, 45); close(secondDone) }()
			select {
			case <-secondDone:
				t.Fatal("direct operation did not wait for member reconciliation")
			case <-time.After(50 * time.Millisecond):
			}
			var replacement *webtypes.DeviceConnection
			var replacementSpeaker *volumeTestSpeaker
			if replaceRegistry {
				replacementSpeaker = newVolumeTestSpeaker(t, 70, "")
				app.RemoveDevice("192.0.2.20")
				replacement = addVolumeTestDevice(app, "192.0.2.20", "REPLACEMENT", "Replacement", "SoundTouch 20", replacementSpeaker, 70, nil, nil)
			} else {
				conn.ApplyGroupEvent(&models.Group{ID: "changed-pair", MasterDeviceID: "OTHER"}, time.Now())
			}
			releaseOnce.Do(func() { close(release) })
			awaitVolumeTestDone(t, firstDone)
			awaitVolumeTestDone(t, secondDone)
			if first.Code != 200 || second.Code != http.StatusConflict {
				t.Fatalf("responses = %d %s / %d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
			}
			if _, posts, gets := member.operations(); fmt.Sprint(posts) != "[40]" || gets != 1 {
				t.Fatalf("queued obsolete operation performed I/O: posts %v gets %d", posts, gets)
			}
			if conn.Status().Volume.ActualVolume != 20 {
				t.Fatalf("obsolete readback merged into original cache: %+v", conn.Status().Volume)
			}
			if replacement != nil {
				if replacement.Status().Volume.ActualVolume != 70 {
					t.Fatalf("obsolete readback merged into replacement: %+v", replacement.Status().Volume)
				}
				if _, posts, gets := replacementSpeaker.operations(); len(posts) != 0 || gets != 0 {
					t.Fatalf("queued operation reached replacement: posts %v gets %d", posts, gets)
				}
			}
		})
	}
}
