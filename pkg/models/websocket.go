package models

import (
	"encoding/xml"
	"fmt"
	"time"
)

// WebSocketEventType represents the type of WebSocket event
type WebSocketEventType string

const (
	// Event types as documented in SoundTouch API
	EventTypeNowPlaying          WebSocketEventType = "nowPlayingUpdated"
	EventTypeVolumeUpdated       WebSocketEventType = "volumeUpdated"
	EventTypeConnectionState     WebSocketEventType = "connectionStateUpdated"
	EventTypePresetUpdated       WebSocketEventType = "presetUpdated"
	EventTypeZoneUpdated         WebSocketEventType = "zoneUpdated"
	EventTypeBassUpdated         WebSocketEventType = "bassUpdated"
	EventTypeClockTimeUpdated    WebSocketEventType = "clockTimeUpdated"
	EventTypeClockDisplayUpdated WebSocketEventType = "clockDisplayUpdated"
	EventTypeNameUpdated         WebSocketEventType = "nameUpdated"
	EventTypeErrorUpdated        WebSocketEventType = "errorUpdated"
	EventTypeRecentsUpdated      WebSocketEventType = "recentsUpdated"
	EventTypeLanguageUpdated     WebSocketEventType = "languageUpdated"
	EventTypeUnknown             WebSocketEventType = "unknown"
)

// String returns a human-readable string representation
func (e WebSocketEventType) String() string {
	switch e {
	case EventTypeNowPlaying:
		return "Now Playing Updated"
	case EventTypeVolumeUpdated:
		return "Volume Updated"
	case EventTypeConnectionState:
		return "Connection State Updated"
	case EventTypePresetUpdated:
		return "Preset Updated"
	case EventTypeZoneUpdated:
		return "Zone Updated"
	case EventTypeBassUpdated:
		return "Bass Updated"
	case EventTypeClockTimeUpdated:
		return "Clock Time Updated"
	case EventTypeClockDisplayUpdated:
		return "Clock Display Updated"
	case EventTypeNameUpdated:
		return "Name Updated"
	case EventTypeErrorUpdated:
		return "Error Updated"
	case EventTypeRecentsUpdated:
		return "Recents Updated"
	case EventTypeLanguageUpdated:
		return "Language Updated"
	default:
		return "Unknown Event"
	}
}

// WebSocketEvent represents a generic WebSocket event from SoundTouch device
type WebSocketEvent struct {
	XMLName                xml.Name                     `xml:"updates"`
	DeviceID               string                       `xml:"deviceID,attr"`
	NowPlayingUpdated      *NowPlayingUpdatedEvent      `xml:"nowPlayingUpdated,omitempty"`
	VolumeUpdated          *VolumeUpdatedEvent          `xml:"volumeUpdated,omitempty"`
	ConnectionStateUpdated *ConnectionStateUpdatedEvent `xml:"connectionStateUpdated,omitempty"`
	PresetUpdated          *PresetUpdatedEvent          `xml:"presetUpdated,omitempty"`
	ZoneUpdated            *ZoneUpdatedEvent            `xml:"zoneUpdated,omitempty"`
	BassUpdated            *BassUpdatedEvent            `xml:"bassUpdated,omitempty"`
	ClockTimeUpdated       *ClockTimeUpdatedEvent       `xml:"clockTimeUpdated,omitempty"`
	ClockDisplayUpdated    *ClockDisplayUpdatedEvent    `xml:"clockDisplayUpdated,omitempty"`
	NameUpdated            *NameUpdatedEvent            `xml:"nameUpdated,omitempty"`
	ErrorUpdated           *ErrorUpdatedEvent           `xml:"errorUpdated,omitempty"`
	RecentsUpdated         *RecentsUpdatedEvent         `xml:"recentsUpdated,omitempty"`
	LanguageUpdated        *LanguageUpdatedEvent        `xml:"languageUpdated,omitempty"`
	Timestamp              time.Time                    `json:"timestamp"` // Added by client for tracking
}

// WebSocketMessage represents a single event message within the updates
type WebSocketMessage struct {
	XMLName   xml.Name           `json:"-"`
	EventType WebSocketEventType `json:"eventType"`
	Content   interface{}        `json:"content"`
}

// GetEventType returns the event type based on the XML element name
func (m *WebSocketMessage) GetEventType() WebSocketEventType {
	switch m.XMLName.Local {
	case "nowPlayingUpdated":
		return EventTypeNowPlaying
	case "volumeUpdated":
		return EventTypeVolumeUpdated
	case "connectionStateUpdated":
		return EventTypeConnectionState
	case "presetUpdated":
		return EventTypePresetUpdated
	case "zoneUpdated":
		return EventTypeZoneUpdated
	case "bassUpdated":
		return EventTypeBassUpdated
	case "clockTimeUpdated":
		return EventTypeClockTimeUpdated
	case "clockDisplayUpdated":
		return EventTypeClockDisplayUpdated
	case "nameUpdated":
		return EventTypeNameUpdated
	case "errorUpdated":
		return EventTypeErrorUpdated
	case "recentsUpdated":
		return EventTypeRecentsUpdated
	case "languageUpdated":
		return EventTypeLanguageUpdated
	default:
		return EventTypeUnknown
	}
}

// GetEvents returns all events present in this WebSocket event
func (e *WebSocketEvent) GetEvents() []interface{} {
	var events []interface{}

	if e.NowPlayingUpdated != nil {
		events = append(events, e.NowPlayingUpdated)
	}
	if e.VolumeUpdated != nil {
		events = append(events, e.VolumeUpdated)
	}
	if e.ConnectionStateUpdated != nil {
		events = append(events, e.ConnectionStateUpdated)
	}
	if e.PresetUpdated != nil {
		events = append(events, e.PresetUpdated)
	}
	if e.ZoneUpdated != nil {
		events = append(events, e.ZoneUpdated)
	}
	if e.BassUpdated != nil {
		events = append(events, e.BassUpdated)
	}
	if e.ClockTimeUpdated != nil {
		events = append(events, e.ClockTimeUpdated)
	}
	if e.ClockDisplayUpdated != nil {
		events = append(events, e.ClockDisplayUpdated)
	}
	if e.NameUpdated != nil {
		events = append(events, e.NameUpdated)
	}
	if e.ErrorUpdated != nil {
		events = append(events, e.ErrorUpdated)
	}
	if e.RecentsUpdated != nil {
		events = append(events, e.RecentsUpdated)
	}
	if e.LanguageUpdated != nil {
		events = append(events, e.LanguageUpdated)
	}

	return events
}

// NowPlayingUpdatedEvent represents a now playing update event
type NowPlayingUpdatedEvent struct {
	XMLName    xml.Name   `xml:"nowPlayingUpdated"`
	DeviceID   string     `xml:"deviceID,attr"`
	NowPlaying NowPlaying `xml:"nowPlaying"`
}

// VolumeUpdatedEvent represents a volume update event
type VolumeUpdatedEvent struct {
	XMLName  xml.Name `xml:"volumeUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Volume   Volume   `xml:"volume"`
}

// ConnectionStateUpdatedEvent represents a connection state update event
type ConnectionStateUpdatedEvent struct {
	XMLName         xml.Name        `xml:"connectionStateUpdated"`
	DeviceID        string          `xml:"deviceID,attr"`
	ConnectionState ConnectionState `xml:"connectionState"`
}

// ConnectionState represents the device's network connection state
type ConnectionState struct {
	XMLName xml.Name `xml:"connectionState"`
	State   string   `xml:"state,attr"`
	Signal  string   `xml:"signal,attr"`
}

// ConnectionStateType represents connection state values
type ConnectionStateType string

const (
	ConnectionStateConnected    ConnectionStateType = "CONNECTED"
	ConnectionStateDisconnected ConnectionStateType = "DISCONNECTED"
	ConnectionStateConnecting   ConnectionStateType = "CONNECTING"
)

// IsConnected returns true if the device is connected
func (cs *ConnectionState) IsConnected() bool {
	return cs.State == string(ConnectionStateConnected)
}

// GetSignalStrength returns the signal strength as a string
func (cs *ConnectionState) GetSignalStrength() string {
	return cs.Signal
}

// PresetUpdatedEvent represents a preset update event
type PresetUpdatedEvent struct {
	XMLName  xml.Name `xml:"presetUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Preset   Preset   `xml:"preset"`
}

// ZoneUpdatedEvent represents a multiroom zone update event
type ZoneUpdatedEvent struct {
	XMLName  xml.Name `xml:"zoneUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Zone     Zone     `xml:"zone"`
}

// Zone represents multiroom zone information
type Zone struct {
	XMLName xml.Name     `xml:"zone"`
	Master  string       `xml:"master,attr"`
	Members []ZoneMember `xml:"member"`
}

// ZoneMember represents a member of a multiroom zone
type ZoneMember struct {
	XMLName  xml.Name `xml:"member"`
	DeviceID string   `xml:",chardata"`
	IP       string   `xml:"ipaddress,attr"`
}

// BassUpdatedEvent represents a bass setting update event
type BassUpdatedEvent struct {
	XMLName  xml.Name `xml:"bassUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Bass     Bass     `xml:"bass"`
}

// ClockTimeUpdatedEvent represents a clock time update event
type ClockTimeUpdatedEvent struct {
	XMLName   xml.Name  `xml:"clockTimeUpdated"`
	DeviceID  string    `xml:"deviceID,attr"`
	ClockTime ClockTime `xml:"clockTime"`
}

// ClockDisplayUpdatedEvent represents a clock display setting update event
type ClockDisplayUpdatedEvent struct {
	XMLName      xml.Name     `xml:"clockDisplayUpdated"`
	DeviceID     string       `xml:"deviceID,attr"`
	ClockDisplay ClockDisplay `xml:"clockDisplay"`
}

// NameUpdatedEvent represents a device name update event
type NameUpdatedEvent struct {
	XMLName  xml.Name `xml:"nameUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Name     Name     `xml:"name"`
}

// ErrorUpdatedEvent represents an error state update event
type ErrorUpdatedEvent struct {
	XMLName  xml.Name `xml:"errorUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Error    Error    `xml:"error"`
}

// Error represents an error state
type Error struct {
	XMLName xml.Name `xml:"error"`
	Value   string   `xml:"value,attr"`
	Name    string   `xml:"name,attr"`
	Text    string   `xml:",chardata"`
}

// RecentsUpdatedEvent represents a recent items update event
type RecentsUpdatedEvent struct {
	XMLName  xml.Name `xml:"recentsUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Recents  Recents  `xml:"recents"`
}

// Recents represents recently played items
type Recents struct {
	XMLName xml.Name     `xml:"recents"`
	Items   []RecentItem `xml:"recent"`
}

// RecentItem represents a recently played item
type RecentItem struct {
	XMLName     xml.Name    `xml:"recent"`
	DeviceID    string      `xml:"deviceID,attr"`
	CreatedOn   int64       `xml:"createdOn,attr"`
	ID          string      `xml:"id,attr"`
	ContentItem ContentItem `xml:"ContentItem"`
}

// LanguageUpdatedEvent represents a language setting update event
type LanguageUpdatedEvent struct {
	XMLName  xml.Name `xml:"languageUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Language Language `xml:"language"`
}

// Language represents language settings
type Language struct {
	XMLName xml.Name `xml:"language"`
	Value   string   `xml:",chardata"`
}

// EventHandler represents a function that handles WebSocket events
type EventHandler func(event *WebSocketEvent)

// TypedEventHandler represents a function that handles specific event types
type TypedEventHandler[T any] func(event T)

// WebSocketEventHandlers holds typed event handlers for different event types
type WebSocketEventHandlers struct {
	OnNowPlaying          TypedEventHandler[*NowPlayingUpdatedEvent]
	OnVolumeUpdated       TypedEventHandler[*VolumeUpdatedEvent]
	OnConnectionState     TypedEventHandler[*ConnectionStateUpdatedEvent]
	OnPresetUpdated       TypedEventHandler[*PresetUpdatedEvent]
	OnZoneUpdated         TypedEventHandler[*ZoneUpdatedEvent]
	OnBassUpdated         TypedEventHandler[*BassUpdatedEvent]
	OnClockTimeUpdated    TypedEventHandler[*ClockTimeUpdatedEvent]
	OnClockDisplayUpdated TypedEventHandler[*ClockDisplayUpdatedEvent]
	OnNameUpdated         TypedEventHandler[*NameUpdatedEvent]
	OnErrorUpdated        TypedEventHandler[*ErrorUpdatedEvent]
	OnRecentsUpdated      TypedEventHandler[*RecentsUpdatedEvent]
	OnLanguageUpdated     TypedEventHandler[*LanguageUpdatedEvent]
	OnUnknownEvent        EventHandler
}

// ParseWebSocketEvent attempts to parse a WebSocket message into a specific event type
func ParseWebSocketEvent(data []byte) (*WebSocketEvent, error) {
	var event WebSocketEvent
	if err := xml.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to parse WebSocket event: %w", err)
	}

	// Add timestamp
	event.Timestamp = time.Now()

	return &event, nil
}

// ParseTypedEvent attempts to parse a WebSocket event into a specific typed event
func ParseTypedEvent[T any](event *WebSocketEvent, eventType WebSocketEventType) (T, error) {
	var result T

	// Get the event directly from the parsed structure
	switch eventType {
	case EventTypeNowPlaying:
		if event.NowPlayingUpdated != nil {
			if typedResult, ok := interface{}(event.NowPlayingUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeVolumeUpdated:
		if event.VolumeUpdated != nil {
			if typedResult, ok := interface{}(event.VolumeUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeConnectionState:
		if event.ConnectionStateUpdated != nil {
			if typedResult, ok := interface{}(event.ConnectionStateUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypePresetUpdated:
		if event.PresetUpdated != nil {
			if typedResult, ok := interface{}(event.PresetUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeZoneUpdated:
		if event.ZoneUpdated != nil {
			if typedResult, ok := interface{}(event.ZoneUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeBassUpdated:
		if event.BassUpdated != nil {
			if typedResult, ok := interface{}(event.BassUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeClockTimeUpdated:
		if event.ClockTimeUpdated != nil {
			if typedResult, ok := interface{}(event.ClockTimeUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeClockDisplayUpdated:
		if event.ClockDisplayUpdated != nil {
			if typedResult, ok := interface{}(event.ClockDisplayUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeNameUpdated:
		if event.NameUpdated != nil {
			if typedResult, ok := interface{}(event.NameUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeErrorUpdated:
		if event.ErrorUpdated != nil {
			if typedResult, ok := interface{}(event.ErrorUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeRecentsUpdated:
		if event.RecentsUpdated != nil {
			if typedResult, ok := interface{}(event.RecentsUpdated).(T); ok {
				return typedResult, nil
			}
		}
	case EventTypeLanguageUpdated:
		if event.LanguageUpdated != nil {
			if typedResult, ok := interface{}(event.LanguageUpdated).(T); ok {
				return typedResult, nil
			}
		}
	}

	return result, fmt.Errorf("event type %s not found in WebSocket event", eventType)
}

// HasEventType checks if the WebSocket event contains a specific event type
func (e *WebSocketEvent) HasEventType(eventType WebSocketEventType) bool {
	switch eventType {
	case EventTypeNowPlaying:
		return e.NowPlayingUpdated != nil
	case EventTypeVolumeUpdated:
		return e.VolumeUpdated != nil
	case EventTypeConnectionState:
		return e.ConnectionStateUpdated != nil
	case EventTypePresetUpdated:
		return e.PresetUpdated != nil
	case EventTypeZoneUpdated:
		return e.ZoneUpdated != nil
	case EventTypeBassUpdated:
		return e.BassUpdated != nil
	case EventTypeClockTimeUpdated:
		return e.ClockTimeUpdated != nil
	case EventTypeClockDisplayUpdated:
		return e.ClockDisplayUpdated != nil
	case EventTypeNameUpdated:
		return e.NameUpdated != nil
	case EventTypeErrorUpdated:
		return e.ErrorUpdated != nil
	case EventTypeRecentsUpdated:
		return e.RecentsUpdated != nil
	case EventTypeLanguageUpdated:
		return e.LanguageUpdated != nil
	}
	return false
}

// GetEventTypes returns all event types present in this WebSocket event
func (e *WebSocketEvent) GetEventTypes() []WebSocketEventType {
	var types []WebSocketEventType

	if e.NowPlayingUpdated != nil {
		types = append(types, EventTypeNowPlaying)
	}
	if e.VolumeUpdated != nil {
		types = append(types, EventTypeVolumeUpdated)
	}
	if e.ConnectionStateUpdated != nil {
		types = append(types, EventTypeConnectionState)
	}
	if e.PresetUpdated != nil {
		types = append(types, EventTypePresetUpdated)
	}
	if e.ZoneUpdated != nil {
		types = append(types, EventTypeZoneUpdated)
	}
	if e.BassUpdated != nil {
		types = append(types, EventTypeBassUpdated)
	}
	if e.ClockTimeUpdated != nil {
		types = append(types, EventTypeClockTimeUpdated)
	}
	if e.ClockDisplayUpdated != nil {
		types = append(types, EventTypeClockDisplayUpdated)
	}
	if e.NameUpdated != nil {
		types = append(types, EventTypeNameUpdated)
	}
	if e.ErrorUpdated != nil {
		types = append(types, EventTypeErrorUpdated)
	}
	if e.RecentsUpdated != nil {
		types = append(types, EventTypeRecentsUpdated)
	}
	if e.LanguageUpdated != nil {
		types = append(types, EventTypeLanguageUpdated)
	}

	return types
}

// String returns a human-readable string representation of the WebSocket event
func (e *WebSocketEvent) String() string {
	events := e.GetEvents()
	eventTypes := e.GetEventTypes()

	if len(events) == 0 {
		return fmt.Sprintf("WebSocket Event [Device: %s] - No events", e.DeviceID)
	}

	if len(events) == 1 {
		return fmt.Sprintf("WebSocket Event [Device: %s] - %s", e.DeviceID, eventTypes[0].String())
	}

	return fmt.Sprintf("WebSocket Event [Device: %s] - %d events", e.DeviceID, len(events))
}
