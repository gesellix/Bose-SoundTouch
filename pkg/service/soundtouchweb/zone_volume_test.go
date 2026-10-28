package soundtouchweb

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

type volumeTestSpeaker struct {
	server         *httptest.Server
	mu             sync.Mutex
	zoneXML        string
	volume         int
	posts          []int
	volumeGets     int
	ignoreWrites   bool
	volumeResponse string
	volumeQueue    []int
	writeStatus    int
	writeBody      string
	uncertainWrite bool
	onVolumeGet    func(int)
	onVolumePost   func()
}

func newVolumeTestSpeaker(t *testing.T, volume int, zoneXML string) *volumeTestSpeaker {
	t.Helper()

	speaker := &volumeTestSpeaker{volume: volume, zoneXML: zoneXML}
	speaker.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/getZone":
			_, _ = io.WriteString(w, speaker.zoneXML)
		case r.Method == http.MethodPost && r.URL.Path == "/volume":
			var request models.VolumeRequest
			if err := xml.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			speaker.mu.Lock()
			if !speaker.ignoreWrites && speaker.writeStatus == 0 {
				speaker.volume = request.Value
			}
			speaker.posts = append(speaker.posts, request.Value)
			postHook := speaker.onVolumePost
			speaker.mu.Unlock()
			if postHook != nil {
				postHook()
			}
			if speaker.writeStatus != 0 {
				w.WriteHeader(speaker.writeStatus)
				body := speaker.writeBody
				if body == "" {
					body = `<errors><error value="1" name="REJECTED">topology word is not a conflict</error></errors>`
				}
				_, _ = io.WriteString(w, body)
				return
			}
			if speaker.uncertainWrite {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/volume":
			speaker.mu.Lock()
			speaker.volumeGets++
			get := speaker.volumeGets
			volume := speaker.volume
			if len(speaker.volumeQueue) > 0 {
				volume = speaker.volumeQueue[0]
				speaker.volumeQueue = speaker.volumeQueue[1:]
			}
			response := speaker.volumeResponse
			hook := speaker.onVolumeGet
			speaker.mu.Unlock()
			if hook != nil {
				hook(get)
			}
			if response != "" {
				_, _ = io.WriteString(w, response)
				return
			}

			_, _ = fmt.Fprintf(w,
				`<volume deviceID="speaker"><targetvolume>%d</targetvolume><actualvolume>%d</actualvolume><muteenabled>false</muteenabled></volume>`,
				volume,
				volume,
			)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(speaker.server.Close)

	return speaker
}

func (speaker *volumeTestSpeaker) operations() (int, []int, int) {
	speaker.mu.Lock()
	defer speaker.mu.Unlock()

	return speaker.volume, append([]int(nil), speaker.posts...), speaker.volumeGets
}

func addVolumeTestDevice(
	app *WebApp,
	controlID, deviceID, name, deviceType string,
	speaker *volumeTestSpeaker,
	volume int,
	group *models.Group,
	zone *models.ZoneInfo,
) *webtypes.DeviceConnection {
	conn := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: speaker.server.URL}),
		&models.DeviceInfo{DeviceID: deviceID, Name: name, Type: deviceType, IPAddress: controlID},
	)
	conn.SetStatus(&webtypes.DeviceStatus{
		IsConnected: true,
		Volume:      &models.Volume{TargetVolume: volume, ActualVolume: volume},
	})

	groupGeneration := conn.BeginGroupRefresh()
	conn.ApplyPolledGroup(groupGeneration, group)
	if _, current := conn.SnapshotGroupTopology(); !current {
		panic("test group topology was not accepted")
	}
	if zone != nil {
		zoneGeneration := conn.BeginZoneRefresh()
		if !conn.ApplyPolledZone(zoneGeneration, deviceID, zone) {
			panic("test zone topology was not accepted")
		}
	}
	app.AddDevice(controlID, conn)

	return conn
}

func testZone() (*models.ZoneInfo, string) {
	zone := &models.ZoneInfo{Master: "MASTER", Members: []models.Member{
		{DeviceID: "MASTER", IP: "192.0.2.10"},
		{DeviceID: "MEMBER", IP: "192.0.2.20"},
	}}

	return zone, `<zone master="MASTER"><member ipaddress="192.0.2.10">MASTER</member><member ipaddress="192.0.2.20">MEMBER</member></zone>`
}

func TestHandleZoneVolumeSharedDeltaClampAndPartialReadback(t *testing.T) {
	zone, zoneXML := testZone()
	masterSpeaker := newVolumeTestSpeaker(t, 95, zoneXML)
	memberSpeaker := newVolumeTestSpeaker(t, 70, "")
	memberSpeaker.ignoreWrites = true
	var writeConcurrency struct {
		sync.Mutex
		active int
		max    int
	}
	observeWrite := func() {
		writeConcurrency.Lock()
		writeConcurrency.active++
		if writeConcurrency.active > writeConcurrency.max {
			writeConcurrency.max = writeConcurrency.active
		}
		writeConcurrency.Unlock()
		time.Sleep(20 * time.Millisecond)
		writeConcurrency.Lock()
		writeConcurrency.active--
		writeConcurrency.Unlock()
	}
	masterSpeaker.onVolumePost = observeWrite
	memberSpeaker.onVolumePost = observeWrite

	app := NewWebApp()
	masterConn := addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 95, nil, zone)
	memberConn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", memberSpeaker, 70, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/zone/volume/100", nil)
	request = withChiParams(request, map[string]string{"id": "192.0.2.10", "volume": "100"})
	response := httptest.NewRecorder()
	app.HandleZoneVolume(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data zoneVolumeResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Data.Partial || payload.Data.Baseline != 95 || payload.Data.Delta != 5 {
		t.Fatalf("result = %+v", payload.Data)
	}

	masterVolume, masterPosts, masterGets := masterSpeaker.operations()
	memberVolume, memberPosts, memberGets := memberSpeaker.operations()
	if masterVolume != 100 || fmt.Sprint(masterPosts) != "[100]" || masterGets != 2 {
		t.Fatalf("master operations = volume %d posts %v gets %d", masterVolume, masterPosts, masterGets)
	}
	if memberVolume != 70 || fmt.Sprint(memberPosts) != "[75]" || memberGets != 4 {
		t.Fatalf("member operations = volume %d posts %v gets %d", memberVolume, memberPosts, memberGets)
	}
	writeConcurrency.Lock()
	maxConcurrentWrites := writeConcurrency.max
	writeConcurrency.Unlock()
	if maxConcurrentWrites > 2 {
		t.Fatalf("concurrent target writes = %d, exceeded the member count", maxConcurrentWrites)
	}
	if masterConn.Status().Volume.ActualVolume != 100 || memberConn.Status().Volume.ActualVolume != 70 {
		t.Fatalf("cache = master %d member %d", masterConn.Status().Volume.ActualVolume, memberConn.Status().Volume.ActualVolume)
	}

	view := app.deviceViewSnapshot()["192.0.2.10"]
	if view.Zone == nil || view.Zone.Volume == nil || *view.Zone.Volume != 100 {
		t.Fatalf("projected zone volume = %+v", view.Zone)
	}

	var mismatch *zoneVolumeMemberResult
	for index := range payload.Data.Members {
		if payload.Data.Members[index].DeviceID == "MEMBER" {
			mismatch = &payload.Data.Members[index]
		}
	}
	if mismatch == nil || mismatch.Target == nil || *mismatch.Target != 75 ||
		mismatch.Actual == nil || *mismatch.Actual != 70 ||
		mismatch.Status != volumeUnverified {
		t.Fatalf("partial member result = %+v", mismatch)
	}
}

func TestZoneVolumeSlowMemberDoesNotDelayOtherWrites(t *testing.T) {
	zone, zoneXML := testZone()
	masterSpeaker := newVolumeTestSpeaker(t, 20, zoneXML)
	memberSpeaker := newVolumeTestSpeaker(t, 10, "")
	started := make(chan struct{})
	release := make(chan struct{})
	otherWritten := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	masterSpeaker.onVolumePost = func() { close(started); <-release }
	memberSpeaker.onVolumePost = func() { close(otherWritten) }
	app := NewWebApp()
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Master", "SoundTouch 30", masterSpeaker, 20, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Member", "SoundTouch 20", memberSpeaker, 10, nil, nil)
	response := httptest.NewRecorder()
	request := withChiParams(httptest.NewRequest(http.MethodPost, "/", nil), map[string]string{"id": "192.0.2.10", "volume": "25"})
	done := make(chan struct{})
	go func() { app.HandleZoneVolume(response, request); close(done) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first member write did not start")
	}
	select {
	case <-otherWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("slow member delayed a healthy member write")
	}
	releaseOnce.Do(func() { close(release) })
	<-done
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStandaloneVolumeRoutesVerifyVolumeKeepSuccessEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		level   int
		handle  func(*WebApp, http.ResponseWriter, *http.Request)
		request func(int) *http.Request
	}{
		{
			name:  "direct path",
			level: 31,
			handle: func(app *WebApp, w http.ResponseWriter, r *http.Request) {
				app.HandleDirectVolumeControl(w, r)
			},
			request: func(level int) *http.Request {
				return withChiParams(
					httptest.NewRequest(http.MethodPost, fmt.Sprintf("/devices/speaker/volume/%d", level), nil),
					map[string]string{"id": "speaker", "volume": strconv.Itoa(level)},
				)
			},
		},
		{
			name:  "JSON action",
			level: 37,
			handle: func(app *WebApp, w http.ResponseWriter, r *http.Request) {
				app.HandleAPIControl(w, r)
			},
			request: func(level int) *http.Request {
				return withChiParams(
					httptest.NewRequest(
						http.MethodPost,
						"/devices/speaker/action/volume",
						strings.NewReader(fmt.Sprintf(`{"level":%d}`, level)),
					),
					map[string]string{"id": "speaker", "action": "volume"},
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			speaker := newVolumeTestSpeaker(t, 10, "")
			app := NewWebApp()
			conn := addVolumeTestDevice(
				app,
				"speaker",
				"SPEAKER",
				"Office",
				"SoundTouch 10",
				speaker,
				10,
				nil,
				nil,
			)
			conn.WebSocket = &client.WebSocketClient{}
			response := httptest.NewRecorder()
			test.handle(app, response, test.request(test.level))

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			var payload struct {
				Success bool `json:"success"`
				Data    struct {
					Requested int    `json:"requested"`
					Actual    int    `json:"actual"`
					Message   string `json:"message"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !payload.Success || payload.Data.Requested != 0 || payload.Data.Actual != 0 {
				t.Fatalf("response = %+v", payload)
			}
			if payload.Data.Message != fmt.Sprintf("Volume set to %d", test.level) {
				t.Fatalf("legacy confirmation message = %q", payload.Data.Message)
			}
			if _, posts, gets := speaker.operations(); fmt.Sprint(posts) != fmt.Sprintf("[%d]", test.level) || gets != 1 {
				t.Fatalf("speaker operations = posts %v gets %d", posts, gets)
			}
			if got := conn.Status().Volume; got == nil || got.TargetVolume != test.level || got.ActualVolume != test.level {
				t.Fatalf("cached readback = %+v", got)
			}
		})
	}
}

func TestHandleZoneVolumeRequiresActualToFinishRamping(t *testing.T) {
	zone, zoneXML := testZone()
	masterSpeaker := newVolumeTestSpeaker(t, 20, zoneXML)
	memberSpeaker := newVolumeTestSpeaker(t, 35, "")
	memberSpeaker.volumeResponse = `<volume deviceID="MEMBER"><targetvolume>80</targetvolume><actualvolume>36</actualvolume></volume>`
	app := NewWebApp()
	app.volumeReadbackRetryWait = func(time.Duration) {}
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 20, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", memberSpeaker, 35, nil, nil)
	request := withChiParams(httptest.NewRequest(http.MethodPost, "/zone/volume/80", nil),
		map[string]string{"id": "192.0.2.10", "volume": "80"})
	response := httptest.NewRecorder()
	app.HandleZoneVolume(response, request)
	var payload struct {
		Success bool             `json:"success"`
		Data    zoneVolumeResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !payload.Success || !payload.Data.Partial {
		t.Fatalf("ramping readback = %d %s", response.Code, response.Body.String())
	}
	for _, member := range payload.Data.Members {
		if member.DeviceID == "MEMBER" && member.Status != volumeUnverified {
			t.Fatalf("member = %+v", member)
		}
		if member.DeviceID == "MEMBER" && (member.Actual == nil || *member.Actual != 36) {
			t.Fatalf("ramping actual = %+v", member)
		}
	}
	if _, posts, gets := memberSpeaker.operations(); fmt.Sprint(posts) != "[80]" || gets != 4 {
		t.Fatalf("operations = posts %v gets %d", posts, gets)
	}
}

func TestApplyVolumeTargetRejectsStaleGenerationsWithoutActual(t *testing.T) {
	speaker := newVolumeTestSpeaker(t, 10, "")
	app := NewWebApp()
	conn := addVolumeTestDevice(app, "192.0.2.30", "SPEAKER", "Office", "SoundTouch 20", speaker, 10, nil, nil)
	topology, current := conn.SnapshotGroupTopology()
	if !current {
		t.Fatal("initial group topology is not confirmed")
	}

	t.Run("topology invalidated before write", func(t *testing.T) {
		replacement := conn.BeginGroupRefresh()
		conn.ApplyPolledGroup(replacement, &models.Group{
			ID:             "replacement-pair",
			MasterDeviceID: "OTHER",
		})
		result := zoneVolumeMemberResult{}
		atTarget, confirmed := app.applyVolumeTarget(&result, "192.0.2.30", conn, topology, nil, 40)
		if atTarget || confirmed || result.Actual != nil || result.Status != volumeFailed {
			t.Fatalf("stale topology result = %+v, atTarget=%v confirmed=%v", result, atTarget, confirmed)
		}
		if _, posts, gets := speaker.operations(); len(posts) != 0 || gets != 0 {
			t.Fatalf("stale topology performed I/O: posts %v gets %d", posts, gets)
		}
	})

	recovery := conn.BeginGroupRefresh()
	conn.ApplyPolledGroup(recovery, nil)
	topology, current = conn.SnapshotGroupTopology()
	if !current {
		t.Fatal("recovered topology is not confirmed")
	}
	speaker.onVolumeGet = func(int) {
		conn.ApplyVolumeEvent(&models.Volume{TargetVolume: 77, ActualVolume: 77}, time.Now())
	}

	t.Run("volume generation invalidated during readback", func(t *testing.T) {
		result := zoneVolumeMemberResult{}
		atTarget, cacheApplied := app.applyVolumeTarget(&result, "192.0.2.30", conn, topology, nil, 40)
		if atTarget || cacheApplied || result.Actual != nil || result.Status != volumeUnverified {
			t.Fatalf("matching stale readback result = %+v, atTarget=%v cacheApplied=%v", result, atTarget, cacheApplied)
		}
		if conn.Status().Volume.ActualVolume != 77 {
			t.Fatalf("rejected readback replaced event cache: %+v", conn.Status().Volume)
		}
		if _, posts, gets := speaker.operations(); fmt.Sprint(posts) != "[40]" || gets != 3 {
			t.Fatalf("readback operations = posts %v gets %d", posts, gets)
		}
	})
}

// TestZoneVolumeUnreachableMasterIsBadGateway: when the master's /getZone
// cannot be read, nothing says the topology changed, so the answer is an
// upstream failure (502), not a conflict (409). A zone the master reports as
// dissolved stays a conflict.
func TestZoneVolumeUnreachableMasterIsBadGateway(t *testing.T) {
	for _, test := range []struct {
		name    string
		zoneXML string
		down    bool
		want    int
	}{
		{name: "master unreachable", down: true, want: http.StatusBadGateway},
		{name: "zone dissolved", zoneXML: `<zone />`, want: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			zone, _ := testZone()
			masterSpeaker := newVolumeTestSpeaker(t, 20, test.zoneXML)
			memberSpeaker := newVolumeTestSpeaker(t, 35, "")
			app := NewWebApp()
			addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 20, nil, zone)
			addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", memberSpeaker, 35, nil, nil)
			if test.down {
				masterSpeaker.server.Close()
			}

			response := httptest.NewRecorder()
			app.HandleZoneVolume(response, withChiParams(httptest.NewRequest(http.MethodPost, "/zone/volume/40", nil),
				map[string]string{"id": "192.0.2.10", "volume": "40"}))
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body.String())
			}

			if _, posts, _ := memberSpeaker.operations(); len(posts) != 0 {
				t.Fatalf("member was written: %v", posts)
			}
		})
	}
}
