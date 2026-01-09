package models

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestWebSocketEventType_String(t *testing.T) {
	tests := []struct {
		name     string
		event    WebSocketEventType
		expected string
	}{
		{"NowPlaying", EventTypeNowPlaying, "Now Playing Updated"},
		{"VolumeUpdated", EventTypeVolumeUpdated, "Volume Updated"},
		{"ConnectionState", EventTypeConnectionState, "Connection State Updated"},
		{"PresetUpdated", EventTypePresetUpdated, "Preset Updated"},
		{"ZoneUpdated", EventTypeZoneUpdated, "Zone Updated"},
		{"BassUpdated", EventTypeBassUpdated, "Bass Updated"},
		{"ClockTimeUpdated", EventTypeClockTimeUpdated, "Clock Time Updated"},
		{"ClockDisplayUpdated", EventTypeClockDisplayUpdated, "Clock Display Updated"},
		{"NameUpdated", EventTypeNameUpdated, "Name Updated"},
		{"ErrorUpdated", EventTypeErrorUpdated, "Error Updated"},
		{"RecentsUpdated", EventTypeRecentsUpdated, "Recents Updated"},
		{"LanguageUpdated", EventTypeLanguageUpdated, "Language Updated"},
		{"Unknown", EventTypeUnknown, "Unknown Event"},
		{"Invalid", WebSocketEventType("invalid"), "Unknown Event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.String()
			if result != tt.expected {
				t.Errorf("WebSocketEventType.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWebSocketMessage_GetEventType(t *testing.T) {
	tests := []struct {
		name     string
		xmlName  xml.Name
		expected WebSocketEventType
	}{
		{"NowPlaying", xml.Name{Local: "nowPlayingUpdated"}, EventTypeNowPlaying},
		{"VolumeUpdated", xml.Name{Local: "volumeUpdated"}, EventTypeVolumeUpdated},
		{"ConnectionState", xml.Name{Local: "connectionStateUpdated"}, EventTypeConnectionState},
		{"PresetUpdated", xml.Name{Local: "presetUpdated"}, EventTypePresetUpdated},
		{"ZoneUpdated", xml.Name{Local: "zoneUpdated"}, EventTypeZoneUpdated},
		{"BassUpdated", xml.Name{Local: "bassUpdated"}, EventTypeBassUpdated},
		{"ClockTimeUpdated", xml.Name{Local: "clockTimeUpdated"}, EventTypeClockTimeUpdated},
		{"ClockDisplayUpdated", xml.Name{Local: "clockDisplayUpdated"}, EventTypeClockDisplayUpdated},
		{"NameUpdated", xml.Name{Local: "nameUpdated"}, EventTypeNameUpdated},
		{"ErrorUpdated", xml.Name{Local: "errorUpdated"}, EventTypeErrorUpdated},
		{"RecentsUpdated", xml.Name{Local: "recentsUpdated"}, EventTypeRecentsUpdated},
		{"LanguageUpdated", xml.Name{Local: "languageUpdated"}, EventTypeLanguageUpdated},
		{"Unknown", xml.Name{Local: "unknownEvent"}, EventTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := WebSocketMessage{XMLName: tt.xmlName}
			result := msg.GetEventType()
			if result != tt.expected {
				t.Errorf("WebSocketMessage.GetEventType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConnectionState_IsConnected(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		expected bool
	}{
		{"Connected", "CONNECTED", true},
		{"Disconnected", "DISCONNECTED", false},
		{"Connecting", "CONNECTING", false},
		{"Unknown", "UNKNOWN", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &ConnectionState{State: tt.state}
			result := cs.IsConnected()
			if result != tt.expected {
				t.Errorf("ConnectionState.IsConnected() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConnectionState_GetSignalStrength(t *testing.T) {
	cs := &ConnectionState{Signal: "EXCELLENT"}
	result := cs.GetSignalStrength()
	expected := "EXCELLENT"
	if result != expected {
		t.Errorf("ConnectionState.GetSignalStrength() = %v, want %v", result, expected)
	}
}

func TestParseWebSocketEvent(t *testing.T) {
	t.Run("ValidNowPlayingEvent", func(t *testing.T) {
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<nowPlayingUpdated deviceID="689E19B8BB8A">
		<nowPlaying deviceID="689E19B8BB8A" source="SPOTIFY">
			<track>Test Track</track>
			<artist>Test Artist</artist>
			<album>Test Album</album>
			<playStatus>PLAY_STATE</playStatus>
		</nowPlaying>
	</nowPlayingUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent() failed: %v", err)
		}

		if event == nil {
			t.Fatal("ParseWebSocketEvent() returned nil event")
		}

		if event.DeviceID != "689E19B8BB8A" {
			t.Errorf("Expected DeviceID '689E19B8BB8A', got '%s'", event.DeviceID)
		}

		if !event.HasEventType(EventTypeNowPlaying) {
			t.Error("Expected event to have EventTypeNowPlaying")
		}

		if event.NowPlayingUpdated == nil {
			t.Error("Expected NowPlayingUpdated to be populated")
		}

		// Check timestamp was added
		if event.Timestamp.IsZero() {
			t.Error("Expected timestamp to be set")
		}
	})

	t.Run("ValidVolumeEvent", func(t *testing.T) {
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<volumeUpdated deviceID="689E19B8BB8A">
		<volume deviceID="689E19B8BB8A">
			<targetvolume>25</targetvolume>
			<actualvolume>25</actualvolume>
			<muteenabled>false</muteenabled>
		</volume>
	</volumeUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent() failed: %v", err)
		}

		if event == nil {
			t.Fatal("ParseWebSocketEvent() returned nil event")
		}

		if !event.HasEventType(EventTypeVolumeUpdated) {
			t.Error("Expected event to have EventTypeVolumeUpdated")
		}

		if event.VolumeUpdated == nil {
			t.Error("Expected VolumeUpdated to be populated")
		}
	})

	t.Run("MultipleEvents", func(t *testing.T) {
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<volumeUpdated deviceID="689E19B8BB8A">
		<volume deviceID="689E19B8BB8A">
			<targetvolume>30</targetvolume>
			<actualvolume>30</actualvolume>
			<muteenabled>false</muteenabled>
		</volume>
	</volumeUpdated>
	<bassUpdated deviceID="689E19B8BB8A">
		<bass deviceID="689E19B8BB8A">
			<targetbass>2</targetbass>
			<actualbass>2</actualbass>
		</bass>
	</bassUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent() failed: %v", err)
		}

		if !event.HasEventType(EventTypeVolumeUpdated) {
			t.Error("Expected event to have EventTypeVolumeUpdated")
		}

		if !event.HasEventType(EventTypeBassUpdated) {
			t.Error("Expected event to have EventTypeBassUpdated")
		}

		eventTypes := event.GetEventTypes()
		if len(eventTypes) != 2 {
			t.Errorf("Expected 2 event types, got %d", len(eventTypes))
		}
	})

	t.Run("InvalidXML", func(t *testing.T) {
		xmlData := `<invalid xml>`

		_, err := ParseWebSocketEvent([]byte(xmlData))
		if err == nil {
			t.Error("Expected error for invalid XML, got nil")
		}
	})
}

func TestWebSocketEvent_HasEventType(t *testing.T) {
	event := &WebSocketEvent{
		NowPlayingUpdated: &NowPlayingUpdatedEvent{},
		VolumeUpdated:     &VolumeUpdatedEvent{},
	}

	t.Run("HasNowPlaying", func(t *testing.T) {
		if !event.HasEventType(EventTypeNowPlaying) {
			t.Error("Expected event to have nowPlayingUpdated type")
		}
	})

	t.Run("HasVolumeUpdated", func(t *testing.T) {
		if !event.HasEventType(EventTypeVolumeUpdated) {
			t.Error("Expected event to have volumeUpdated type")
		}
	})

	t.Run("DoesNotHaveBass", func(t *testing.T) {
		if event.HasEventType(EventTypeBassUpdated) {
			t.Error("Expected event to not have bassUpdated type")
		}
	})
}

func TestWebSocketEvent_GetEventTypes(t *testing.T) {
	event := &WebSocketEvent{
		NowPlayingUpdated: &NowPlayingUpdatedEvent{},
		VolumeUpdated:     &VolumeUpdatedEvent{},
		// No unknown events in the new structure
	}

	types := event.GetEventTypes()
	expected := []WebSocketEventType{
		EventTypeNowPlaying,
		EventTypeVolumeUpdated,
	}

	if len(types) != len(expected) {
		t.Errorf("Expected %d event types, got %d", len(expected), len(types))
		return
	}

	for i, expectedType := range expected {
		if types[i] != expectedType {
			t.Errorf("Expected event type %v at index %d, got %v", expectedType, i, types[i])
		}
	}
}

func TestWebSocketEvent_String(t *testing.T) {
	t.Run("NoEvents", func(t *testing.T) {
		event := &WebSocketEvent{
			DeviceID: "TEST123",
		}

		result := event.String()
		expected := "WebSocket Event [Device: TEST123] - No events"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("SingleEvent", func(t *testing.T) {
		event := &WebSocketEvent{
			DeviceID:          "TEST123",
			NowPlayingUpdated: &NowPlayingUpdatedEvent{},
		}

		result := event.String()
		expected := "WebSocket Event [Device: TEST123] - Now Playing Updated"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})

	t.Run("MultipleEvents", func(t *testing.T) {
		event := &WebSocketEvent{
			DeviceID:          "TEST123",
			NowPlayingUpdated: &NowPlayingUpdatedEvent{},
			VolumeUpdated:     &VolumeUpdatedEvent{},
		}

		result := event.String()
		expected := "WebSocket Event [Device: TEST123] - 2 events"
		if result != expected {
			t.Errorf("Expected '%s', got '%s'", expected, result)
		}
	})
}

func TestParseTypedEvent(t *testing.T) {
	t.Run("ParseNowPlayingEvent", func(t *testing.T) {
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<nowPlayingUpdated deviceID="689E19B8BB8A">
		<nowPlaying deviceID="689E19B8BB8A" source="SPOTIFY">
			<track>Test Track</track>
			<artist>Test Artist</artist>
			<album>Test Album</album>
			<playStatus>PLAY_STATE</playStatus>
		</nowPlaying>
	</nowPlayingUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent() failed: %v", err)
		}

		typedEvent, err := ParseTypedEvent[*NowPlayingUpdatedEvent](event, EventTypeNowPlaying)
		if err != nil {
			t.Fatalf("ParseTypedEvent() failed: %v", err)
		}

		if typedEvent.DeviceID != "689E19B8BB8A" {
			t.Errorf("Expected DeviceID '689E19B8BB8A', got '%s'", typedEvent.DeviceID)
		}

		if typedEvent.NowPlaying.Track != "Test Track" {
			t.Errorf("Expected Track 'Test Track', got '%s'", typedEvent.NowPlaying.Track)
		}
	})

	t.Run("ParseVolumeEvent", func(t *testing.T) {
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<volumeUpdated deviceID="689E19B8BB8A">
		<volume deviceID="689E19B8BB8A">
			<targetvolume>25</targetvolume>
			<actualvolume>25</actualvolume>
			<muteenabled>false</muteenabled>
		</volume>
	</volumeUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent() failed: %v", err)
		}

		typedEvent, err := ParseTypedEvent[*VolumeUpdatedEvent](event, EventTypeVolumeUpdated)
		if err != nil {
			t.Fatalf("ParseTypedEvent() failed: %v", err)
		}

		if typedEvent.DeviceID != "689E19B8BB8A" {
			t.Errorf("Expected DeviceID '689E19B8BB8A', got '%s'", typedEvent.DeviceID)
		}

		if typedEvent.Volume.TargetVolume != 25 {
			t.Errorf("Expected TargetVolume 25, got %d", typedEvent.Volume.TargetVolume)
		}

		if typedEvent.Volume.ActualVolume != 25 {
			t.Errorf("Expected ActualVolume 25, got %d", typedEvent.Volume.ActualVolume)
		}
	})

	t.Run("EventTypeNotFound", func(t *testing.T) {
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<volumeUpdated deviceID="689E19B8BB8A">
		<volume deviceID="689E19B8BB8A">
			<targetvolume>25</targetvolume>
			<actualvolume>25</actualvolume>
			<muteenabled>false</muteenabled>
		</volume>
	</volumeUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent() failed: %v", err)
		}

		_, err = ParseTypedEvent[*NowPlayingUpdatedEvent](event, EventTypeNowPlaying)
		if err == nil {
			t.Error("Expected error when parsing non-existent event type, got nil")
		}
	})
}

// Benchmark tests for performance
func BenchmarkParseWebSocketEvent(b *testing.B) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
	<nowPlayingUpdated deviceID="689E19B8BB8A">
		<nowPlaying deviceID="689E19B8BB8A" source="SPOTIFY">
			<track>Test Track</track>
			<artist>Test Artist</artist>
			<album>Test Album</album>
			<playStatus>PLAY_STATE</playStatus>
		</nowPlaying>
	</nowPlayingUpdated>
</updates>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWebSocketEventGetEventTypes(b *testing.B) {
	event := &WebSocketEvent{
		NowPlayingUpdated: &NowPlayingUpdatedEvent{},
		VolumeUpdated:     &VolumeUpdatedEvent{},
		BassUpdated:       &BassUpdatedEvent{},
		PresetUpdated:     &PresetUpdatedEvent{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = event.GetEventTypes()
	}
}

// Test helper for creating mock events
func createMockWebSocketEvent(deviceID string, eventTypes ...WebSocketEventType) *WebSocketEvent {
	event := &WebSocketEvent{
		DeviceID:  deviceID,
		Timestamp: time.Now(),
	}

	for _, eventType := range eventTypes {
		switch eventType {
		case EventTypeNowPlaying:
			event.NowPlayingUpdated = &NowPlayingUpdatedEvent{}
		case EventTypeVolumeUpdated:
			event.VolumeUpdated = &VolumeUpdatedEvent{}
		case EventTypeBassUpdated:
			event.BassUpdated = &BassUpdatedEvent{}
		}
	}

	return event
}

func TestCreateMockWebSocketEvent(t *testing.T) {
	event := createMockWebSocketEvent("TEST123", EventTypeNowPlaying, EventTypeVolumeUpdated)

	if event.DeviceID != "TEST123" {
		t.Errorf("Expected DeviceID 'TEST123', got '%s'", event.DeviceID)
	}

	events := event.GetEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	types := event.GetEventTypes()
	if len(types) != 2 {
		t.Errorf("Expected 2 event types, got %d", len(types))
	}

	if types[0] != EventTypeNowPlaying || types[1] != EventTypeVolumeUpdated {
		t.Errorf("Event types don't match expected values")
	}
}
