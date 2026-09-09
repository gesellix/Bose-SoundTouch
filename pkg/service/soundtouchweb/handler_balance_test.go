package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleBalanceControl_RequiresWebSocket pins the one structural
// difference between balance and every other audio control: the write goes
// over the WebSocket, because POST /balance hangs rather than refusing
// (GH-699). With no live socket there is no way to write at all, and the API
// must say so rather than fall back to HTTP.
func TestHandleBalanceControl_RequiresWebSocket(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest(http.MethodPost,
		"/api/control/devices/test-device/action/balance", strings.NewReader(`{"level": -3}`))
	req = withChiParams(req, map[string]string{"id": "test-device", "action": "balance"})
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	app.HandleAPIControl(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d when no WebSocket is connected", w.Code, http.StatusServiceUnavailable)
	}

	if body := w.Body.String(); !strings.Contains(body, "WebSocket") {
		t.Errorf("body = %q, want it to explain that a WebSocket is required", body)
	}
}

func TestHandleBalanceControl_RejectsNonPost(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest(http.MethodGet, "/api/control/devices/test-device/action/balance", nil)
	req = withChiParams(req, map[string]string{"id": "test-device", "action": "balance"})

	w := httptest.NewRecorder()
	app.HandleAPIControl(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleBalanceControl_RejectsInvalidBody(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest(http.MethodPost,
		"/api/control/devices/test-device/action/balance", strings.NewReader(`not json`))
	req = withChiParams(req, map[string]string{"id": "test-device", "action": "balance"})
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	app.HandleAPIControl(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestBalanceIsNotOnTheStatusPollPath is a guard, not a behaviour test.
//
// GET /balance BLOCKS instead of refusing while a speaker is in deep standby
// (measured at 12 s and counting), so folding it into the periodic status poll
// next to /volume and /bass would let one sleeping speaker stall every other
// field with it. The reading is refreshed from the WebSocket instead — on
// connect, on groupUpdated, and on balanceUpdated.
//
// If this test fails, someone added balance to updateDeviceStatus; move it
// back out rather than deleting the check.
func TestBalanceIsNotOnTheStatusPollPath(t *testing.T) {
	source, err := readSourceFile("websocket.go")
	if err != nil {
		t.Fatalf("read websocket.go: %v", err)
	}

	poll := sliceBetween(source, "func (app *WebApp) updateDeviceStatus(", "\n}\n")
	if poll == "" {
		t.Fatal("could not locate updateDeviceStatus; update this guard")
	}

	for _, forbidden := range []string{"GetBalance", "FieldBalance"} {
		if strings.Contains(poll, forbidden) {
			t.Errorf("updateDeviceStatus references %s; /balance must not be read on the "+
				"poll path, it blocks while the speaker is in deep standby", forbidden)
		}
	}
}

// TestBalanceIsRetriedAfterConnect guards the other half.
//
// A single read on connect is not enough, and both reasons were found on
// hardware: a pair that existed before startup is discovered by the /getGroup
// poll running concurrently (groupUpdated fires only when the pairing
// CHANGES), and a sleeping speaker answers balanceAvailable=false even when it
// is genuinely paired. Either way the symptom was the same — a paired speaker
// with no slider until something unrelated triggered a read.
func TestBalanceIsRetriedAfterConnect(t *testing.T) {
	source, err := readSourceFile("websocket.go")
	if err != nil {
		t.Fatalf("read websocket.go: %v", err)
	}

	watch := sliceBetween(source, "func (app *WebApp) watchBalance(", "\n}\n")
	if watch == "" {
		t.Fatal("watchBalance is gone; a single read misses a pair that already " +
			"existed at startup, discovered by the concurrent /getGroup poll")
	}

	if !strings.Contains(watch, "Status().Balance != nil") {
		t.Error("watchBalance no longer stops once a balance is known; it would " +
			"keep polling a speaker that has none")
	}

	if !strings.Contains(watch, "conn.Done()") {
		t.Error("watchBalance does not stop when the device goes away")
	}
}

// TestBalanceRequestJSONShape pins the body the player sends.
func TestBalanceRequestJSONShape(t *testing.T) {
	var decoded struct {
		Level int `json:"level"`
	}

	if err := json.Unmarshal([]byte(`{"level": -7}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Level != -7 {
		t.Errorf("Level = %d, want -7", decoded.Level)
	}
}

// TestRefreshBalanceDoesNotPrecheckGroup is the regression for a race found on
// hardware: refreshBalance used to skip the read when status.Group was nil.
//
// On a fresh connection that check runs alongside the first /getGroup poll, so
// the group is usually still nil — the reading was skipped exactly when it was
// first needed, and nothing retried it, because groupUpdated only fires when
// the pairing itself changes. A genuinely paired speaker showed no slider in
// the Player until an unrelated write happened to populate the field.
//
// balanceAvailable from the device is the authority; a local group snapshot is
// not. If this guard fails, someone reintroduced the pre-check.
func TestRefreshBalanceDoesNotPrecheckGroup(t *testing.T) {
	source, err := readSourceFile("websocket.go")
	if err != nil {
		t.Fatalf("read websocket.go: %v", err)
	}

	body := sliceBetween(source, "func (app *WebApp) refreshBalance(", "\n}\n")
	if body == "" {
		t.Fatal("could not locate refreshBalance; update this guard")
	}

	if strings.Contains(body, "Status().Group") {
		t.Error("refreshBalance pre-checks status.Group; at startup that races " +
			"the first /getGroup poll and silently skips the read")
	}

	// Reads must not depend on the WebSocket. It is created lazily, on the
	// first control request that needs one, so a paired speaker showed no
	// slider until something was pressed — the speaker was answering
	// balanceAvailable=true the whole time, and nobody was asking.
	if strings.Contains(body, "wsClient") || strings.Contains(body, "CurrentWebSocket") {
		t.Error("refreshBalance depends on the WebSocket; reads go over HTTP, " +
			"only writes need the socket")
	}
}

// TestBalanceWatchStartsWithTheDevice pins where the watch is anchored. Tying
// it to the WebSocket meant it never ran for a speaker nobody had interacted
// with, which is precisely the speaker whose slider was missing.
func TestBalanceWatchStartsWithTheDevice(t *testing.T) {
	source, err := readSourceFile("discovery.go")
	if err != nil {
		t.Fatalf("read discovery.go: %v", err)
	}

	if !strings.Contains(source, "go app.watchBalance(") {
		t.Error("device registration no longer starts the balance watch; a paired " +
			"speaker will show no slider until something else triggers a read")
	}
}

// TestBalanceWriteUsesCachedBounds pins that a write validates against the
// reading already on the status instead of fetching a fresh one.
//
// Reading per write made balance two round trips where volume and bass are
// one, and put the extra hit on /balance — the one endpoint that blocks rather
// than refusing when a speaker is asleep.
func TestBalanceWriteUsesCachedBounds(t *testing.T) {
	source, err := readSourceFile("handler.go")
	if err != nil {
		t.Fatalf("read handler.go: %v", err)
	}

	body := sliceBetween(source, "func (app *WebApp) handleBalanceControl(", "\n}\n")
	if body == "" {
		t.Fatal("could not locate handleBalanceControl; update this guard")
	}

	if !strings.Contains(body, "device.Status().Balance") {
		t.Error("handleBalanceControl no longer validates against the cached reading; " +
			"every write would fetch bounds it already has")
	}

	// The fallback read must stay: a first write can arrive before the watch
	// has produced a reading.
	if !strings.Contains(body, "wsClient.GetBalance(ctx)") {
		t.Error("the fallback read is gone; a write arriving before the first " +
			"successful read would have no bounds to validate against")
	}
}
