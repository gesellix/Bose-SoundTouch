package soundtouchweb

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// speakerWebSocketLogger belongs to one device supervisor, including all its
// initial dial attempts. Transport callbacks and initial failures share the
// outage marker; client read/ping messages cannot establish transport state.
type speakerWebSocketLogger struct {
	mu        sync.Mutex
	deviceID  string
	conn      *webtypes.DeviceConnection
	outage    bool
	connected bool
}

func (l *speakerWebSocketLogger) Printf(format string, args ...interface{}) {
	// Match the client's routine formats exactly so new diagnostics stay
	// visible. Accepted transport callbacks report outages and recovery below.
	switch format {
	case "Connecting to %s", "Connected to %s", "Disconnected",
		"WebSocket read error: %v", "Failed to send ping: %v",
		"Reconnection attempt %d", "Reconnection attempt %d failed: %v",
		"Reconnected successfully", "Max reconnection attempts reached or reconnection disabled":
		return
	}

	log.Printf("[WebSocket] device %s: %s", sanitizeLog(l.deviceID), sanitizeLog(fmt.Sprintf(format, args...)))
}

func (l *speakerWebSocketLogger) initialFailure(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	select {
	case <-l.conn.Done():
		return
	default:
	}

	l.reportOutage(err.Error())
}

// transportState keeps acceptance and reporting ordered even if callbacks for
// different generations arrive concurrently. Shutdown is not an outage.
func (l *speakerWebSocketLogger) transportState(connected bool, generation uint64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	select {
	case <-l.conn.Done():
		return false
	default:
	}

	if !l.conn.ObserveEventStreamTransport(generation, connected, time.Now()) {
		return false
	}

	if !connected {
		l.reportOutage("transport disconnected")
		return true
	}

	if l.outage {
		log.Printf("WebSocket recovered for device %s", sanitizeLog(l.deviceID))
	} else if !l.connected {
		log.Printf("WebSocket connected for device %s", sanitizeLog(l.deviceID))
	}

	l.outage = false
	l.connected = true

	return true
}

// reportOutage is called with mu held.
func (l *speakerWebSocketLogger) reportOutage(reason string) {
	if !l.outage {
		log.Printf("WebSocket unavailable for device %s: %s", sanitizeLog(l.deviceID), sanitizeLog(reason))
		l.outage = true
	}
}
