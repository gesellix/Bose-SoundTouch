package stockholm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- resolveClientID ----

func TestResolveClientID_Header(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Stockholm-Client-Id", "tab-123")

	if got := resolveClientID(r); got != "tab-123" {
		t.Errorf("expected tab-123, got %q", got)
	}
}

func TestResolveClientID_QueryParam(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?clientId=browser-abc", nil)

	if got := resolveClientID(r); got != "browser-abc" {
		t.Errorf("expected browser-abc, got %q", got)
	}
}

func TestResolveClientID_Default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if got := resolveClientID(r); got != "default" {
		t.Errorf("expected default, got %q", got)
	}
}

func TestResolveClientID_HeaderTakesPrecedence(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?clientId=from-query", nil)
	r.Header.Set("X-Stockholm-Client-Id", "from-header")

	if got := resolveClientID(r); got != "from-header" {
		t.Errorf("expected from-header, got %q", got)
	}
}

// ---- legalDocPath ----

func TestLegalDocPath(t *testing.T) {
	cases := []struct {
		params map[string]interface{}
		want   string
	}{
		{map[string]interface{}{"type": "lcns"}, "legal/gui_licenses_en.txt"},
		{map[string]interface{}{}, "legal/eula_en.txt"},
		{map[string]interface{}{"type": "eula", "lang": "de"}, "legal/eula_de.txt"},
		{map[string]interface{}{"type": "privacy"}, "legal/eula_en.txt"},
	}

	for _, tc := range cases {
		if got := legalDocPath(tc.params); got != tc.want {
			t.Errorf("legalDocPath(%v) = %q, want %q", tc.params, got, tc.want)
		}
	}
}

// ---- Bridge dispatch via HTTP handlers ----

func newTestBridge(t *testing.T) *Bridge {
	t.Helper()
	state := NewNativeState(t.TempDir())
	return newBridge(&Config{}, state)
}

func appSend(t *testing.T, b *Bridge, method string, params map[string]interface{}, id interface{}) {
	t.Helper()
	body := map[string]interface{}{"method": method, "params": params, "id": id}

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal appSend body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/native/appSend", bytes.NewReader(data))
	req.Header.Set("X-Stockholm-Client-Id", "test")
	rec := httptest.NewRecorder()

	b.HandleAppSend(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("HandleAppSend returned %d, want 204", rec.Code)
	}
}

func drainQueue(t *testing.T, b *Bridge) []bridgeMessage {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/native/runQueue", nil)
	req.Header.Set("X-Stockholm-Client-Id", "test")
	rec := httptest.NewRecorder()

	b.HandleRunQueue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HandleRunQueue returned %d, want 200", rec.Code)
	}

	var resp struct {
		Messages []bridgeMessage `json:"messages"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode runQueue response: %v", err)
	}

	return resp.Messages
}

// TestBridge_DeviceDiscovery_CumulativeList verifies that each "devices" message
// sent during incremental discovery includes all previously found devices.
// The JS "devices" handler reconciles the full list and would drop earlier
// devices if only the latest one were included.
func TestBridge_DeviceDiscovery_CumulativeList(t *testing.T) {
	b := newTestBridge(t)

	// Simulate what runDeviceDiscovery now does: build a cumulative slice and
	// enqueue it with every new device.
	d1 := RendererDevice{UID: "AABBCC112233", IP: "192.0.2.10"}
	d2 := RendererDevice{UID: "DDEEFF445566", IP: "192.0.2.11"}

	var seen []RendererDevice
	for _, d := range []RendererDevice{d1, d2} {
		seen = append(seen, d)
		b.enqueueMethod("test", "devices", seen)
	}

	msgs := drainQueue(t, b)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 queued messages, got %d", len(msgs))
	}

	// First message: only d1
	firstParams, ok := msgs[0].Params.([]interface{})
	if !ok || len(firstParams) != 1 {
		t.Errorf("first message: expected 1-element params, got %v", msgs[0].Params)
	}

	// Second message: d1 AND d2
	secondParams, ok := msgs[1].Params.([]interface{})
	if !ok || len(secondParams) != 2 {
		t.Errorf("second message: expected 2-element params, got %v", msgs[1].Params)
	}
}

func TestBridge_Locale_IsNoOp(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "locale", nil, 1)

	msgs := drainQueue(t, b)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for locale, got %d", len(msgs))
	}
}

func TestBridge_GetLanStatus_ReturnsTrue(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "getLanStatus", nil, 42)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Result != true {
		t.Errorf("expected result=true, got %v", msgs[0].Result)
	}
}

func TestBridge_SetData_Get_RoundTrip(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "setData", map[string]interface{}{"name": "myKey", "value": "hello"}, nil)
	appSend(t, b, "getData", map[string]interface{}{"name": "myKey"}, 7)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (for getData), got %d", len(msgs))
	}

	if msgs[0].Result != "hello" {
		t.Errorf("expected result=hello, got %v", msgs[0].Result)
	}
}

func TestBridge_GetConstant_Kilo_Default(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "getConstant", map[string]interface{}{"name": "kilo"}, 1)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Result != "a7928d7b43dcd49f0af31e5aeed26458" {
		t.Errorf("unexpected kilo value: %v", msgs[0].Result)
	}
}

func TestBridge_GetTimeZone_ContainsTimezoneInfo(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "getTimeZone", nil, 2)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	result, ok := msgs[0].Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", msgs[0].Result)
	}

	if _, hasKey := result["timezoneInfo"]; !hasKey {
		t.Error("expected timezoneInfo key in getTimeZone result")
	}
}

func TestBridge_CanPerformAutoAPSetup_ReturnsFalse(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "canPerformAutoAPSetup", nil, 3)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	result, ok := msgs[0].Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", msgs[0].Result)
	}

	if result["permission"] != false {
		t.Errorf("expected permission=false, got %v", result["permission"])
	}
}

func TestBridge_UnsupportedMethod_ReturnsError(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "downloadNewGui", nil, 99)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Error == nil {
		t.Error("expected error for unsupported method")
	}
}

func TestBridge_Log_IsNoOp(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "log", map[string]interface{}{"msg": "hello from js"}, nil)

	msgs := drainQueue(t, b)
	if len(msgs) != 0 {
		t.Errorf("expected no queued messages for log, got %d", len(msgs))
	}
}

func TestBridge_GetLegalDocPath(t *testing.T) {
	b := newTestBridge(t)
	appSend(t, b, "getLegalDocPath", map[string]interface{}{"type": "lcns"}, 5)

	msgs := drainQueue(t, b)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	if !strings.HasSuffix(msgs[0].Result.(string), "gui_licenses_en.txt") {
		t.Errorf("unexpected legal doc path: %v", msgs[0].Result)
	}
}

func TestBridge_RunQueue_EmptyWhenNoPendingMessages(t *testing.T) {
	b := newTestBridge(t)
	msgs := drainQueue(t, b)

	if len(msgs) != 0 {
		t.Errorf("expected empty queue, got %d messages", len(msgs))
	}
}

func TestBridge_AppSend_WrongMethod_Returns405(t *testing.T) {
	b := newTestBridge(t)
	req := httptest.NewRequest(http.MethodGet, "/api/native/appSend", nil)
	rec := httptest.NewRecorder()

	b.HandleAppSend(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestBridge_RunQueue_WrongMethod_Returns405(t *testing.T) {
	b := newTestBridge(t)
	req := httptest.NewRequest(http.MethodPost, "/api/native/runQueue", nil)
	rec := httptest.NewRecorder()

	b.HandleRunQueue(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
