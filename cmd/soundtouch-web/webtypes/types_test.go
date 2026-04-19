// Package types contains tests for type definitions.
package webtypes

import (
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestAPIResponse(t *testing.T) {
	tests := []struct {
		name     string
		response APIResponse
		wantJSON string
	}{
		{
			name: "success response",
			response: APIResponse{
				Success: true,
				Data:    map[string]string{"message": "OK"},
			},
			wantJSON: `{"success":true,"data":{"message":"OK"}}`,
		},
		{
			name: "error response",
			response: APIResponse{
				Success: false,
				Error:   "Something went wrong",
			},
			wantJSON: `{"success":false,"error":"Something went wrong"}`,
		},
		{
			name: "success with nil data",
			response: APIResponse{
				Success: true,
			},
			wantJSON: `{"success":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the struct fields are correctly set
			if tt.response.Success != (tt.name == "success response" || tt.name == "success with nil data") {
				t.Errorf("Expected success to match test case")
			}
		})
	}
}

func TestVolumeRequest(t *testing.T) {
	tests := []struct {
		name  string
		req   VolumeRequest
		level int
	}{
		{"zero volume", VolumeRequest{Level: 0}, 0},
		{"mid volume", VolumeRequest{Level: 50}, 50},
		{"max volume", VolumeRequest{Level: 100}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Level != tt.level {
				t.Errorf("Expected level %d, got %d", tt.level, tt.req.Level)
			}
		})
	}
}

func TestBassRequest(t *testing.T) {
	tests := []struct {
		name  string
		req   BassRequest
		level int
	}{
		{"min bass", BassRequest{Level: -9}, -9},
		{"neutral bass", BassRequest{Level: 0}, 0},
		{"max bass", BassRequest{Level: 9}, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req.Level != tt.level {
				t.Errorf("Expected level %d, got %d", tt.level, tt.req.Level)
			}
		})
	}
}

func TestWebSocketMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      WebSocketMessage
		wantType string
	}{
		{
			name: "devices message",
			msg: WebSocketMessage{
				Type: "devices",
				Data: map[string]interface{}{"device1": "data"},
			},
			wantType: "devices",
		},
		{
			name: "status update message",
			msg: WebSocketMessage{
				Type:     "status_update",
				DeviceID: "device1",
				Data:     DeviceStatus{IsConnected: true},
			},
			wantType: "status_update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Type != tt.wantType {
				t.Errorf("Expected type %s, got %s", tt.wantType, tt.msg.Type)
			}
		})
	}
}

func TestDeviceConnection(t *testing.T) {
	deviceInfo := &models.DeviceInfo{
		Name: "Test Speaker",
		Type: "SoundTouch 30",
		NetworkInfo: []models.NetworkInfo{
			{MacAddress: "TEST123", IPAddress: "192.168.1.100"},
		},
	}

	nowPlaying := &models.NowPlaying{
		Track:      "Test Track",
		Artist:     "Test Artist",
		Album:      "Test Album",
		PlayStatus: models.PlayStatusPlaying,
		Source:     "SPOTIFY",
	}

	volume := &models.Volume{
		ActualVolume: 50,
		MuteEnabled:  false,
	}

	conn := &DeviceConnection{
		DeviceInfo: deviceInfo,
		LastSeen:   time.Now(),
		Status: DeviceStatus{
			NowPlaying:   nowPlaying,
			Volume:       volume,
			IsConnected:  true,
			LastActivity: time.Now(),
		},
	}

	t.Run("device connection fields", func(t *testing.T) {
		if conn.DeviceInfo.Name != "Test Speaker" {
			t.Errorf("Expected device name 'Test Speaker', got '%s'", conn.DeviceInfo.Name)
		}

		if conn.Status.NowPlaying.Track != "Test Track" {
			t.Errorf("Expected track 'Test Track', got '%s'", conn.Status.NowPlaying.Track)
		}

		if conn.Status.Volume.ActualVolume != 50 {
			t.Errorf("Expected volume 50, got %d", conn.Status.Volume.ActualVolume)
		}

		if !conn.Status.IsConnected {
			t.Error("Expected device to be connected")
		}
	})
}

func TestDeviceStatus(t *testing.T) {
	status := DeviceStatus{
		NowPlaying: &models.NowPlaying{
			Track:      "Test Track",
			PlayStatus: models.PlayStatusPlaying,
		},
		Volume: &models.Volume{
			ActualVolume: 75,
			MuteEnabled:  false,
		},
		Bass: &models.Bass{
			ActualBass: 3,
		},
		IsConnected:  true,
		LastActivity: time.Now(),
	}

	t.Run("device status fields", func(t *testing.T) {
		if status.NowPlaying == nil {
			t.Error("Expected now playing to be set")
		}

		if status.Volume == nil {
			t.Error("Expected volume to be set")
		}

		if status.Bass == nil {
			t.Error("Expected bass to be set")
		}

		if !status.IsConnected {
			t.Error("Expected device to be connected")
		}

		if status.LastActivity.IsZero() {
			t.Error("Expected last activity to be set")
		}
	})

	t.Run("nil fields", func(t *testing.T) {
		emptyStatus := DeviceStatus{}

		if emptyStatus.NowPlaying != nil {
			t.Error("Expected now playing to be nil")
		}

		if emptyStatus.Volume != nil {
			t.Error("Expected volume to be nil")
		}

		if emptyStatus.IsConnected {
			t.Error("Expected device to be disconnected by default")
		}
	})
}

// Benchmark tests
func BenchmarkAPIResponse(b *testing.B) {
	response := APIResponse{
		Success: true,
		Data:    map[string]string{"message": "OK"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = response.Success
		_ = response.Data
	}
}

func BenchmarkDeviceStatus(b *testing.B) {
	status := DeviceStatus{
		NowPlaying:   &models.NowPlaying{Track: "Test Track"},
		Volume:       &models.Volume{ActualVolume: 50},
		IsConnected:  true,
		LastActivity: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = status.IsConnected
		_ = status.NowPlaying.Track
		_ = status.Volume.ActualVolume
	}
}

func BenchmarkWebSocketMessage(b *testing.B) {
	msg := WebSocketMessage{
		Type:     "status_update",
		DeviceID: "device1",
		Data: DeviceStatus{
			IsConnected:  true,
			LastActivity: time.Now(),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Type
		_ = msg.DeviceID
		_ = msg.Data
	}
}
