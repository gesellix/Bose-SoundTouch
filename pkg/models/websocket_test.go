package models

import (
	"strings"
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
		{"GroupUpdated", EventTypeGroupUpdated, "Stereo Pair Updated"},
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

// TestConnectionStateUpdatedEvent_Parse pins the shape captured from a
// SoundTouch 10 (variant=rhino, moduleType=sm2, FW 27.0.6): state, up and
// signal are attributes on <connectionStateUpdated> itself. The type used to
// model a nested <connectionState> child that never appears, which left State
// and Signal permanently empty (GH-701).
func TestConnectionStateUpdatedEvent_Parse(t *testing.T) {
	tests := []struct {
		name       string
		xmlData    string
		wantState  string
		wantUp     bool
		wantSignal string
	}{
		{
			name: "captured wifi-connected frame",
			xmlData: `<updates deviceID="DEVICEID01">` +
				`<connectionStateUpdated state="NETWORK_WIFI_CONNECTED" up="true" signal="EXCELLENT_SIGNAL" />` +
				`</updates>`,
			wantState:  "NETWORK_WIFI_CONNECTED",
			wantUp:     true,
			wantSignal: "EXCELLENT_SIGNAL",
		},
		{
			name: "link down",
			xmlData: `<updates deviceID="DEVICEID01">` +
				`<connectionStateUpdated state="NETWORK_WIFI_DISCONNECTED" up="false" signal="NO_SIGNAL" />` +
				`</updates>`,
			wantState:  "NETWORK_WIFI_DISCONNECTED",
			wantUp:     false,
			wantSignal: "NO_SIGNAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseWebSocketEvent([]byte(tt.xmlData))
			if err != nil {
				t.Fatalf("ParseWebSocketEvent() error = %v", err)
			}

			cs := event.ConnectionStateUpdated
			if cs == nil {
				t.Fatal("ConnectionStateUpdated is nil")
			}

			if cs.State != tt.wantState {
				t.Errorf("State = %q, want %q", cs.State, tt.wantState)
			}

			if cs.Up != tt.wantUp {
				t.Errorf("Up = %v, want %v", cs.Up, tt.wantUp)
			}

			if cs.Signal != tt.wantSignal {
				t.Errorf("Signal = %q, want %q", cs.Signal, tt.wantSignal)
			}

			if got := cs.IsConnected(); got != tt.wantUp {
				t.Errorf("IsConnected() = %v, want %v", got, tt.wantUp)
			}

			if got := cs.GetSignalStrength(); got != tt.wantSignal {
				t.Errorf("GetSignalStrength() = %q, want %q", got, tt.wantSignal)
			}
		})
	}
}

// TestConnectionStateUpdatedEvent_IsConnectedReadsUp documents why
// IsConnected does not compare State against ConnectionStateConnected: the
// device never sends that bare value, so a string match would report every
// healthy speaker as disconnected.
func TestConnectionStateUpdatedEvent_IsConnectedReadsUp(t *testing.T) {
	event := &ConnectionStateUpdatedEvent{State: "NETWORK_WIFI_CONNECTED", Up: true}
	if !event.IsConnected() {
		t.Error("IsConnected() = false for up=\"true\"; must read the up attribute")
	}

	stale := &ConnectionStateUpdatedEvent{State: string(ConnectionStateConnected), Up: false}
	if stale.IsConnected() {
		t.Error("IsConnected() = true for up=\"false\"; State must not override up")
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

	t.Run("ValidGroupUpdatedEvent", func(t *testing.T) {
		// The device fans this out to both ROLE devices when a stereo
		// pair is created via POST /addGroup.
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="9070658C9D4A">
	<groupUpdated deviceID="9070658C9D4A">
		<group id="1234567">
			<name>Living Room Pair</name>
			<masterDeviceId>9070658C9D4A</masterDeviceId>
			<roles>
				<groupRole>
					<deviceId>9070658C9D4A</deviceId>
					<role>LEFT</role>
					<ipAddress>192.0.2.131</ipAddress>
				</groupRole>
				<groupRole>
					<deviceId>F45EAB3115DA</deviceId>
					<role>RIGHT</role>
					<ipAddress>192.0.2.134</ipAddress>
				</groupRole>
			</roles>
			<status>GROUP_OK</status>
		</group>
	</groupUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent: %v", err)
		}

		if !event.HasEventType(EventTypeGroupUpdated) {
			t.Fatal("HasEventType(EventTypeGroupUpdated) = false, want true")
		}

		if event.GroupUpdated == nil {
			t.Fatal("GroupUpdated is nil")
		}

		g := event.GroupUpdated.Group

		if g.ID != "1234567" {
			t.Errorf("group ID = %q, want 1234567", g.ID)
		}

		if g.MasterDeviceID != "9070658C9D4A" {
			t.Errorf("MasterDeviceID = %q", g.MasterDeviceID)
		}

		if len(g.Roles.Roles) != 2 || g.Roles.Roles[0].Role != "LEFT" || g.Roles.Roles[1].Role != "RIGHT" {
			t.Errorf("roles not parsed as LEFT/RIGHT: %+v", g.Roles.Roles)
		}

		if g.Status != "GROUP_OK" {
			t.Errorf("status = %q, want GROUP_OK", g.Status)
		}
	})

	t.Run("GroupUpdatedTeardown", func(t *testing.T) {
		// On /removeGroup, the device emits a groupUpdated with an empty
		// <group/> body. Parsing must surface that as IsEmpty=true so the
		// UI can render "pair dissolved" cleanly.
		xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="9070658C9D4A">
	<groupUpdated deviceID="9070658C9D4A">
		<group/>
	</groupUpdated>
</updates>`

		event, err := ParseWebSocketEvent([]byte(xmlData))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent: %v", err)
		}

		if event.GroupUpdated == nil {
			t.Fatal("GroupUpdated is nil")
		}

		if !event.GroupUpdated.Group.IsEmpty() {
			t.Errorf("Group.IsEmpty() = false on teardown; got %+v", event.GroupUpdated.Group)
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

// TestParseWebSocketEvent_UnknownElements verifies that an <updates> envelope
// whose only child is an unmodeled element parses with no known event types
// but with the element captured by name, so callers can log something useful
// instead of an empty list.
func TestParseWebSocketEvent_UnknownElements(t *testing.T) {
	raw := []byte(`<updates deviceID="DEVICEID01"><someFutureUpdated><whatever/></someFutureUpdated></updates>`)

	event, err := ParseWebSocketEvent(raw)
	if err != nil {
		t.Fatalf("ParseWebSocketEvent: %v", err)
	}

	if got := event.GetEventTypes(); len(got) != 0 {
		t.Errorf("expected no known event types, got %v", got)
	}

	names := event.UnknownEventNames()
	if len(names) != 1 || names[0] != "someFutureUpdated" {
		t.Errorf("expected [someFutureUpdated], got %v", names)
	}
}

// TestParseWebSocketEvent_NowSelectionUpdated pins the two shapes captured on
// SoundTouch 10 firmware: a TuneIn station and a stored-music container. The
// preset id is 0 in both, because neither selection came from a preset slot.
func TestParseWebSocketEvent_NowSelectionUpdated(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantSource   string
		wantType     string
		wantLocation string
		wantItemName string
	}{
		{
			name:         "tunein station",
			raw:          `<updates deviceID="DEVICEID01"><nowSelectionUpdated><preset id="0"><ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s308770" sourceAccount="" isPresetable="true"><itemName>A Station</itemName></ContentItem></preset></nowSelectionUpdated></updates>`,
			wantSource:   "TUNEIN",
			wantType:     "stationurl",
			wantLocation: "/v1/playback/station/s308770",
			wantItemName: "A Station",
		},
		{
			name:         "stored music container",
			raw:          `<updates deviceID="DEVICEID01"><nowSelectionUpdated><preset id="0"><ContentItem source="STORED_MUSIC" type="dir" location="4:cont2:615:part12:39" isPresetable="true"><itemName>A Folder</itemName></ContentItem></preset></nowSelectionUpdated></updates>`,
			wantSource:   "STORED_MUSIC",
			wantType:     "dir",
			wantLocation: "4:cont2:615:part12:39",
			wantItemName: "A Folder",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event, err := ParseWebSocketEvent([]byte(test.raw))
			if err != nil {
				t.Fatalf("ParseWebSocketEvent: %v", err)
			}

			if names := event.UnknownEventNames(); len(names) != 0 {
				t.Errorf("modeled event still reported as unknown: %v", names)
			}

			types := event.GetEventTypes()
			if len(types) != 1 || types[0] != EventTypeNowSelectionUpdated {
				t.Fatalf("event types = %v, want [%s]", types, EventTypeNowSelectionUpdated)
			}

			selection := event.NowSelectionUpdated
			if selection == nil {
				t.Fatal("NowSelectionUpdated is nil")
			}

			// The device ID lives on <updates> and is copied down to children.
			if selection.DeviceID != "DEVICEID01" {
				t.Errorf("deviceID = %q, want DEVICEID01", selection.DeviceID)
			}

			if _, ok := selection.PresetID(); ok {
				t.Error("preset id 0 reported as a stored preset")
			}

			item := selection.SelectedContentItem()
			if item == nil {
				t.Fatal("SelectedContentItem is nil")
			}

			if item.Source != test.wantSource || item.Type != test.wantType ||
				item.Location != test.wantLocation || item.ItemName != test.wantItemName {
				t.Errorf("content item = %+v, want source=%q type=%q location=%q itemName=%q",
					item, test.wantSource, test.wantType, test.wantLocation, test.wantItemName)
			}
		})
	}
}

// A non-zero preset id names the slot the selection came from.
func TestNowSelectionUpdatedReportsAPresetSlot(t *testing.T) {
	raw := []byte(`<updates deviceID="DEVICEID01"><nowSelectionUpdated><preset id="4"><ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s1" isPresetable="true"><itemName>A Station</itemName></ContentItem></preset></nowSelectionUpdated></updates>`)

	event, err := ParseWebSocketEvent(raw)
	if err != nil {
		t.Fatalf("ParseWebSocketEvent: %v", err)
	}

	id, ok := event.NowSelectionUpdated.PresetID()
	if !ok || id != 4 {
		t.Errorf("PresetID() = (%d, %t), want (4, true)", id, ok)
	}
}

// The accessors are nil-safe, since a frame may carry no preset at all.
func TestNowSelectionUpdatedAccessorsTolerateEmptyFrames(t *testing.T) {
	var nilEvent *NowSelectionUpdatedEvent

	if nilEvent.SelectedContentItem() != nil {
		t.Error("SelectedContentItem on a nil event is not nil")
	}

	if _, ok := nilEvent.PresetID(); ok {
		t.Error("PresetID on a nil event reported a slot")
	}

	empty := &NowSelectionUpdatedEvent{}
	if empty.SelectedContentItem() != nil {
		t.Error("SelectedContentItem without a preset is not nil")
	}
}

// TestParseWebSocketEvent_KnownEventNoUnknowns confirms a modeled event is not
// also captured as an unknown element.
func TestParseWebSocketEvent_KnownEventNoUnknowns(t *testing.T) {
	raw := []byte(`<updates deviceID="A81B6A536A98"><nowPlayingUpdated><nowPlaying deviceID="A81B6A536A98" source="TUNEIN"></nowPlaying></nowPlayingUpdated></updates>`)

	event, err := ParseWebSocketEvent(raw)
	if err != nil {
		t.Fatalf("ParseWebSocketEvent: %v", err)
	}

	if names := event.UnknownEventNames(); len(names) != 0 {
		t.Errorf("expected no unknown elements for a modeled event, got %v", names)
	}
}

// TestParseSpecialMessage covers the non-<updates> frames the speaker sends.
//
// The errorUpdate cases are verbatim captures from a SoundTouch 10
// (variant=rhino, moduleType=sm2, FW 27.0.6), with the device ID replaced.
// Before GH-701 they fell through to "unknown special message type", so every
// device-side error was lost — which is exactly the diagnostic information
// playback bug reports keep lacking.
func TestParseSpecialMessage(t *testing.T) {
	t.Run("errorUpdate — STORED_MUSIC_AP_TIMEOUT", func(t *testing.T) {
		data := []byte(`<errorUpdate deviceID="DEVICEID01">` +
			`<error value="1654" name="STORED_MUSIC_AP_TIMEOUT" severity="Unrecoverable">APServer: Timeout</error>` +
			`</errorUpdate>`)

		msg, err := ParseSpecialMessage(data)
		if err != nil {
			t.Fatalf("ParseSpecialMessage() error = %v", err)
		}

		if msg.Type != MessageTypeErrorUpdate {
			t.Errorf("Type = %q, want %q", msg.Type, MessageTypeErrorUpdate)
		}

		if msg.DeviceID != "DEVICEID01" {
			t.Errorf("DeviceID = %q, want DEVICEID01", msg.DeviceID)
		}

		update := msg.GetErrorUpdate()
		if update == nil {
			t.Fatal("GetErrorUpdate() = nil")
		}

		if update.Error.Value != "1654" {
			t.Errorf("Error.Value = %q, want 1654", update.Error.Value)
		}

		if update.Error.Name != "STORED_MUSIC_AP_TIMEOUT" {
			t.Errorf("Error.Name = %q, want STORED_MUSIC_AP_TIMEOUT", update.Error.Name)
		}

		if update.Error.Severity != "Unrecoverable" {
			t.Errorf("Error.Severity = %q, want Unrecoverable", update.Error.Severity)
		}

		if update.Error.Text != "APServer: Timeout" {
			t.Errorf("Error.Text = %q, want %q", update.Error.Text, "APServer: Timeout")
		}
	})

	t.Run("errorUpdate — AUDIO_ERROR_TIMEOUT", func(t *testing.T) {
		data := []byte(`<errorUpdate deviceID="DEVICEID01">` +
			`<error value="3103" name="AUDIO_ERROR_TIMEOUT" severity="Unknown">AudioPath error4, reason 1</error>` +
			`</errorUpdate>`)

		msg, err := ParseSpecialMessage(data)
		if err != nil {
			t.Fatalf("ParseSpecialMessage() error = %v", err)
		}

		update := msg.GetErrorUpdate()
		if update == nil {
			t.Fatal("GetErrorUpdate() = nil")
		}

		if update.Error.Value != "3103" || update.Error.Name != "AUDIO_ERROR_TIMEOUT" {
			t.Errorf("got %s/%s, want 3103/AUDIO_ERROR_TIMEOUT", update.Error.Value, update.Error.Name)
		}

		if !strings.Contains(msg.String(), "AUDIO_ERROR_TIMEOUT") {
			t.Errorf("String() = %q, want it to name the error", msg.String())
		}
	})

	t.Run("SoundTouchSdkInfo", func(t *testing.T) {
		msg, err := ParseSpecialMessage([]byte(`<SoundTouchSdkInfo serverVersion="4" serverBuild="trunk r46330" />`))
		if err != nil {
			t.Fatalf("ParseSpecialMessage() error = %v", err)
		}

		if msg.Type != MessageTypeSdkInfo {
			t.Errorf("Type = %q, want %q", msg.Type, MessageTypeSdkInfo)
		}

		if msg.GetErrorUpdate() != nil {
			t.Error("GetErrorUpdate() must be nil for a non-error message")
		}
	})

	t.Run("userActivityUpdate", func(t *testing.T) {
		msg, err := ParseSpecialMessage([]byte(`<userActivityUpdate deviceID="DEVICEID01" />`))
		if err != nil {
			t.Fatalf("ParseSpecialMessage() error = %v", err)
		}

		if msg.Type != MessageTypeUserActivity {
			t.Errorf("Type = %q, want %q", msg.Type, MessageTypeUserActivity)
		}
	})

	t.Run("userInactivityUpdate", func(t *testing.T) {
		msg, err := ParseSpecialMessage([]byte(`<userInactivityUpdate deviceID="DEVICEID01" />`))
		if err != nil {
			t.Fatalf("ParseSpecialMessage() error = %v", err)
		}

		if msg.Type != MessageTypeUserInactivity {
			t.Errorf("Type = %q, want %q", msg.Type, MessageTypeUserInactivity)
		}
	})

	t.Run("unknown type still reports an error", func(t *testing.T) {
		if _, err := ParseSpecialMessage([]byte(`<somethingElse deviceID="DEVICEID01" />`)); err == nil {
			t.Fatal("expected an error for an unmodelled special message")
		}
	})
}

// TestParseSpecialMessage_ErrorUpdatedIsNotErrorUpdate guards the shared
// prefix: "<errorUpdate" is also a prefix of "<errorUpdated", the (never
// observed) <updates> child. A bare-prefix match in ParseSpecialMessage would
// swallow the past-tense element.
func TestParseSpecialMessage_ErrorUpdatedIsNotErrorUpdate(t *testing.T) {
	data := []byte(`<updates deviceID="DEVICEID01">` +
		`<errorUpdated deviceID="DEVICEID01"><error value="7" name="SOMETHING">boom</error></errorUpdated>` +
		`</updates>`)

	event, err := ParseWebSocketEvent(data)
	if err != nil {
		t.Fatalf("ParseWebSocketEvent() error = %v", err)
	}

	if event.ErrorUpdated == nil {
		t.Fatal("ErrorUpdated is nil; <errorUpdated> must still parse as an <updates> child")
	}

	if event.ErrorUpdated.Error.Name != "SOMETHING" {
		t.Errorf("Error.Name = %q, want SOMETHING", event.ErrorUpdated.Error.Name)
	}

	// The root-level branch must not claim a bare <errorUpdated> frame.
	if _, err := ParseSpecialMessage([]byte(`<errorUpdated deviceID="DEVICEID01"/>`)); err == nil {
		t.Error("ParseSpecialMessage accepted <errorUpdated>; only <errorUpdate> is a special message")
	}
}

// TestParseSpecialMessage_ErrorUpdateShapeVariants guards the root-element
// dispatch against shapes a substring needle would miss: attributes wrapped
// onto the next line, and a self-closing element.
func TestParseSpecialMessage_ErrorUpdateShapeVariants(t *testing.T) {
	tests := []struct {
		name    string
		xmlData string
	}{
		{
			name:    "attributes on the next line",
			xmlData: "<errorUpdate\n  deviceID=\"DEVICEID01\">\n  <error value=\"1654\" name=\"STORED_MUSIC_AP_TIMEOUT\"/>\n</errorUpdate>",
		},
		{
			name:    "self-closing",
			xmlData: `<errorUpdate deviceID="DEVICEID01"/>`,
		},
		{
			name:    "with an XML prolog",
			xmlData: `<?xml version="1.0" encoding="UTF-8" ?><errorUpdate deviceID="DEVICEID01"><error value="7"/></errorUpdate>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseSpecialMessage([]byte(tt.xmlData))
			if err != nil {
				t.Fatalf("ParseSpecialMessage() error = %v", err)
			}

			if msg.Type != MessageTypeErrorUpdate {
				t.Errorf("Type = %q, want %q", msg.Type, MessageTypeErrorUpdate)
			}

			if msg.DeviceID != "DEVICEID01" {
				t.Errorf("DeviceID = %q, want DEVICEID01", msg.DeviceID)
			}
		})
	}
}

func TestRootElementName(t *testing.T) {
	tests := []struct {
		name    string
		xmlData string
		want    string
		wantErr bool
	}{
		{"plain", `<updates deviceID="X"/>`, "updates", false},
		{"prolog skipped", `<?xml version="1.0" ?><status>/setup</status>`, "status", false},
		{"leading whitespace", "\n  <errorUpdate/>", "errorUpdate", false},
		{"not xml", `not xml at all`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RootElementName([]byte(tt.xmlData))
			if (err != nil) != tt.wantErr {
				t.Fatalf("RootElementName() error = %v, wantErr %v", err, tt.wantErr)
			}

			if got != tt.want {
				t.Errorf("RootElementName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPayloadFreeUpdateFramesCarryNoValue pins a shape that cost a real bug:
// the speaker sends some <xUpdated> elements EMPTY, as a "re-read" signal
// rather than a value.
//
// Frequencies across the reference captures (an ST-20 and both members of an
// ST-10 pair, FW 27.0.6): bassUpdated 4 of 4 empty, presetsUpdated 3 of 9
// empty, balanceUpdated always empty. Decoding a value out of one yields the
// zero value, and storing that overwrites the real setting — the Player's bass
// slider snapping to 0 on every change was exactly this.
func TestPayloadFreeUpdateFramesCarryNoValue(t *testing.T) {
	t.Run("empty bassUpdated carries no bass", func(t *testing.T) {
		// Captured verbatim, single-quoted attribute included.
		event, err := ParseWebSocketEvent(
			[]byte(`<updates deviceID='DEVICEID01'><bassUpdated></bassUpdated></updates>`))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent: %v", err)
		}

		if event.BassUpdated == nil {
			t.Fatal("BassUpdated is nil; the element was present")
		}

		if event.BassUpdated.HasPayload() {
			t.Error("HasPayload() = true for an empty frame; level 0 would be fabricated")
		}

		if event.BassUpdated.Bass != nil {
			t.Errorf("Bass = %+v, want nil for a signal-only frame", event.BassUpdated.Bass)
		}
	})

	t.Run("bassUpdated with a value still decodes", func(t *testing.T) {
		event, err := ParseWebSocketEvent([]byte(
			`<updates deviceID="DEVICEID01"><bassUpdated><bass deviceID="DEVICEID01">` +
				`<targetbass>-5</targetbass><actualbass>-5</actualbass></bass></bassUpdated></updates>`))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent: %v", err)
		}

		if !event.BassUpdated.HasPayload() {
			t.Fatal("HasPayload() = false for a frame that carries a value")
		}

		if event.BassUpdated.Bass.ActualBass != -5 {
			t.Errorf("ActualBass = %d, want -5", event.BassUpdated.Bass.ActualBass)
		}
	})

	t.Run("self-closing presetsUpdated carries no list", func(t *testing.T) {
		event, err := ParseWebSocketEvent(
			[]byte(`<updates deviceID="DEVICEID01"><presetsUpdated/></updates>`))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent: %v", err)
		}

		if event.PresetUpdated == nil {
			t.Fatal("PresetUpdated is nil; the element was present")
		}

		if event.PresetUpdated.HasPayload() {
			t.Error("HasPayload() = true for a bare signal; an empty list would blank the UI")
		}
	})

	t.Run("presetsUpdated with a list still decodes", func(t *testing.T) {
		event, err := ParseWebSocketEvent([]byte(
			`<updates deviceID="DEVICEID01"><presetsUpdated><presets>` +
				`<preset id="1"><ContentItem source="TUNEIN"><itemName>X</itemName></ContentItem></preset>` +
				`</presets></presetsUpdated></updates>`))
		if err != nil {
			t.Fatalf("ParseWebSocketEvent: %v", err)
		}

		if !event.PresetUpdated.HasPayload() {
			t.Fatal("HasPayload() = false for a frame that carries a list")
		}

		if len(event.PresetUpdated.Presets.Preset) != 1 {
			t.Errorf("got %d presets, want 1", len(event.PresetUpdated.Presets.Preset))
		}
	})
}

// TestDeviceIDPropagatesToChildEvents pins that a typed handler can tell which
// speaker an event came from.
//
// The device ID appears only on <updates>: across the reference captures, 226
// child elements carry no deviceID attribute and none carries one. Typed
// handlers receive only the child, so without propagation every DeviceID is
// empty — which is what printed the bare "[]" in the CLI's event output.
func TestDeviceIDPropagatesToChildEvents(t *testing.T) {
	tests := []struct {
		name    string
		xmlData string
		got     func(*WebSocketEvent) string
	}{
		{
			name: "volumeUpdated",
			xmlData: `<updates deviceID="DEVICEID01"><volumeUpdated><volume>` +
				`<targetvolume>16</targetvolume><actualvolume>16</actualvolume></volume></volumeUpdated></updates>`,
			got: func(e *WebSocketEvent) string { return e.VolumeUpdated.DeviceID },
		},
		{
			name:    "bassUpdated, even when payload-free",
			xmlData: `<updates deviceID='DEVICEID01'><bassUpdated></bassUpdated></updates>`,
			got:     func(e *WebSocketEvent) string { return e.BassUpdated.DeviceID },
		},
		{
			name: "nowPlayingUpdated",
			xmlData: `<updates deviceID="DEVICEID01"><nowPlayingUpdated>` +
				`<nowPlaying source="SPOTIFY"></nowPlaying></nowPlayingUpdated></updates>`,
			got: func(e *WebSocketEvent) string { return e.NowPlayingUpdated.DeviceID },
		},
		{
			name: "connectionStateUpdated",
			xmlData: `<updates deviceID="DEVICEID01">` +
				`<connectionStateUpdated state="NETWORK_WIFI_CONNECTED" up="true" signal="GOOD_SIGNAL" /></updates>`,
			got: func(e *WebSocketEvent) string { return e.ConnectionStateUpdated.DeviceID },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseWebSocketEvent([]byte(tt.xmlData))
			if err != nil {
				t.Fatalf("ParseWebSocketEvent: %v", err)
			}

			if got := tt.got(event); got != "DEVICEID01" {
				t.Errorf("child DeviceID = %q, want DEVICEID01 (from the parent <updates>)", got)
			}
		})
	}
}

// TestDeviceIDPropagationKeepsAnExplicitChildValue guards the direction of the
// copy: the parent fills in a gap, it does not overwrite.
func TestDeviceIDPropagationKeepsAnExplicitChildValue(t *testing.T) {
	event, err := ParseWebSocketEvent([]byte(
		`<updates deviceID="PARENT01"><nameUpdated deviceID="CHILD02">` +
			`<name>Kitchen</name></nameUpdated></updates>`))
	if err != nil {
		t.Fatalf("ParseWebSocketEvent: %v", err)
	}

	if event.NameUpdated.DeviceID != "CHILD02" {
		t.Errorf("DeviceID = %q, want the child's own value to win", event.NameUpdated.DeviceID)
	}
}
