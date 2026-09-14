package soundtouchweb

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
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
)

type settingsSpeakerFixture struct {
	server *httptest.Server

	mu                    sync.Mutex
	clockEnabled          bool
	clockFormat           string
	clockTimeZone         string
	clockHasEnabled       bool
	clockHasFormat        bool
	clockHasZone          bool
	clockHasTime          bool
	timeoutEnabled        bool
	language              int
	syncMode              string
	sourceName            string
	discoverable          bool
	pairingConfirmAfter   int32
	nowPlayingSource      string
	nowPlayingStatus      int
	legacyBluetoothURLs   bool
	pairingMutation       string
	clearMutation         string
	liveDeviceID          string
	liveDeviceIDs         []string
	endpointDeviceID      string
	timeoutReadFail       bool
	timeoutMutationFail   bool
	timeoutMutationReject bool
	languageReadFail      bool
	syncReadFail          bool
	infoStarted           chan struct{}
	infoRelease           chan struct{}

	clockPosts          atomic.Int32
	clockTimePosts      atomic.Int32
	timeoutPosts        atomic.Int32
	languagePosts       atomic.Int32
	syncPosts           atomic.Int32
	infoGets            atomic.Int32
	clearGets           atomic.Int32
	capabilityGets      atomic.Int32
	pairingGets         atomic.Int32
	pairingReads        atomic.Int32
	capabilityFailAfter int32
}

func newSettingsSpeakerFixture(t *testing.T, advertiseSettings bool) *settingsSpeakerFixture {
	t.Helper()

	fixture := &settingsSpeakerFixture{
		clockEnabled:     false,
		clockFormat:      "TIME_FORMAT_24HOUR_ID",
		clockTimeZone:    "Europe/Prague",
		clockHasEnabled:  true,
		clockHasFormat:   true,
		clockHasZone:     true,
		clockHasTime:     true,
		timeoutEnabled:   true,
		language:         15,
		syncMode:         "SYNC_TO_ZONE",
		sourceName:       "Line in",
		nowPlayingSource: "BLUETOOTH",
		nowPlayingStatus: http.StatusOK,
		liveDeviceID:     "AABBCCDDEEFF",
	}

	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()

		w.Header().Set("Content-Type", "application/xml")

		switch r.URL.Path {
		case "/info":
			infoGet := int(fixture.infoGets.Add(1))
			if fixture.infoStarted != nil {
				select {
				case fixture.infoStarted <- struct{}{}:
				default:
				}
			}
			if fixture.infoRelease != nil {
				<-fixture.infoRelease
			}
			liveDeviceID := fixture.liveDeviceID
			if infoGet <= len(fixture.liveDeviceIDs) {
				liveDeviceID = fixture.liveDeviceIDs[infoGet-1]
			}
			_, _ = fmt.Fprintf(w, `<info deviceID="%s"><name>Test speaker</name><type>SoundTouch 20</type></info>`,
				liveDeviceID)
		case "/capabilities":
			capabilityGet := fixture.capabilityGets.Add(1)
			if fixture.capabilityFailAfter > 0 && capabilityGet >= fixture.capabilityFailAfter {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `<error>capabilities unavailable</error>`)

				return
			}
			if advertiseSettings {
				_, _ = fmt.Fprintf(w, `<capabilities deviceID="%s"><clockDisplay>true</clockDisplay>`+
					`<networkConfig><hostedWifiConfigWebPage hostedBy="BCO" generation="1" port="80">true</hostedWifiConfigWebPage></networkConfig>`+
					`<capability name="systemtimeout" url="/systemtimeout"/>`+
					`<capability name="rebroadcastlatencymode" url="/rebroadcastlatencymode"/>`+
					`</capabilities>`, fixture.endpointDeviceID)
			} else {
				_, _ = fmt.Fprintf(w, `<capabilities deviceID="%s"><clockDisplay>false</clockDisplay></capabilities>`,
					fixture.endpointDeviceID)
			}
		case "/supportedURLs":
			locations := []string{"/sources"}
			if advertiseSettings {
				locations = append(locations,
					"/clockDisplay", "/clockTime", "/systemtimeout", "/language",
					"/rebroadcastlatencymode", "/bluetoothInfo", "/nameSource",
				)
				if fixture.legacyBluetoothURLs {
					locations = append(locations, "/enterPairingMode", "/clearPairedList")
				} else {
					locations = append(locations, "/enterBluetoothPairing", "/clearBluetoothPaired")
				}
			}

			_, _ = io.WriteString(w, `<supportedURLs>`)
			for _, location := range locations {
				_, _ = fmt.Fprintf(w, `<URL location="%s"/>`, location)
			}
			_, _ = io.WriteString(w, `</supportedURLs>`)
		case "/sources":
			_, _ = fmt.Fprintf(w, `<sources><sourceItem source="BLUETOOTH" status="READY">Bluetooth</sourceItem>`+
				`<sourceItem source="AUX" sourceAccount="AUX1" status="READY" isLocal="true">%s</sourceItem></sources>`, fixture.sourceName)
		case "/clockDisplay":
			if r.Method == http.MethodPatch || r.Method == http.MethodPost {
				fixture.clockPosts.Add(1)
				body, _ := io.ReadAll(r.Body)
				text := string(body)
				if strings.Contains(text, `userEnable="true"`) {
					fixture.clockEnabled = true
				}
				if strings.Contains(text, `userEnable="false"`) {
					fixture.clockEnabled = false
				}
				if strings.Contains(text, `timeFormat="TIME_FORMAT_12HOUR_ID"`) {
					fixture.clockFormat = "TIME_FORMAT_12HOUR_ID"
				}
				if strings.Contains(text, `timeFormat="TIME_FORMAT_24HOUR_ID"`) {
					fixture.clockFormat = "TIME_FORMAT_24HOUR_ID"
				}
			}

			attributes := []string{`brightnessLevel="70"`}
			if fixture.clockHasEnabled {
				attributes = append(attributes, fmt.Sprintf(`userEnable="%t"`, fixture.clockEnabled))
			}
			if fixture.clockHasZone {
				attributes = append(attributes, fmt.Sprintf(`timezoneInfo="%s"`, fixture.clockTimeZone))
			}
			if fixture.clockHasFormat {
				attributes = append(attributes, fmt.Sprintf(`timeFormat="%s"`, fixture.clockFormat))
			}
			_, _ = fmt.Fprintf(w, `<clockDisplay><clockConfig %s/></clockDisplay>`, strings.Join(attributes, " "))
		case "/clockTime":
			if r.Method == http.MethodPost {
				fixture.clockTimePosts.Add(1)
			}
			if fixture.clockHasTime {
				_, _ = fmt.Fprintf(w, `<clockTime utcTime="%d"/>`, time.Now().Unix())
			} else {
				_, _ = io.WriteString(w, `<clockTime/>`)
			}
		case "/systemtimeout":
			if r.Method == http.MethodGet && fixture.timeoutReadFail {
				http.Error(w, "system timeout unavailable", http.StatusServiceUnavailable)

				return
			}
			if r.Method == http.MethodPost {
				fixture.timeoutPosts.Add(1)
				body, _ := io.ReadAll(r.Body)
				var request models.SystemTimeout
				if err := xml.Unmarshal(body, &request); err != nil {
					http.Error(w, "invalid systemtimeout body", http.StatusBadRequest)

					return
				}
				if fixture.timeoutMutationReject {
					w.WriteHeader(http.StatusConflict)
					_, _ = io.WriteString(w, `<errors deviceID="AABBCCDDEEFF"><error value="1029" name="UNKNOWN_ACTION_ERROR">rejected</error></errors>`)

					return
				}
				fixture.timeoutEnabled = request.PowerSavingEnabled
				if fixture.timeoutMutationFail {
					http.Error(w, "response lost", http.StatusInternalServerError)

					return
				}
			}
			_, _ = fmt.Fprintf(w, `<systemtimeout><powersaving_enabled>%t</powersaving_enabled></systemtimeout>`, fixture.timeoutEnabled)
		case "/language":
			if r.Method == http.MethodGet && fixture.languageReadFail {
				http.Error(w, "language unavailable", http.StatusServiceUnavailable)

				return
			}
			if r.Method == http.MethodPost {
				fixture.languagePosts.Add(1)
				body, _ := io.ReadAll(r.Body)
				_, _ = fmt.Sscanf(string(body), `<sysLanguage>%d</sysLanguage>`, &fixture.language)
			}
			_, _ = fmt.Fprintf(w, `<sysLanguage>%d</sysLanguage>`, fixture.language)
		case "/rebroadcastlatencymode":
			if r.Method == http.MethodGet && fixture.syncReadFail {
				http.Error(w, "sync unavailable", http.StatusServiceUnavailable)

				return
			}
			if r.Method == http.MethodPost {
				fixture.syncPosts.Add(1)
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), `SYNC_TO_ROOM`) {
					fixture.syncMode = "SYNC_TO_ROOM"
				} else {
					fixture.syncMode = "SYNC_TO_ZONE"
				}
			}
			_, _ = fmt.Fprintf(w, `<rebroadcastlatencymode mode="%s" controllable="true"/>`, fixture.syncMode)
		case "/bluetoothInfo":
			_, _ = io.WriteString(w, `<BluetoothInfo BluetoothMACAddress="AA:BB:CC:DD:EE:FF"/>`)
		case "/enterBluetoothPairing":
			fixture.pairingGets.Add(1)
			switch fixture.pairingMutation {
			case "unknown-confirmed":
				fixture.discoverable = true
				http.Error(w, "response lost", http.StatusInternalServerError)
			case "unknown-unconfirmed":
				http.Error(w, "response lost", http.StatusInternalServerError)
			case "typed-error":
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `<errors deviceID="AABBCCDDEEFF"><error value="1029" name="UNKNOWN_ACTION_ERROR">rejected</error></errors>`)
			default:
				fixture.discoverable = true
				_, _ = io.WriteString(w, `<status>/enterBluetoothPairing</status>`)
			}
		case "/clearBluetoothPaired":
			fixture.clearGets.Add(1)
			switch fixture.clearMutation {
			case "unknown":
				http.Error(w, "response lost", http.StatusInternalServerError)
			case "typed-error":
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `<errors deviceID="AABBCCDDEEFF"><error value="1029" name="UNKNOWN_ACTION_ERROR">rejected</error></errors>`)
			default:
				_, _ = io.WriteString(w, `<status>/clearBluetoothPaired</status>`)
			}
		case "/now_playing":
			if fixture.nowPlayingStatus != http.StatusOK {
				w.WriteHeader(fixture.nowPlayingStatus)
				_, _ = io.WriteString(w, `<error>now playing unavailable</error>`)

				return
			}

			status := "DISCONNECTED"
			pairingRead := int32(0)
			if fixture.pairingGets.Load() > 0 {
				pairingRead = fixture.pairingReads.Add(1)
			}
			if fixture.discoverable && (fixture.pairingConfirmAfter == 0 || pairingRead >= fixture.pairingConfirmAfter) {
				status = "DISCOVERABLE"
			}
			_, _ = fmt.Fprintf(w, `<nowPlaying source="%s"><connectionStatusInfo status="%s" deviceName="Test phone"/></nowPlaying>`, fixture.nowPlayingSource, status)
		case "/nameSource":
			body, _ := io.ReadAll(r.Body)
			var request models.SourceRenameRequest
			if err := xml.Unmarshal(body, &request); err != nil {
				http.Error(w, "invalid nameSource body", http.StatusBadRequest)

				return
			}
			fixture.sourceName = request.ItemName
			_, _ = io.WriteString(w, `<status>/nameSource</status>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(fixture.server.Close)

	return fixture
}

func settingsTestApp(fixture *settingsSpeakerFixture) *WebApp {
	app := NewWebApp()
	info := &models.DeviceInfo{
		DeviceID: "AABBCCDDEEFF",
		Name:     "Test speaker",
		Type:     "SoundTouch 20",
		NetworkInfo: []models.NetworkInfo{
			{IPAddress: "192.0.2.10"},
		},
	}
	connection := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: fixture.server.URL}), info)
	connection.SetStatus(&webtypes.DeviceStatus{NowPlaying: &models.NowPlaying{Source: "STANDBY"}})
	app.AddDevice("speaker", connection)
	app.OnboardingURL = "/setup/"

	return app
}

func settingsRequest(method, path, body string) *http.Request {
	return settingsRequestForDevice("speaker", method, path, body)
}

func settingsRequestForDevice(deviceID, method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set(settingsTargetHeader, "AABBCCDDEEFF")
	}

	return withChiParams(request, map[string]string{"id": deviceID})
}

func withBluetoothPairingPoll(request *http.Request, attempts int) *http.Request {
	strategy := bluetoothPairingPollStrategy{
		Attempts: attempts,
		Wait:     func() error { return nil },
	}

	return request.WithContext(context.WithValue(request.Context(), bluetoothPairingPollStrategyKey{}, strategy))
}

func decodeSettingsResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Success bool                   `json:"success"`
	Outcome string                 `json:"outcome"`
	Error   string                 `json:"error"`
	Warning string                 `json:"warning"`
	Data    deviceSettingsSnapshot `json:"data"`
} {
	t.Helper()

	var response struct {
		Success bool                   `json:"success"`
		Outcome string                 `json:"outcome"`
		Error   string                 `json:"error"`
		Warning string                 `json:"warning"`
		Data    deviceSettingsSnapshot `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}

	return response
}

func TestHandleGetDeviceSettingsProjectsOnlyAdvertisedControls(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(recorder, settingsRequest(http.MethodGet, "/api/control/devices/speaker/settings", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	responseBody := recorder.Body.String()
	response := decodeSettingsResponse(t, recorder)
	if !response.Success {
		t.Fatalf("settings response failed: %s", response.Error)
	}
	if response.Data.TargetIdentity != "AABBCCDDEEFF" {
		t.Fatalf("settings target identity = %q, want AABBCCDDEEFF", response.Data.TargetIdentity)
	}

	support := response.Data.Support
	if !support.ClockDisplay || !support.ClockTime || !support.SystemTimeout || !support.Language || !support.Sync {
		t.Fatalf("missing supported system controls: %+v", support)
	}
	if response.Data.ClockDisplay == nil || response.Data.ClockDisplay.Enabled == nil ||
		*response.Data.ClockDisplay.Enabled ||
		response.Data.ClockDisplay.Format != "24" || response.Data.ClockDisplay.TimeZone != "Europe/Prague" {
		t.Fatalf("unexpected clock-display projection: %+v", response.Data.ClockDisplay)
	}
	if response.Data.ClockTime == nil || response.Data.ClockTime.UTC == 0 {
		t.Fatalf("unexpected clock-time projection: %+v", response.Data.ClockTime)
	}
	if response.Data.SystemTimeout == nil || !response.Data.SystemTimeout.Enabled {
		t.Fatalf("unexpected automatic-standby projection: %+v", response.Data.SystemTimeout)
	}
	if !support.Bluetooth || !support.BluetoothPair || !support.BluetoothClear || !support.SourceNaming {
		t.Fatalf("missing supported source controls: %+v", support)
	}
	if response.Data.Language == nil || response.Data.Language.Code != 15 || len(response.Data.Language.Options) != 24 {
		t.Fatalf("unexpected language projection: %+v", response.Data.Language)
	}
	if response.Data.Sync == nil || response.Data.Sync.Mode != "SYNC_TO_ZONE" {
		t.Fatalf("unexpected sync projection: %+v", response.Data.Sync)
	}
	if response.Data.Bluetooth == nil || response.Data.Bluetooth.MACAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("unexpected Bluetooth projection: %+v", response.Data.Bluetooth)
	}
	if len(response.Data.Sources) != 1 || response.Data.Sources[0].SourceAccount != "AUX1" {
		t.Fatalf("unexpected renameable sources: %+v", response.Data.Sources)
	}
	if !support.WiFiOnboarding || response.Data.OnboardingURL != "/setup/" {
		t.Fatalf("unexpected Wi-Fi onboarding projection: support=%+v url=%q",
			support, response.Data.OnboardingURL)
	}
	if strings.Contains(responseBody, `"network"`) {
		t.Fatalf("excluded network diagnostics were projected: %s", responseBody)
	}
}

func TestClockDisplayOmittedEnableStateRemainsNonWritable(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.clockHasEnabled = false
	app := settingsTestApp(fixture)
	getRecorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(getRecorder, settingsRequest(http.MethodGet,
		"/api/control/devices/speaker/settings", ""))

	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
	}
	responseBody := getRecorder.Body.String()
	response := decodeSettingsResponse(t, getRecorder)
	if response.Data.ClockDisplay == nil || response.Data.ClockDisplay.Enabled != nil {
		t.Fatalf("clock enabled projection = %+v, want omitted", response.Data.ClockDisplay)
	}
	var rawResponse struct {
		Data struct {
			ClockDisplay map[string]interface{} `json:"clockDisplay"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(responseBody), &rawResponse); err != nil {
		t.Fatalf("decode raw settings response: %v", err)
	}
	if _, present := rawResponse.Data.ClockDisplay["enabled"]; present {
		t.Fatalf("omitted firmware enable state became clock-display JSON data: %s", responseBody)
	}

	patchRecorder := httptest.NewRecorder()
	app.HandleSetClockDisplay(patchRecorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/clock-display", `{"enabled":false}`))

	if patchRecorder.Code != http.StatusConflict {
		t.Fatalf("PATCH status = %d, body = %s", patchRecorder.Code, patchRecorder.Body.String())
	}
	if fixture.clockPosts.Load() != 0 {
		t.Fatalf("clock writes = %d, want 0 without current enable readback", fixture.clockPosts.Load())
	}
	if fixture.infoGets.Load() != 4 {
		t.Fatalf("live identity reads = %d, want two fences for each settings read", fixture.infoGets.Load())
	}
}

func TestHandleGetDeviceSettingsOmitsWiFiOnboardingWithoutMountedWorkflow(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	app.OnboardingURL = ""
	recorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(recorder, settingsRequest(http.MethodGet,
		"/api/control/devices/speaker/settings", ""))

	response := decodeSettingsResponse(t, recorder)
	if recorder.Code != http.StatusOK || !response.Success {
		t.Fatalf("response = status %d %+v", recorder.Code, response)
	}
	if response.Data.OnboardingURL != "" {
		t.Fatalf("onboarding URL = %q, want omitted", response.Data.OnboardingURL)
	}
}

func TestHandleGetDeviceSettingsIgnoresGeneralPairingEndpoints(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.legacyBluetoothURLs = true
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(recorder, settingsRequest(http.MethodGet, "/api/control/devices/speaker/settings", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if !response.Data.Support.Bluetooth {
		t.Fatal("Bluetooth information should remain supported")
	}
	if response.Data.Support.BluetoothPair || response.Data.Support.BluetoothClear {
		t.Fatalf("general pairing endpoints enabled Bluetooth controls: %+v", response.Data.Support)
	}
}

func TestHandleGetDeviceSettingsPreservesUnknownCurrentLanguage(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.language = 99
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(recorder, settingsRequest(http.MethodGet,
		"/api/control/devices/speaker/settings", ""))
	response := decodeSettingsResponse(t, recorder)
	if recorder.Code != http.StatusOK || !response.Success || response.Data.Language == nil {
		t.Fatalf("response = status %d %+v", recorder.Code, response)
	}
	options := response.Data.Language.Options
	last := options[len(options)-1]
	if response.Data.Language.Code != 99 || last.Code != 99 || last.Name != "Unknown (99)" {
		t.Fatalf("unknown current language was not preserved: %+v", response.Data.Language)
	}
}

func TestSettingsMutationRequiresCurrentPhysicalTarget(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	if !app.RemoveDevice("speaker") {
		t.Fatal("remove original settings target")
	}

	replacement := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: fixture.server.URL}),
		&models.DeviceInfo{DeviceID: "FFEEDDCCBBAA", Name: "Replacement speaker"},
	)
	replacement.SetStatus(&webtypes.DeviceStatus{NowPlaying: &models.NowPlaying{Source: "STANDBY"}})
	if !app.AddDevice("speaker", replacement) {
		t.Fatal("add replacement settings target")
	}

	recorder := httptest.NewRecorder()
	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", `{"enabled":false}`))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.timeoutPosts.Load() != 0 {
		t.Fatalf("stale target sent %d automatic-standby mutations", fixture.timeoutPosts.Load())
	}
}

func TestSettingsMutationRejectsSameRouteWithDifferentLiveSpeaker(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.liveDeviceID = "FFEEDDCCBBAA"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", `{"enabled":false}`))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.infoGets.Load() != 1 {
		t.Fatalf("live identity reads = %d, want 1", fixture.infoGets.Load())
	}
	if fixture.timeoutPosts.Load() != 0 {
		t.Fatalf("replacement speaker received %d automatic-standby mutations", fixture.timeoutPosts.Load())
	}
}

func TestSettingsMutationDoesNotPinUnrelatedRegistryDuringNetworkIO(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.infoStarted = make(chan struct{}, 1)
	fixture.infoRelease = make(chan struct{})
	var releaseOnce sync.Once
	releaseInfo := func() { releaseOnce.Do(func() { close(fixture.infoRelease) }) }
	defer releaseInfo()
	app := settingsTestApp(fixture)
	other := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "112233445566"})
	if !app.AddDevice("other", other) {
		t.Fatal("add unrelated settings target")
	}
	recorder := httptest.NewRecorder()
	handlerDone := make(chan struct{})

	go func() {
		defer close(handlerDone)
		app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
			"/api/control/devices/speaker/settings/system-timeout", `{"enabled":false}`))
	}()

	select {
	case <-fixture.infoStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("mutation did not reach live identity validation")
	}

	removeDone := make(chan bool, 1)
	go func() {
		removeDone <- app.RemoveDevice("other")
	}()

	select {
	case removed := <-removeDone:
		if !removed {
			t.Fatal("unrelated settings target was not removed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("settings network I/O blocked registry removal")
	}

	releaseInfo()

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("settings mutation did not complete")
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.timeoutPosts.Load() != 1 {
		t.Fatalf("settings target received %d mutations, want 1", fixture.timeoutPosts.Load())
	}
}

func TestRemoveDeviceWaitsForPhysicalSettingsOperation(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	release := app.beginSettingsOperation("AABBCCDDEEFF")
	removeStarted := make(chan struct{})
	removeDone := make(chan bool, 1)

	go func() {
		close(removeStarted)
		removeDone <- app.RemoveDevice("speaker")
	}()
	<-removeStarted

	select {
	case <-removeDone:
		release()
		t.Fatal("device was detached during its physical settings operation")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case removed := <-removeDone:
		if !removed {
			t.Fatal("device was not removed after its settings operation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("device removal remained blocked after its settings operation")
	}
}

func TestSettingsOperationsSerializeAcrossConnectionsForSamePhysicalDevice(t *testing.T) {
	firstFixture := newSettingsSpeakerFixture(t, true)
	firstFixture.infoStarted = make(chan struct{}, 1)
	firstFixture.infoRelease = make(chan struct{})
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(firstFixture.infoRelease) }) }
	defer releaseFirst()

	secondFixture := newSettingsSpeakerFixture(t, true)
	secondFixture.infoStarted = make(chan struct{}, 1)
	app := settingsTestApp(firstFixture)
	second := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: secondFixture.server.URL}),
		&models.DeviceInfo{DeviceID: "AABBCCDDEEFF", Name: "Rediscovered speaker"},
	)
	second.SetStatus(&webtypes.DeviceStatus{NowPlaying: &models.NowPlaying{Source: "STANDBY"}})
	if !app.AddDevice("speaker-rediscovered", second) {
		t.Fatal("add rediscovered settings target")
	}

	firstRecorder := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		app.HandleSetSystemTimeout(firstRecorder, settingsRequest(http.MethodPatch,
			"/api/control/devices/speaker/settings/system-timeout", "{\"enabled\":false}"))
	}()
	select {
	case <-firstFixture.infoStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first settings operation did not reach the speaker")
	}

	secondRecorder := httptest.NewRecorder()
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		close(secondStarted)
		app.HandleSetSystemTimeout(secondRecorder, settingsRequestForDevice(
			"speaker-rediscovered", http.MethodPatch,
			"/api/control/devices/speaker-rediscovered/settings/system-timeout", "{\"enabled\":false}"))
	}()
	<-secondStarted

	select {
	case <-secondFixture.infoStarted:
		t.Fatal("same physical speaker accepted overlapping settings operations")
	case <-time.After(50 * time.Millisecond):
	}

	releaseFirst()
	for name, done := range map[string]<-chan struct{}{
		"first":  firstDone,
		"second": secondDone,
	} {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s settings operation did not complete", name)
		}
	}

	if firstRecorder.Code != http.StatusOK || secondRecorder.Code != http.StatusOK {
		t.Fatalf("settings statuses = %d/%d, bodies = %s / %s",
			firstRecorder.Code, secondRecorder.Code, firstRecorder.Body.String(), secondRecorder.Body.String())
	}
	if firstFixture.timeoutPosts.Load() != 1 || secondFixture.timeoutPosts.Load() != 1 {
		t.Fatalf("settings writes = %d/%d, want one per serialized connection",
			firstFixture.timeoutPosts.Load(), secondFixture.timeoutPosts.Load())
	}
}

func TestSettingsOperationLocksFollowPhysicalIdentity(t *testing.T) {
	app := NewWebApp()
	releaseA := app.beginSettingsOperation("AABBCCDDEEFF")

	app.settingsLocksMu.Lock()
	firstA := app.settingsLocks["AABBCCDDEEFF"]
	app.settingsLocksMu.Unlock()
	if firstA == nil {
		t.Fatal("physical settings lock was not registered")
	}
	if firstA.TryLock() {
		firstA.Unlock()
		t.Fatal("physical settings operation did not hold its identity lock")
	}

	releaseB := app.beginSettingsOperation("FFEEDDCCBBAA")
	app.settingsLocksMu.Lock()
	firstB := app.settingsLocks["FFEEDDCCBBAA"]
	app.settingsLocksMu.Unlock()
	if firstB == nil || firstB == firstA {
		t.Fatal("different physical speakers did not receive independent settings locks")
	}
	releaseB()
	releaseA()

	releaseSecondA := app.beginSettingsOperation("AABBCCDDEEFF")
	app.settingsLocksMu.Lock()
	secondA := app.settingsLocks["AABBCCDDEEFF"]
	app.settingsLocksMu.Unlock()
	releaseSecondA()
	if secondA != firstA {
		t.Fatal("settings lock was not retained across connection generations")
	}
}

func TestGetSettingsRejectsIdentityChangeDuringSnapshot(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.liveDeviceIDs = []string{"AABBCCDDEEFF", "FFEEDDCCBBAA"}
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(recorder, settingsRequest(http.MethodGet,
		"/api/control/devices/speaker/settings", ""))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.infoGets.Load() != 2 {
		t.Fatalf("live identity reads = %d, want bracketed snapshot", fixture.infoGets.Load())
	}
}

func TestGetSettingsRejectsEndpointIdentityMismatch(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.endpointDeviceID = "FFEEDDCCBBAA"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleGetDeviceSettings(recorder, settingsRequest(http.MethodGet,
		"/api/control/devices/speaker/settings", ""))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.infoGets.Load() != 1 {
		t.Fatalf("live identity reads = %d, want rejection at the mismatched endpoint", fixture.infoGets.Load())
	}
}

func TestSettingsMutationReportsPostWriteIdentityChangeAsUnverified(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.liveDeviceIDs = []string{
		"AABBCCDDEEFF",
		"AABBCCDDEEFF",
		"AABBCCDDEEFF",
		"FFEEDDCCBBAA",
	}
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", "{\"enabled\":false}"))

	response := decodeSettingsResponse(t, recorder)
	if recorder.Code != http.StatusAccepted || response.Success || response.Outcome != "unverified" {
		t.Fatalf("response = status %d %+v, want unverified", recorder.Code, response)
	}
	if fixture.timeoutPosts.Load() != 1 {
		t.Fatalf("automatic-standby mutations = %d, want exactly one", fixture.timeoutPosts.Load())
	}
}

func TestSettingsMutationReportsAmbiguousPOSTAsUnverified(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.timeoutMutationFail = true
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", "{\"enabled\":false}"))

	response := decodeSettingsResponse(t, recorder)
	if recorder.Code != http.StatusAccepted || response.Success || response.Outcome != "unverified" {
		t.Fatalf("response = status %d %+v, want unverified", recorder.Code, response)
	}
	if fixture.timeoutPosts.Load() != 1 || fixture.timeoutEnabled {
		t.Fatalf("ambiguous mutation = posts %d enabled %t, want one applied but unverified disable",
			fixture.timeoutPosts.Load(), fixture.timeoutEnabled)
	}
}

func TestSettingsMutationReportsSpeakerRejectionAsFailure(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.timeoutMutationReject = true
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", "{\"enabled\":false}"))

	response := decodeSettingsResponse(t, recorder)
	if recorder.Code != http.StatusBadGateway || response.Success || response.Outcome != "" {
		t.Fatalf("response = status %d %+v, want definitive failure", recorder.Code, response)
	}
	if !strings.Contains(response.Error, "UNKNOWN_ACTION_ERROR") {
		t.Fatalf("speaker rejection was not preserved: %+v", response)
	}
	if fixture.timeoutPosts.Load() != 1 {
		t.Fatalf("settings writes = %d, want one rejected request", fixture.timeoutPosts.Load())
	}
}

func TestSettingsMutationRequiresTargetIdentity(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	request := settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", `{"enabled":false}`)
	request.Header.Del(settingsTargetHeader)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, request)

	if recorder.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.timeoutPosts.Load() != 0 {
		t.Fatalf("unversioned target sent %d automatic-standby mutations", fixture.timeoutPosts.Load())
	}
}

func TestSettingsMutationsRequireCurrentReadbackBeforeWrite(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*settingsSpeakerFixture)
		invoke     func(*WebApp, http.ResponseWriter, *http.Request)
		method     string
		path       string
		body       string
		writeCount func(*settingsSpeakerFixture) int32
	}{
		{
			name: "automatic standby",
			configure: func(fixture *settingsSpeakerFixture) {
				fixture.timeoutReadFail = true
			},
			invoke: func(app *WebApp, w http.ResponseWriter, r *http.Request) {
				app.HandleSetSystemTimeout(w, r)
			},
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/system-timeout",
			body:   `{"enabled":false}`,
			writeCount: func(fixture *settingsSpeakerFixture) int32 {
				return fixture.timeoutPosts.Load()
			},
		},
		{
			name: "language",
			configure: func(fixture *settingsSpeakerFixture) {
				fixture.languageReadFail = true
			},
			invoke: func(app *WebApp, w http.ResponseWriter, r *http.Request) {
				app.HandleSetSystemLanguage(w, r)
			},
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/language",
			body:   `{"code":16}`,
			writeCount: func(fixture *settingsSpeakerFixture) int32 {
				return fixture.languagePosts.Load()
			},
		},
		{
			name: "sync priority",
			configure: func(fixture *settingsSpeakerFixture) {
				fixture.syncReadFail = true
			},
			invoke: func(app *WebApp, w http.ResponseWriter, r *http.Request) {
				app.HandleSetRebroadcastLatencyMode(w, r)
			},
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/sync",
			body:   `{"mode":"SYNC_TO_ROOM"}`,
			writeCount: func(fixture *settingsSpeakerFixture) int32 {
				return fixture.syncPosts.Load()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSettingsSpeakerFixture(t, true)
			test.configure(fixture)
			app := settingsTestApp(fixture)
			recorder := httptest.NewRecorder()

			test.invoke(app, recorder, settingsRequest(test.method, test.path, test.body))

			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if writes := test.writeCount(fixture); writes != 0 {
				t.Fatalf("writes = %d, want 0 after failed readback", writes)
			}
			if fixture.infoGets.Load() != 2 {
				t.Fatalf("live identity reads = %d, want one bracketed settings read", fixture.infoGets.Load())
			}
		})
	}
}

func TestHandleSetClockDisplayRequiresCapabilityAndReadback(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		fixture := newSettingsSpeakerFixture(t, true)
		app := settingsTestApp(fixture)
		recorder := httptest.NewRecorder()

		app.HandleSetClockDisplay(recorder, settingsRequest(http.MethodPatch,
			"/api/control/devices/speaker/settings/clock-display", `{"enabled":true,"format":"12"}`))

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		response := decodeSettingsResponse(t, recorder)
		if response.Data.ClockDisplay == nil || response.Data.ClockDisplay.Enabled == nil ||
			!*response.Data.ClockDisplay.Enabled || response.Data.ClockDisplay.Format != "12" {
			t.Fatalf("unexpected readback: %+v", response.Data.ClockDisplay)
		}
		if fixture.clockPosts.Load() != 1 {
			t.Fatalf("clock POST count = %d, want 1", fixture.clockPosts.Load())
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		fixture := newSettingsSpeakerFixture(t, false)
		app := settingsTestApp(fixture)
		recorder := httptest.NewRecorder()

		app.HandleSetClockDisplay(recorder, settingsRequest(http.MethodPatch,
			"/api/control/devices/speaker/settings/clock-display", `{"enabled":true}`))

		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if fixture.clockPosts.Load() != 0 {
			t.Fatalf("unsupported clock control sent %d POSTs", fixture.clockPosts.Load())
		}
	})

	t.Run("format omitted from readback", func(t *testing.T) {
		fixture := newSettingsSpeakerFixture(t, true)
		fixture.clockHasFormat = false
		app := settingsTestApp(fixture)
		recorder := httptest.NewRecorder()

		app.HandleSetClockDisplay(recorder, settingsRequest(http.MethodPatch,
			"/api/control/devices/speaker/settings/clock-display", `{"format":"12"}`))

		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if fixture.clockPosts.Load() != 0 {
			t.Fatalf("unsupported time format sent %d POSTs", fixture.clockPosts.Load())
		}
	})

	t.Run("timezone omitted from readback", func(t *testing.T) {
		fixture := newSettingsSpeakerFixture(t, true)
		fixture.clockHasZone = false
		app := settingsTestApp(fixture)
		recorder := httptest.NewRecorder()

		app.HandleSetClockDisplay(recorder, settingsRequest(http.MethodPatch,
			"/api/control/devices/speaker/settings/clock-display", `{"timeZone":"Europe/Berlin"}`))

		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		if fixture.clockPosts.Load() != 0 {
			t.Fatalf("unsupported timezone sent %d POSTs", fixture.clockPosts.Load())
		}
	})
}

func TestHandleSetClockDisplayRejectsInvalidFormat(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetClockDisplay(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/clock-display", "{\"format\":\"decimal\"}"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.clockPosts.Load() != 0 {
		t.Fatalf("invalid clock format sent %d POSTs", fixture.clockPosts.Load())
	}
	if fixture.infoGets.Load() != 0 {
		t.Fatalf("invalid clock format triggered %d network identity reads", fixture.infoGets.Load())
	}
}

func TestHandleSetClockTimeRequiresCurrentTimeReadback(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.clockHasTime = false
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetClockTime(recorder, settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/clock-time", ""))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.clockTimePosts.Load() != 0 {
		t.Fatalf("unsupported current-time control sent %d POSTs", fixture.clockTimePosts.Load())
	}
}

func TestHandleSetSystemTimeoutRejectsUnsupportedDeviceBeforeWrite(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, false)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", `{"enabled":false}`))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.timeoutPosts.Load() != 0 {
		t.Fatalf("unsupported automatic standby sent %d POSTs", fixture.timeoutPosts.Load())
	}
}

func TestHandleSetSystemTimeoutPreservesConfirmedOutcomeWhenRefreshFails(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.capabilityFailAfter = 2
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemTimeout(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/system-timeout", `{"enabled":false}`))

	response := decodeSettingsResponse(t, recorder)
	if recorder.Code != http.StatusOK || !response.Success || response.Outcome != "confirmed" {
		t.Fatalf("response = status %d %+v, want confirmed success", recorder.Code, response)
	}
	if response.Error != "" || !strings.Contains(response.Warning, "could not be refreshed") {
		t.Fatalf("confirmed refresh warning = %+v", response)
	}
	if fixture.timeoutPosts.Load() != 1 || fixture.timeoutEnabled {
		t.Fatalf("automatic standby mutation = posts %d enabled %t, want one confirmed disable",
			fixture.timeoutPosts.Load(), fixture.timeoutEnabled)
	}
}

func TestSettingsMutationsConfirmReadback(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		invoke func(*WebApp, http.ResponseWriter, *http.Request)
		verify func(*testing.T, *settingsSpeakerFixture)
	}{
		{
			name:   "clock time",
			method: http.MethodPost,
			path:   "/api/control/devices/speaker/settings/clock-time",
			invoke: (*WebApp).HandleSetClockTime,
		},
		{
			name:   "automatic standby",
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/system-timeout",
			body:   `{"enabled":false}`,
			invoke: (*WebApp).HandleSetSystemTimeout,
			verify: func(t *testing.T, fixture *settingsSpeakerFixture) {
				t.Helper()
				if fixture.timeoutEnabled {
					t.Fatal("automatic standby readback remained enabled")
				}
				if fixture.timeoutPosts.Load() != 1 {
					t.Fatalf("automatic standby POST count = %d, want 1", fixture.timeoutPosts.Load())
				}
			},
		},
		{
			name:   "language",
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/language",
			body:   `{"code":3}`,
			invoke: (*WebApp).HandleSetSystemLanguage,
			verify: func(t *testing.T, fixture *settingsSpeakerFixture) {
				t.Helper()
				if fixture.language != 3 {
					t.Fatalf("language readback = %d, want 3", fixture.language)
				}
			},
		},
		{
			name:   "sync priority",
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/sync",
			body:   `{"mode":"SYNC_TO_ROOM"}`,
			invoke: (*WebApp).HandleSetRebroadcastLatencyMode,
			verify: func(t *testing.T, fixture *settingsSpeakerFixture) {
				t.Helper()
				if fixture.syncMode != "SYNC_TO_ROOM" {
					t.Fatalf("sync readback = %q, want SYNC_TO_ROOM", fixture.syncMode)
				}
			},
		},
		{
			name:   "Bluetooth pairing",
			method: http.MethodPost,
			path:   "/api/control/devices/speaker/settings/bluetooth/pair",
			invoke: (*WebApp).HandleEnterBluetoothPairing,
			verify: func(t *testing.T, fixture *settingsSpeakerFixture) {
				t.Helper()
				if !fixture.discoverable {
					t.Fatal("Bluetooth pairing did not reach discoverable state")
				}
				if fixture.pairingGets.Load() != 1 {
					t.Fatalf("Bluetooth pairing GET count = %d, want 1", fixture.pairingGets.Load())
				}
			},
		},
		{
			name:   "source name",
			method: http.MethodPatch,
			path:   "/api/control/devices/speaker/settings/source-name",
			body:   `{"source":"AUX","sourceAccount":"AUX1","name":"  Turntable  "}`,
			invoke: (*WebApp).HandleSetSourceName,
			verify: func(t *testing.T, fixture *settingsSpeakerFixture) {
				t.Helper()
				if fixture.sourceName != "Turntable" {
					t.Fatalf("source-name readback = %q, want Turntable", fixture.sourceName)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSettingsSpeakerFixture(t, true)
			app := settingsTestApp(fixture)
			recorder := httptest.NewRecorder()

			test.invoke(app, recorder, settingsRequest(test.method, test.path, test.body))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			response := decodeSettingsResponse(t, recorder)
			if !response.Success {
				t.Fatalf("settings response failed: %s", response.Error)
			}
			if test.verify != nil {
				test.verify(t, fixture)
			}
		})
	}
}

func TestHandleEnterBluetoothPairingPollsWithoutMutationReplay(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.pairingConfirmAfter = 3
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 4)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.pairingGets.Load() != 1 {
		t.Fatalf("Bluetooth pairing GET count = %d, want 1", fixture.pairingGets.Load())
	}
	if fixture.pairingReads.Load() < 3 {
		t.Fatalf("post-mutation now-playing reads = %d, want at least 3", fixture.pairingReads.Load())
	}
}

func TestHandleEnterBluetoothPairingDoesNotWriteUnversionedSharedStatus(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	device, ok := app.GetDevice("speaker")
	if !ok {
		t.Fatal("settings test device is missing")
	}

	device.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.NowPlaying = &models.NowPlaying{Source: "SPOTIFY"}
	})
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 1)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	if got := device.Status().NowPlaying; got == nil || got.Source != "SPOTIFY" {
		t.Fatalf("shared now-playing cache = %+v, want existing revision-owned state", got)
	}
}

func TestHandleEnterBluetoothPairingConfirmsUnknownMutationOutcome(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.pairingMutation = "unknown-confirmed"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 3)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.pairingGets.Load() != 1 || fixture.pairingReads.Load() != 2 {
		t.Fatalf("pairing mutation/read counts = %d/%d, want 1/2 including settings refresh",
			fixture.pairingGets.Load(), fixture.pairingReads.Load())
	}
}

func TestHandleEnterBluetoothPairingReportsUnknownUnconfirmedOutcome(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.pairingMutation = "unknown-unconfirmed"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 3)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if response.Success || response.Outcome != "unverified" ||
		!strings.Contains(response.Error, "indeterminate") ||
		!strings.Contains(response.Error, "discoverability remains unconfirmed") {
		t.Fatalf("unknown pairing outcome = %+v, want explicit unverified", response)
	}
	if fixture.pairingGets.Load() != 1 || fixture.pairingReads.Load() != 3 {
		t.Fatalf("pairing mutation/read counts = %d/%d, want 1/3",
			fixture.pairingGets.Load(), fixture.pairingReads.Load())
	}
}

func TestHandleEnterBluetoothPairingFailsTypedMutationError(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.pairingMutation = "typed-error"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 3)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.pairingGets.Load() != 1 || fixture.pairingReads.Load() != 0 {
		t.Fatalf("pairing mutation/read counts = %d/%d, want 1/0",
			fixture.pairingGets.Load(), fixture.pairingReads.Load())
	}
}

func TestHandleEnterBluetoothPairingRejectsDiscoverableWrongSource(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.nowPlayingSource = "AUX"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 3)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if response.Success || response.Outcome != "unverified" || response.Error == "" {
		t.Fatalf("wrong-source response = %+v, want explicit unverified outcome", response)
	}
	if fixture.pairingGets.Load() != 1 {
		t.Fatalf("Bluetooth pairing GET count = %d, want 1", fixture.pairingGets.Load())
	}
	if fixture.pairingReads.Load() != 3 {
		t.Fatalf("post-mutation now-playing reads = %d, want 3", fixture.pairingReads.Load())
	}
}

func TestHandleEnterBluetoothPairingReportsAcceptedReadFailureAsUnverified(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.nowPlayingStatus = http.StatusServiceUnavailable
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 3)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if response.Success || response.Outcome != "unverified" {
		t.Fatalf("accepted pairing read failure = %+v, want unverified", response)
	}
	if !strings.Contains(response.Error, "state readback failed") {
		t.Fatalf("read failure diagnostic was not preserved: %+v", response)
	}
	if fixture.pairingGets.Load() != 1 {
		t.Fatalf("Bluetooth pairing GET count = %d, want 1", fixture.pairingGets.Load())
	}
}

func TestHandleEnterBluetoothPairingPreservesConfirmedOutcomeWhenRefreshFails(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.capabilityFailAfter = 2
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()
	request := withBluetoothPairingPoll(settingsRequest(http.MethodPost,
		"/api/control/devices/speaker/settings/bluetooth/pair", ""), 3)

	app.HandleEnterBluetoothPairing(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if !response.Success || response.Outcome != "confirmed" || response.Error != "" {
		t.Fatalf("confirmed pairing refresh failure = %+v, want confirmed success", response)
	}
	if !strings.Contains(response.Warning, "speaker confirmed the change") ||
		!strings.Contains(response.Warning, "could not be refreshed") {
		t.Fatalf("refresh warning was not preserved: %+v", response)
	}
	if fixture.pairingGets.Load() != 1 || fixture.pairingReads.Load() != 1 {
		t.Fatalf("pairing mutation/read counts = %d/%d, want 1/1",
			fixture.pairingGets.Load(), fixture.pairingReads.Load())
	}
	if fixture.capabilityGets.Load() != 2 {
		t.Fatalf("capability reads = %d, want preflight and failed refresh", fixture.capabilityGets.Load())
	}
}

func TestHandleSetSystemLanguageRejectsUnknownCodeBeforeWrite(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSystemLanguage(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/language", `{"code":14}`))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandleSetSourceNameRejectsWhitespaceBeforeWrite(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleSetSourceName(recorder, settingsRequest(http.MethodPatch,
		"/api/control/devices/speaker/settings/source-name",
		`{"source":"AUX","sourceAccount":"AUX1","name":"   "}`))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.sourceName != "Line in" {
		t.Fatalf("source name changed to %q", fixture.sourceName)
	}
}

func TestHandleClearBluetoothPairingsReportsUnverifiedReadback(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleClearBluetoothPairings(recorder, settingsRequest(http.MethodDelete,
		"/api/control/devices/speaker/settings/bluetooth/pairings?confirmed=true", ""))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if response.Success || response.Outcome != "unverified" || response.Error == "" {
		t.Fatalf("clear response claimed success: %+v", response)
	}
	if response.Data.Errors["bluetoothClear"] == "" {
		t.Fatalf("clear response claimed verified success: %+v", response.Data.Errors)
	}
	if fixture.clearGets.Load() != 1 {
		t.Fatalf("clear GET count = %d, want 1", fixture.clearGets.Load())
	}
}

func TestHandleClearBluetoothPairingsReportsAcceptedRefreshFailureAsUnverified(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.capabilityFailAfter = 2
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleClearBluetoothPairings(recorder, settingsRequest(http.MethodDelete,
		"/api/control/devices/speaker/settings/bluetooth/pairings?confirmed=true", ""))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if response.Success || response.Outcome != "unverified" {
		t.Fatalf("accepted clear refresh failure = %+v, want unverified", response)
	}
	if !strings.Contains(response.Error, "settings refresh failed") ||
		!strings.Contains(response.Error, "read device capabilities") {
		t.Fatalf("refresh failure diagnostic was not preserved: %+v", response)
	}
	if fixture.clearGets.Load() != 1 {
		t.Fatalf("clear GET count = %d, want 1", fixture.clearGets.Load())
	}
	if fixture.capabilityGets.Load() != 2 {
		t.Fatalf("capability reads = %d, want preflight and failed refresh", fixture.capabilityGets.Load())
	}
}

func TestHandleClearBluetoothPairingsRequiresConfirmation(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleClearBluetoothPairings(recorder, settingsRequest(http.MethodDelete,
		"/api/control/devices/speaker/settings/bluetooth/pairings", ""))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.clearGets.Load() != 0 {
		t.Fatalf("unconfirmed request sent %d clear mutations", fixture.clearGets.Load())
	}
}

func TestHandleClearBluetoothPairingsReportsUnknownOutcomeUnverified(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.clearMutation = "unknown"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleClearBluetoothPairings(recorder, settingsRequest(http.MethodDelete,
		"/api/control/devices/speaker/settings/bluetooth/pairings?confirmed=true", ""))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeSettingsResponse(t, recorder)
	if response.Success || response.Outcome != "unverified" ||
		!strings.Contains(response.Error, "outcome is unknown") {
		t.Fatalf("unknown clear outcome = %+v, want explicit unverified", response)
	}
	if fixture.clearGets.Load() != 1 {
		t.Fatalf("clear GET count = %d, want 1", fixture.clearGets.Load())
	}
}

func TestHandleClearBluetoothPairingsFailsTypedMutationError(t *testing.T) {
	fixture := newSettingsSpeakerFixture(t, true)
	fixture.clearMutation = "typed-error"
	app := settingsTestApp(fixture)
	recorder := httptest.NewRecorder()

	app.HandleClearBluetoothPairings(recorder, settingsRequest(http.MethodDelete,
		"/api/control/devices/speaker/settings/bluetooth/pairings?confirmed=true", ""))

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.clearGets.Load() != 1 {
		t.Fatalf("clear GET count = %d, want 1", fixture.clearGets.Load())
	}
}
