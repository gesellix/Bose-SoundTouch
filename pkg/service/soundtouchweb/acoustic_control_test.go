package soundtouchweb

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gorilla/websocket"
)

const acousticTestCaps = `<bassCapabilities deviceID="MASTER"><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax><bassDefault>0</bassDefault></bassCapabilities>`
const acousticTestPairXML = `<group id="PAIR"><masterDeviceId>MASTER</masterDeviceId><roles><groupRole><deviceId>MASTER</deviceId><role>RIGHT</role></groupRole><groupRole><deviceId>PEER</deviceId><role>LEFT</role></groupRole></roles></group>`

func acousticBassDocument(hardware string, target, actual int) string {
	return fmt.Sprintf(`<bass deviceID="%s"><targetbass>%d</targetbass><actualbass>%d</actualbass></bass>`, hardware, target, actual)
}

func newAcousticTestConnection(t *testing.T, handler http.HandlerFunc) (*WebApp, *webtypes.DeviceConnection) {
	t.Helper()
	speaker := httptest.NewServer(handler)
	t.Cleanup(speaker.Close)
	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(client.NewClientFromHost(speaker.URL), &models.DeviceInfo{DeviceID: "MASTER", Type: "SoundTouch 10"})
	conn.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.Bass = &models.Bass{DeviceID: "MASTER", TargetBass: -2, ActualBass: -2}
		status.BassCapabilities = &models.BassCapabilities{DeviceID: "MASTER", BassAvailable: true, BassMin: -9, BassMax: 0, BassDefault: 0}
	})
	app.AddDevice("speaker", conn)
	t.Cleanup(conn.Close)
	return app, conn
}

func acousticRequest(app *WebApp, action, body, hardware, group string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/control/devices/speaker/action/"+action, strings.NewReader(body))
	request = withChiParams(request, map[string]string{"id": "speaker", "action": action})
	request.Header.Set(acousticDeviceTargetHeader, hardware)
	request.Header.Set(acousticGroupTargetHeader, group)
	response := httptest.NewRecorder()
	app.HandleAPIControl(response, request)
	return response
}

func awaitAcoustic(t *testing.T, ready <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestAcousticBassControlRequiresVerifiedReadbackAndOneWrite(t *testing.T) {
	tests := []struct {
		name, hardware          string
		target, actual          int
		malformed, writeFailure bool
		code                    int
		changed                 bool
	}{
		{name: "confirmed", hardware: "MASTER", target: -3, actual: -3, code: 200, changed: true},
		{name: "target mismatch", hardware: "MASTER", target: -4, actual: -4, code: 409, changed: true},
		{name: "actual mismatch", hardware: "MASTER", target: -3, actual: -2, code: 409, changed: true},
		{name: "wrong hardware", hardware: "OTHER", target: -3, actual: -3, code: 502},
		{name: "missing hardware", target: -3, actual: -3, code: 502},
		{name: "outside capabilities", hardware: "MASTER", target: 1, actual: 1, code: 502},
		{name: "missing actual", malformed: true, code: 502},
		{name: "uncertain write confirmed by read", hardware: "MASTER", target: -3, actual: -3, writeFailure: true, code: 200, changed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var writes atomic.Int32
			app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/info":
					fmt.Fprint(w, `<info deviceID="MASTER"><type>SoundTouch 10</type></info>`)
				case "/bass":
					if r.Method == http.MethodPost {
						writes.Add(1)
						if test.writeFailure {
							http.Error(w, "uncertain", http.StatusBadGateway)
						} else {
							fmt.Fprint(w, `<bass/>`)
						}
						return
					}
					if test.malformed {
						fmt.Fprint(w, `<bass deviceID="MASTER"><targetbass>-3</targetbass></bass>`)
					} else {
						fmt.Fprint(w, acousticBassDocument(test.hardware, test.target, test.actual))
					}
				default:
					http.NotFound(w, r)
				}
			})
			old := conn.Status().Bass
			response := acousticRequest(app, "bass", `{"level":-3}`, "MASTER", "")
			if response.Code != test.code {
				t.Fatalf("code=%d body=%s, want %d", response.Code, response.Body.String(), test.code)
			}
			if writes.Load() != 1 {
				t.Fatalf("writes=%d, want exactly one", writes.Load())
			}
			if test.changed {
				if conn.Status().Bass == old || conn.Status().BassRevision == 0 {
					t.Fatal("valid readback not projected")
				}
			} else if conn.Status().Bass != old {
				t.Fatal("invalid readback replaced last verified value")
			}
			if response.Code == 200 {
				var envelope struct {
					Success bool                  `json:"success"`
					Data    acousticControlResult `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if !envelope.Success || !envelope.Data.AtTarget || envelope.Data.Message != "Bass set to -3" {
					t.Fatalf("response=%s", response.Body.String())
				}
			}
		})
	}
}

func TestAcousticBassRejectsStaleIdentityAndAdvertisedRangeBeforeWrite(t *testing.T) {
	var calls atomic.Int32
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); http.Error(w, "unexpected", 500) })
	for _, test := range []struct {
		body, identity string
		code           int
	}{{`{"level":-3}`, "OLD", 409}, {`{"level":1}`, "MASTER", 400}} {
		response := acousticRequest(app, "bass", test.body, test.identity, "")
		if response.Code != test.code {
			t.Fatalf("code=%d body=%s", response.Code, response.Body.String())
		}
	}
	if calls.Load() != 0 || conn.Status().Bass.ActualBass != -2 {
		t.Fatal("rejected target performed I/O or changed projection")
	}
}

func TestAcousticBassHintDuringBlockedReadRejectsOldResultAndRetainsValueOnTrailingFailure(t *testing.T) {
	started, release, trailing := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var reads atomic.Int32
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bassCapabilities" {
			fmt.Fprint(w, acousticTestCaps)
			return
		}
		if r.URL.Path == "/bass" {
			if reads.Add(1) == 1 {
				close(started)
				<-release
				fmt.Fprint(w, acousticBassDocument("MASTER", -6, -6))
			} else {
				close(trailing)
				http.Error(w, "temporarily unavailable", http.StatusBadGateway)
			}
			return
		}
		http.NotFound(w, r)
	})
	original := conn.Status().Bass
	revision := conn.Status().BassRevision
	app.refreshBass("speaker", conn)
	awaitAcoustic(t, started, "blocked initial bass read")
	for range 20 {
		app.refreshBass("speaker", conn)
	}
	unblock()
	awaitAcoustic(t, trailing, "coalesced trailing bass read")
	if got := conn.Status(); got.Bass != original || got.BassRevision != revision {
		t.Fatalf("invalidated read overwrote verified value: %+v", got.Bass)
	}
	if reads.Load() != 2 {
		t.Fatalf("read count=%d, want one active plus one trailing", reads.Load())
	}
}

func TestAcousticStartupGroupPollTriggersBalanceWithoutWebSocket(t *testing.T) {
	read := make(chan struct{})
	var once sync.Once
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getGroup":
			fmt.Fprint(w, acousticTestPairXML)
		case "/balance":
			once.Do(func() { close(read) })
			fmt.Fprint(w, `<balance deviceID="MASTER"><balanceAvailable>true</balanceAvailable><balanceMin>-7</balanceMin><balanceMax>7</balanceMax><balanceDefault>0</balanceDefault><targetBalance>3</targetBalance><actualBalance>3</actualBalance></balance>`)
		case "/bassCapabilities":
			fmt.Fprint(w, acousticTestCaps)
		case "/bass":
			fmt.Fprint(w, acousticBassDocument("MASTER", -2, -2))
		default:
			http.NotFound(w, r)
		}
	})
	app.UpdateDeviceStatus("speaker", conn)
	awaitAcoustic(t, read, "startup balance read after confirmed group")
	deadline := time.Now().Add(2 * time.Second)
	for conn.Status().Balance == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if conn.Status().Balance == nil || conn.Status().Balance.Actual != 3 || conn.CurrentWebSocket() != nil {
		t.Fatal("startup HTTP balance did not converge independently of WebSocket")
	}
	// An unchanged accepted group poll must schedule another authoritative
	// read too, rather than relying on groupUpdated only firing on changes.
	previous := conn.Status().BalanceRevision
	app.UpdateDeviceStatus("speaker", conn)
	deadline = time.Now().Add(2 * time.Second)
	for conn.Status().BalanceRevision <= previous && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if conn.Status().BalanceRevision <= previous {
		t.Fatal("unchanged accepted group did not refresh balance")
	}
}

func TestAcousticBalanceRejectsOrdinaryMemberAndStalePair(t *testing.T) {
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "unexpected", 500) })
	response := acousticRequest(app, "balance", `{"level":3}`, "MASTER", "")
	if response.Code != 409 {
		t.Fatalf("ordinary speaker code=%d", response.Code)
	}
	conn.ApplyGroupEvent(&models.Group{ID: "PAIR", MasterDeviceID: "MASTER", Roles: models.GroupRoles{Roles: []models.GroupRole{{DeviceID: "MASTER", Role: "RIGHT"}, {DeviceID: "PEER", Role: "LEFT"}}}}, time.Now())
	response = acousticRequest(app, "balance", `{"level":3}`, "MASTER", "OLD-PAIR")
	if response.Code != 409 {
		t.Fatalf("stale pair code=%d", response.Code)
	}
	response = acousticRequest(app, "balance", `{"level":3}`, "MASTER", "PAIR")
	if response.Code != 503 {
		t.Fatalf("confirmed RIGHT master without socket code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAcousticBalanceHintBeforeEchoReconcilesOverWebSocketWithoutReplay(t *testing.T) {
	releaseHTTP := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releaseHTTP) }) }
	defer unblock()
	var writes, wsReads atomic.Int32
	balance := `<balance deviceID="MASTER"><balanceAvailable>true</balanceAvailable><balanceMin>-7</balanceMin><balanceMax>7</balanceMax><balanceDefault>0</balanceDefault><targetBalance>-3</targetBalance><actualBalance>-3</actualBalance></balance>`
	upgrader := websocket.Upgrader{Subprotocols: []string{"gabbo"}, CheckOrigin: func(*http.Request) bool { return true }}
	listener, err := net.Listen("tcp", "127.0.0.2:8080")
	if err != nil {
		t.Fatalf("local speaker fixture listener: %v", err)
	}
	speaker := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			fmt.Fprint(w, `<info deviceID="MASTER"><type>SoundTouch 10</type></info>`)
			return
		}
		if r.URL.Path == "/balance" {
			<-releaseHTTP
			fmt.Fprint(w, balance)
			return
		}
		remote, upgradeErr := upgrader.Upgrade(w, r, nil)
		if upgradeErr != nil {
			return
		}
		defer remote.Close()
		for {
			_, frame, readErr := remote.ReadMessage()
			if readErr != nil {
				return
			}
			text := string(frame)
			requestID := ""
			if start := strings.Index(text, `requestID="`); start >= 0 {
				rest := text[start+len(`requestID="`):]
				requestID = strings.SplitN(rest, `"`, 2)[0]
			}
			if strings.Contains(text, `method="POST"`) {
				writes.Add(1)
				if err := remote.WriteMessage(websocket.TextMessage, []byte(`<updates deviceID="MASTER"><balanceUpdated><balance><actualBalance>7</actualBalance></balance></balanceUpdated></updates>`)); err != nil {
					return
				}
			} else {
				wsReads.Add(1)
			}
			response := fmt.Sprintf(`<msg><header deviceID="MASTER" url="balance" method="GET"><request requestID="%s" msgType="RESPONSE"><info type="new"/></request></header><body>%s</body></msg>`, requestID, balance)
			if err := remote.WriteMessage(websocket.TextMessage, []byte(response)); err != nil {
				return
			}
		}
	}))
	speaker.Listener.Close()
	speaker.Listener = listener
	speaker.Start()
	t.Cleanup(speaker.Close)
	conn := webtypes.NewDeviceConnection(client.NewClientFromHost(speaker.URL), &models.DeviceInfo{DeviceID: "MASTER", Type: "SoundTouch 10"})
	app := NewWebApp()
	app.AddDevice("speaker", conn)
	t.Cleanup(conn.Close)
	conn.ApplyGroupEvent(&models.Group{ID: "PAIR", MasterDeviceID: "MASTER", Roles: models.GroupRoles{Roles: []models.GroupRole{{DeviceID: "MASTER", Role: "RIGHT"}, {DeviceID: "PEER", Role: "LEFT"}}}}, time.Time{})
	conn.ApplyFieldEvent(webtypes.FieldBalance, func(status *webtypes.DeviceStatus) {
		status.Balance = &models.Balance{DeviceID: "MASTER", Available: true, Min: -7, Max: 7, Default: 0, Target: 0, Actual: 0}
	})
	ws := conn.Client.NewWebSocketClient(nil)
	ws.OnBalanceUpdated(func(*models.BalanceUpdatedEvent) { app.refreshBalance("speaker", conn) })
	if err := ws.Connect(); err != nil {
		t.Fatal(err)
	}
	conn.SetWebSocket(ws)
	response := acousticRequest(app, "balance", `{"level":-3}`, "MASTER", "PAIR")
	if response.Code != 200 {
		t.Fatalf("code=%d body=%s", response.Code, response.Body.String())
	}
	if writes.Load() != 1 || wsReads.Load() != 1 {
		t.Fatalf("POST=%d WS GET=%d, want one each", writes.Load(), wsReads.Load())
	}
	if got := conn.Status(); got.Balance == nil || got.Balance.Actual != -3 || got.BalanceRevision == 0 {
		t.Fatalf("authoritative balance=%+v", got.Balance)
	}
	var envelope struct {
		Data acousticControlResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.AtTarget || envelope.Data.Message != "Balance set to -3" || envelope.Data.BalanceRevision != conn.Status().BalanceRevision {
		t.Fatalf("response=%s", response.Body.String())
	}
	unblock()
}

func TestAcousticPollRejectsBassWhenCapabilityIdentityIsWrong(t *testing.T) {
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bassCapabilities":
			fmt.Fprint(w, strings.ReplaceAll(acousticTestCaps, "MASTER", "OTHER"))
		case "/bass":
			fmt.Fprint(w, acousticBassDocument("MASTER", -7, -7))
		case "/getGroup":
			fmt.Fprint(w, `<group/>`)
		default:
			http.NotFound(w, r)
		}
	})
	previous := conn.Status().Bass
	revision := conn.Status().BassRevision
	app.UpdateDeviceStatus("speaker", conn)
	if conn.Status().Bass != previous || conn.Status().BassRevision != revision {
		t.Fatal("wrong capability identity permitted bass projection")
	}
}

func TestAcousticBalanceRefreshSurvivesTopologyRevalidationDuringRead(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var reads atomic.Int32
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/balance" {
			http.NotFound(w, r)
			return
		}
		actual := 3
		if reads.Add(1) == 1 {
			close(started)
			<-release
			actual = 7
		}
		fmt.Fprintf(w, `<balance deviceID="MASTER"><balanceAvailable>true</balanceAvailable><balanceMin>-7</balanceMin><balanceMax>7</balanceMax><balanceDefault>0</balanceDefault><targetBalance>%d</targetBalance><actualBalance>%d</actualBalance></balance>`, actual, actual)
	})
	group := &models.Group{ID: "PAIR", MasterDeviceID: "MASTER", Roles: models.GroupRoles{Roles: []models.GroupRole{{DeviceID: "MASTER", Role: "RIGHT"}, {DeviceID: "PEER", Role: "LEFT"}}}}
	conn.ApplyGroupEvent(group, time.Time{})
	conn.ApplyFieldEvent(webtypes.FieldBalance, func(status *webtypes.DeviceStatus) {
		status.Balance = &models.Balance{DeviceID: "MASTER", Available: true, Min: -7, Max: 7, Target: 1, Actual: 1}
	})
	app.refreshBalance("speaker", conn)
	awaitAcoustic(t, started, "initial balance read")
	generation := conn.BeginGroupRefresh()
	app.refreshBalance("speaker", conn)
	if _, ok := conn.ConfirmedBalanceTarget(); ok {
		t.Fatal("pending topology admits balance")
	}
	conn.ApplyPolledGroup(generation, group)
	app.refreshBalance("speaker", conn)
	unblock()
	deadline := time.Now().Add(2 * time.Second)
	for conn.Status().Balance.Actual != 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := conn.Status().Balance.Actual; got != 3 {
		t.Fatalf("balance=%d, want latest recertified read", got)
	}
	previous := conn.Status().BalanceRevision
	app.refreshBalance("speaker", conn)
	deadline = time.Now().Add(2 * time.Second)
	for conn.Status().BalanceRevision <= previous && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if conn.Status().BalanceRevision <= previous {
		t.Fatal("coalescer lost worker after revalidation")
	}
}

func TestAcousticUnavailableBassCapabilityDisablesWritesWithoutInventingValue(t *testing.T) {
	var writes atomic.Int32
	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writes.Add(1)
		}
		switch r.URL.Path {
		case "/bassCapabilities":
			fmt.Fprint(w, `<bassCapabilities deviceID="MASTER"><bassAvailable>false</bassAvailable></bassCapabilities>`)
		case "/bass":
			http.Error(w, "unsupported", 404)
		case "/getGroup":
			fmt.Fprint(w, `<group/>`)
		default:
			http.NotFound(w, r)
		}
	})
	previous := conn.Status().Bass
	app.UpdateDeviceStatus("speaker", conn)
	if conn.Status().BassCapabilities == nil || conn.Status().BassCapabilities.BassAvailable || conn.Status().Bass != previous {
		t.Fatal("unavailable capability was not projected while retaining verified value")
	}
	response := acousticRequest(app, "bass", `{"level":-3}`, "MASTER", "")
	if response.Code != 409 || writes.Load() != 0 {
		t.Fatalf("code=%d writes=%d, unavailable bass must not write", response.Code, writes.Load())
	}
}
