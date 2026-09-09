package webtypes

import (
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestApplySpeakerConnectionEvent classifies the frames a real speaker sends.
//
// Before GH-701 the switch compared State against a bare "CONNECTED", which no
// captured frame carries — so every real event fell through to "unknown" and
// the speaker-reported state was never usable. Up is now the only positive
// evidence, which also keeps this in step with
// models.ConnectionStateUpdatedEvent.IsConnected: the same event feeds both.
func TestApplySpeakerConnectionEvent(t *testing.T) {
	tests := []struct {
		name          string
		state         SpeakerConnectionState
		wantKnown     bool
		wantConnected bool
	}{
		{
			name:          "captured wifi-connected frame",
			state:         SpeakerConnectionState{State: "NETWORK_WIFI_CONNECTED", Up: true, Signal: "EXCELLENT_SIGNAL"},
			wantKnown:     true,
			wantConnected: true,
		},
		{
			name:          "wifi disconnected",
			state:         SpeakerConnectionState{State: "NETWORK_WIFI_DISCONNECTED", Up: false},
			wantKnown:     true,
			wantConnected: false,
		},
		{
			// up wins over a state string that claims otherwise. Accepting
			// the state here would contradict IsConnected() on the very same
			// event and hold the device out of the offline path.
			name:          "up=false outranks a CONNECTED state name",
			state:         SpeakerConnectionState{State: "NETWORK_WIFI_CONNECTED", Up: false},
			wantKnown:     false,
			wantConnected: false,
		},
		{
			name:          "no usable evidence",
			state:         SpeakerConnectionState{State: "", Up: false},
			wantKnown:     false,
			wantConnected: false,
		},
		{
			name:          "bare CONNECTED with up set",
			state:         SpeakerConnectionState{State: string(models.ConnectionStateConnected), Up: true},
			wantKnown:     true,
			wantConnected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "DEVICEID01"})

			conn.ApplySpeakerConnectionEvent(tt.state, time.Now())

			conn.healthMu.Lock()
			gotKnown := conn.speakerConnectionKnown
			gotConnected := conn.speakerConnectionConnected
			conn.healthMu.Unlock()

			if gotKnown != tt.wantKnown {
				t.Errorf("speakerConnectionKnown = %v, want %v", gotKnown, tt.wantKnown)
			}

			if gotConnected != tt.wantConnected {
				t.Errorf("speakerConnectionConnected = %v, want %v", gotConnected, tt.wantConnected)
			}

			reported := conn.Status().SpeakerConnectionState
			if reported == nil {
				t.Fatal("SpeakerConnectionState not surfaced on the status")
			}

			if reported.Up != tt.state.Up || reported.State != tt.state.State {
				t.Errorf("reported = %+v, want %+v", reported, tt.state)
			}
		})
	}
}
