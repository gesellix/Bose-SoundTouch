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
			if !speaker.ignoreWrites {
				speaker.volume = request.Value
			}
			speaker.posts = append(speaker.posts, request.Value)
			postHook := speaker.onVolumePost
			speaker.mu.Unlock()
			if postHook != nil {
				postHook()
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/volume":
			speaker.mu.Lock()
			speaker.volumeGets++
			get := speaker.volumeGets
			volume := speaker.volume
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
	app.volumeReadbackRetryWait = func(delay time.Duration) {
		if delay != 50*time.Millisecond {
			t.Errorf("retry delay = %s", delay)
		}
	}
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
	if maxConcurrentWrites != 1 {
		t.Fatalf("concurrent target writes = %d, want serialized writes", maxConcurrentWrites)
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
		!strings.Contains(mismatch.Error, "does not both match requested") {
		t.Fatalf("partial member result = %+v", mismatch)
	}
}

func TestStandaloneVolumeRoutesReturnAuthoritativeReadback(t *testing.T) {
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
				"SoundTouch 20",
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
			if !payload.Success || payload.Data.Requested != test.level || payload.Data.Actual != test.level {
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

func TestHandleZoneVolumeWaitsForLaggingReadbackWithoutRepeatingWrite(t *testing.T) {
	zone, zoneXML := testZone()
	masterSpeaker := newVolumeTestSpeaker(t, 20, zoneXML)
	memberSpeaker := newVolumeTestSpeaker(t, 35, "")
	memberSpeaker.ignoreWrites = true
	memberSpeaker.onVolumeGet = func(get int) {
		if get != 2 {
			return
		}

		memberSpeaker.mu.Lock()
		memberSpeaker.volume = 40
		memberSpeaker.mu.Unlock()
	}

	app := NewWebApp()
	app.volumeReadbackRetryWait = func(delay time.Duration) {
		if delay != zoneVolumeReadbackRetryDelay {
			t.Errorf("retry delay = %s", delay)
		}
	}
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 20, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", memberSpeaker, 35, nil, nil)

	request := httptest.NewRequest(http.MethodPost, "/zone/volume/40", nil)
	request = withChiParams(request, map[string]string{"id": "192.0.2.10", "volume": "40"})
	response := httptest.NewRecorder()
	app.HandleZoneVolume(response, request)

	var payload struct {
		Data zoneVolumeResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK || payload.Data.Partial {
		t.Fatalf("lagging readback result: status=%d data=%+v", response.Code, payload.Data)
	}
	if volume, posts, gets := memberSpeaker.operations(); volume != 40 || fmt.Sprint(posts) != "[40]" || gets != 3 {
		t.Fatalf("member operations = volume %d posts %v gets %d", volume, posts, gets)
	}

	for _, member := range payload.Data.Members {
		if member.DeviceID == "MEMBER" &&
			(member.Actual == nil || *member.Actual != 40 || member.Error != "") {
			t.Fatalf("converged member result = %+v", member)
		}
	}
}

func TestApplyVolumeTargetRejectsStaleGenerationsWithoutActual(t *testing.T) {
	speaker := newVolumeTestSpeaker(t, 10, "")
	app := NewWebApp()
	app.volumeReadbackRetryWait = func(time.Duration) {}
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
		if atTarget || confirmed || result.Actual != nil {
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
		atTarget, confirmed := app.applyVolumeTarget(&result, "192.0.2.30", conn, topology, nil, 40)
		if atTarget || confirmed || result.Actual != nil {
			t.Fatalf("stale readback result = %+v, atTarget=%v confirmed=%v", result, atTarget, confirmed)
		}
		if conn.Status().Volume.ActualVolume != 77 {
			t.Fatalf("rejected readback replaced event cache: %+v", conn.Status().Volume)
		}
		if _, posts, gets := speaker.operations(); fmt.Sprint(posts) != "[40]" || gets != 3 {
			t.Fatalf("readback operations = posts %v gets %d", posts, gets)
		}
	})
}

func TestInvalidVolumeReadbackCannotConfirmZeroOrReplaceCachedVolume(t *testing.T) {
	for _, body := range []string{
		`<volume/>`,
		`<volume><targetvolume>0</targetvolume></volume>`,
		`<volume><actualvolume>0</actualvolume></volume>`,
		`<volume><targetvolume/><actualvolume>0</actualvolume></volume>`,
	} {
		t.Run(body, func(t *testing.T) {
			speaker := newVolumeTestSpeaker(t, 20, "")
			speaker.volumeResponse = body
			app := NewWebApp()
			conn := addVolumeTestDevice(app, "speaker", "SPEAKER", "Office", "SoundTouch 20", speaker, 20, nil, nil)
			topology, current := conn.SnapshotGroupTopology()
			if !current {
				t.Fatal("initial group topology was not confirmed")
			}
			result := app.applyZoneMemberVolume(conn, zoneMemberView{
				ControlID: "speaker", HardwareID: "SPEAKER", Name: "Office",
			}, topology, nil, 0)
			member := result.Members[0]
			if !result.Partial || member.Actual != nil || !strings.Contains(member.Error, "readback volume") {
				t.Fatalf("invalid readback result = %+v", result)
			}
			if cached := conn.Status().Volume; cached == nil || cached.TargetVolume != 20 || cached.ActualVolume != 20 {
				t.Fatalf("invalid readback changed cache: %+v", cached)
			}
			if _, posts, gets := speaker.operations(); fmt.Sprint(posts) != "[0]" || gets != 1 {
				t.Fatalf("operations = posts %v gets %d, want exactly one write and rejected readback", posts, gets)
			}
		})
	}
}
