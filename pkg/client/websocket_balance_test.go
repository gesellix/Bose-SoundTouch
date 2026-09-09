package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// balanceDocument is the response shape a paired SoundTouch 10 master returns,
// captured on hardware (FW 27.0.6).
func balanceDocument(target, actual int) string {
	return `<balance deviceID="DEVICEID01">` +
		`<balanceAvailable>true</balanceAvailable>` +
		`<balanceMin>-7</balanceMin><balanceMax>7</balanceMax><balanceDefault>0</balanceDefault>` +
		`<targetBalance>` + itoa(target) + `</targetBalance>` +
		`<actualBalance>` + itoa(actual) + `</actualBalance></balance>`
}

func itoa(v int) string {
	if v < 0 {
		return "-" + itoa(-v)
	}

	if v < 10 {
		return string(rune('0' + v))
	}

	return itoa(v/10) + itoa(v%10)
}

func TestWebSocketGetBalance(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{okResponse(route, id, balanceDocument(-3, -3))}
	}

	wsClient := f.connect(t)

	balance, err := wsClient.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}

	if !balance.Available {
		t.Error("Available = false, want true")
	}

	if balance.Target != -3 || balance.Min != -7 || balance.Max != 7 {
		t.Errorf("got target=%d range=%d..%d, want -3 and -7..7", balance.Target, balance.Min, balance.Max)
	}
}

// TestWebSocketSetBalanceSendsStockholmFrame pins the exact write the firmware
// accepts, as the app Bose ships on the speaker builds it. Confirmed on
// hardware at both range endpoints.
func TestWebSocketSetBalanceSendsStockholmFrame(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, frame string) []string {
		if strings.Contains(frame, `method="POST"`) {
			return []string{okResponse(route, id, balanceDocument(-3, -3))}
		}

		return []string{okResponse(route, id, balanceDocument(0, 0))}
	}

	wsClient := f.connect(t)

	updated, err := wsClient.SetBalance(context.Background(), -3)
	if err != nil {
		t.Fatalf("SetBalance: %v", err)
	}

	// The write echoes the whole document, so no read-back is needed — which
	// matters because HTTP GET /balance lags briefly behind a write.
	if updated.Target != -3 {
		t.Errorf("Target = %d, want -3 from the echoed response", updated.Target)
	}

	requests := f.recordedRequests()
	if len(requests) != 2 {
		t.Fatalf("want a read then a write, got %d requests", len(requests))
	}

	write := requests[1]
	for _, want := range []string{
		`url="balance"`,
		`method="POST"`,
		`<info mainNode="balanceSet" type="new"/>`,
		`<balance><targetBalance>-3</targetBalance></balance>`,
	} {
		if !strings.Contains(write, want) {
			t.Errorf("write frame missing %q in: %s", want, write)
		}
	}
}

// TestWebSocketSetBalanceValidatesAgainstDeviceRange pins that the bound check
// uses what the speaker reported, not a hardcoded constant. ±50 is the range
// the old implementation assumed, and the firmware does not agree.
func TestWebSocketSetBalanceValidatesAgainstDeviceRange(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{okResponse(route, id, balanceDocument(0, 0))}
	}

	wsClient := f.connect(t)

	_, err := wsClient.SetBalance(context.Background(), 50)
	if err == nil {
		t.Fatal("SetBalance accepted 50 against a device range of -7..7")
	}

	if !strings.Contains(err.Error(), "-7 to 7") {
		t.Errorf("err = %v, want it to quote the device-reported range", err)
	}

	// Only the read should have gone out; the write must not be attempted.
	if requests := f.recordedRequests(); len(requests) != 1 {
		t.Errorf("want only the read, got %d requests: %v", len(requests), requests)
	}
}

func TestWebSocketSetBalanceRejectsUnavailableSpeaker(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{okResponse(route, id,
			`<balance deviceID="DEVICEID01"><balanceAvailable>false</balanceAvailable>`+
				`<balanceMin>0</balanceMin><balanceMax>0</balanceMax>`+
				`<targetBalance>0</targetBalance><actualBalance>0</actualBalance></balance>`)}
	}

	wsClient := f.connect(t)

	_, err := wsClient.SetBalance(context.Background(), 3)
	if err == nil {
		t.Fatal("expected a write to an unpaired speaker to be refused")
	}

	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("err = %v, want it to explain that balance is unavailable here", err)
	}
}

// TestHTTPSetBalanceIsRefused pins that the HTTP write says why it cannot work
// rather than hanging, which is what the firmware does.
func TestHTTPSetBalanceIsRefused(t *testing.T) {
	c := NewClientFromHost("192.0.2.10")

	err := c.SetBalance(3)
	if err == nil {
		t.Fatal("POST /balance must be refused, not attempted")
	}

	if !strings.Contains(err.Error(), "WebSocketClient.SetBalance") {
		t.Errorf("err = %v, want it to point at the WebSocket write", err)
	}
}

// TestHTTPGetBalanceUsesItsOwnShortBudget pins the separate timeout. /balance
// BLOCKS rather than refusing while a speaker is in deep standby, so it must
// not inherit the client's general timeout, and must never be polled.
func TestHTTPGetBalanceUsesItsOwnShortBudget(t *testing.T) {
	blocked := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-blocked // imitate the deep-standby hang
	}))

	t.Cleanup(func() {
		close(blocked)
		server.Close()
	})

	host, port := splitHostPort(t, server.URL)
	c := NewClient(&Config{Host: host, Port: port, Timeout: time.Minute})

	start := time.Now()

	if _, err := c.GetBalance(); err == nil {
		t.Fatal("expected the hanging read to time out")
	}

	if elapsed := time.Since(start); elapsed > 3*BalanceReadTimeout {
		t.Errorf("GetBalance waited %s; it must use its own %s budget, not the client's",
			elapsed, BalanceReadTimeout)
	}
}

func TestHTTPGetBalanceParsesCapturedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(balanceDocument(-3, -3)))
	}))

	t.Cleanup(server.Close)

	host, port := splitHostPort(t, server.URL)
	c := NewClient(&Config{Host: host, Port: port})

	balance, err := c.GetBalance()
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}

	if balance.Target != -3 || balance.Max != 7 {
		t.Errorf("got %+v, want target -3 within -7..7", balance)
	}

	if got := balance.LevelName(balance.Target); got != "Left" {
		t.Errorf("LevelName(-3) = %q, want Left", got)
	}
}

// TestBalanceUpdatedReachesHandler pins that the payload-free notification is
// dispatched. It signals "re-read", so a subscriber only needs to know it fired.
func TestBalanceUpdatedReachesHandler(t *testing.T) {
	c := NewClientFromHost("192.0.2.10")
	wsClient := c.NewWebSocketClient(&WebSocketConfig{Logger: &mockLogger{}})

	var got *models.BalanceUpdatedEvent

	wsClient.OnBalanceUpdated(func(event *models.BalanceUpdatedEvent) { got = event })

	wsClient.handleMessage([]byte(`<updates deviceID="DEVICEID01"><balanceUpdated></balanceUpdated></updates>`))

	if got == nil {
		t.Fatal("OnBalanceUpdated was not called")
	}
}

// TestBalanceUpdatedCarriesNoPayload documents why the event type has no
// fields: everything a caller could want is on the parent <updates>, and the
// element itself is empty on the wire.
func TestBalanceUpdatedCarriesNoPayload(t *testing.T) {
	event, err := models.ParseWebSocketEvent(
		[]byte(`<updates deviceID="DEVICEID01"><balanceUpdated></balanceUpdated></updates>`))
	if err != nil {
		t.Fatalf("ParseWebSocketEvent: %v", err)
	}

	if event.BalanceUpdated == nil {
		t.Fatal("BalanceUpdated is nil")
	}

	if event.DeviceID != "DEVICEID01" {
		t.Errorf("the device ID belongs to <updates>; got %q", event.DeviceID)
	}

	if !event.HasEventType(models.EventTypeBalanceUpdated) {
		t.Error("HasEventType(EventTypeBalanceUpdated) = false")
	}
}

// TestBalanceUpdatedIsNotAnUnknownEvent pins the captured frame verbatim,
// single quotes and all: before balanceUpdated was modelled it reached the CLI
// as "Unknown Event / Unmodelled element: balanceUpdated".
func TestBalanceUpdatedIsNotAnUnknownEvent(t *testing.T) {
	c := NewClientFromHost("192.0.2.10")
	wsClient := c.NewWebSocketClient(&WebSocketConfig{Logger: &mockLogger{}})

	var (
		typed   bool
		unknown bool
	)

	wsClient.OnBalanceUpdated(func(*models.BalanceUpdatedEvent) { typed = true })
	wsClient.OnUnknownEvent(func(*models.WebSocketEvent) { unknown = true })

	// Exactly as the speaker sent it, including the single-quoted attribute.
	wsClient.handleMessage([]byte(`<updates deviceID='DEVICEID01'><balanceUpdated></balanceUpdated></updates>`))

	if !typed {
		t.Error("OnBalanceUpdated did not fire")
	}

	if unknown {
		t.Error("balanceUpdated is still reported as an unknown event")
	}
}
