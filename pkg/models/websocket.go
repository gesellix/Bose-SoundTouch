package models

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// WebSocketEventType represents the type of WebSocket event
type WebSocketEventType string

const (
	// EventTypeNowPlaying indicates a now playing status update
	EventTypeNowPlaying WebSocketEventType = "nowPlayingUpdated"
	// EventTypeVolumeUpdated indicates a volume level change
	EventTypeVolumeUpdated WebSocketEventType = "volumeUpdated"
	// EventTypeConnectionState indicates a connection state change
	EventTypeConnectionState WebSocketEventType = "connectionStateUpdated"
	// EventTypePresetUpdated indicates a preset configuration change
	EventTypePresetUpdated WebSocketEventType = "presetsUpdated"
	// EventTypeZoneUpdated indicates a zone configuration change
	EventTypeZoneUpdated WebSocketEventType = "zoneUpdated"
	// EventTypeGroupUpdated is emitted to both ROLE devices when an ST-10
	// stereo pair is created, renamed, or removed via /addGroup,
	// /updateGroup, or /removeGroup.
	EventTypeGroupUpdated WebSocketEventType = "groupUpdated"
	// EventTypeBassUpdated indicates a bass level change
	EventTypeBassUpdated WebSocketEventType = "bassUpdated"
	// EventTypeClockTimeUpdated indicates a clock time change
	EventTypeClockTimeUpdated WebSocketEventType = "clockTimeUpdated"
	// EventTypeClockDisplayUpdated indicates a clock display setting change
	EventTypeClockDisplayUpdated WebSocketEventType = "clockDisplayUpdated"
	// EventTypeNameUpdated indicates a device name change
	EventTypeNameUpdated WebSocketEventType = "nameUpdated"
	// EventTypeErrorUpdated indicates an error status change
	EventTypeErrorUpdated WebSocketEventType = "errorUpdated"
	// EventTypeRecentsUpdated indicates a recent items list change
	EventTypeRecentsUpdated WebSocketEventType = "recentsUpdated"
	// EventTypeLanguageUpdated indicates a language setting change
	EventTypeLanguageUpdated WebSocketEventType = "languageUpdated"
	// EventTypePairDeviceWithAccount indicates a device pairing request
	EventTypePairDeviceWithAccount WebSocketEventType = "PairDeviceWithAccount"
	// EventTypeUnPairDeviceWithAccount indicates a device unpairing request
	EventTypeUnPairDeviceWithAccount WebSocketEventType = "UnPairDeviceWithAccount"
	// EventTypeUnknown indicates an unrecognized event type
	EventTypeUnknown WebSocketEventType = "unknown"
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
	case EventTypeGroupUpdated:
		return "Stereo Pair Updated"
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
	case EventTypePairDeviceWithAccount:
		return "Pair Device With Account"
	case EventTypeUnPairDeviceWithAccount:
		return "UnPair Device With Account"
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
	PresetUpdated          *PresetUpdatedEvent          `xml:"presetsUpdated,omitempty"`
	ZoneUpdated            *ZoneUpdatedEvent            `xml:"zoneUpdated,omitempty"`
	GroupUpdated           *GroupUpdatedEvent           `xml:"groupUpdated,omitempty"`
	BassUpdated            *BassUpdatedEvent            `xml:"bassUpdated,omitempty"`
	ClockTimeUpdated       *ClockTimeUpdatedEvent       `xml:"clockTimeUpdated,omitempty"`
	ClockDisplayUpdated    *ClockDisplayUpdatedEvent    `xml:"clockDisplayUpdated,omitempty"`
	NameUpdated            *NameUpdatedEvent            `xml:"nameUpdated,omitempty"`
	ErrorUpdated           *ErrorUpdatedEvent           `xml:"errorUpdated,omitempty"`
	RecentsUpdated         *RecentsUpdatedEvent         `xml:"recentsUpdated,omitempty"`
	LanguageUpdated        *LanguageUpdatedEvent        `xml:"languageUpdated,omitempty"`
	// UnknownElements captures <updates> children we don't model yet (e.g.
	// nowSelectionUpdated), so callers can log them by name instead of an
	// empty list when no known event matched.
	UnknownElements []UnknownElement `xml:",any"`
	Timestamp       time.Time        `json:"timestamp"` // Added by client for tracking
}

// UnknownElement records the tag name of an <updates> child element that the
// WebSocketEvent struct does not (yet) model.
type UnknownElement struct {
	XMLName xml.Name
}

// UnknownEventNames returns the tag names of any unmodeled <updates> children,
// for diagnostic logging.
func (e *WebSocketEvent) UnknownEventNames() []string {
	names := make([]string, 0, len(e.UnknownElements))
	for _, u := range e.UnknownElements {
		names = append(names, u.XMLName.Local)
	}

	return names
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

	if e.GroupUpdated != nil {
		events = append(events, e.GroupUpdated)
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
	// ConnectionStateConnected indicates the device is connected
	ConnectionStateConnected ConnectionStateType = "CONNECTED"
	// ConnectionStateDisconnected indicates the device is disconnected
	ConnectionStateDisconnected ConnectionStateType = "DISCONNECTED"
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
	XMLName  xml.Name `xml:"presetsUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Presets  Presets  `xml:"presets"`
}

// ZoneUpdatedEvent represents a multiroom zone update event
type ZoneUpdatedEvent struct {
	XMLName  xml.Name `xml:"zoneUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Zone     Zone     `xml:"zone"`
}

// GroupUpdatedEvent represents an ST-10 stereo-pair update notification.
// The device fans this event out to both LEFT and RIGHT speakers whenever
// the pair is created, renamed, or removed. Group will be the zero value
// for a teardown notification — see (*Group).IsEmpty.
type GroupUpdatedEvent struct {
	XMLName  xml.Name `xml:"groupUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Group    Group    `xml:"group"`
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

// PairDeviceWithAccount represents a device pairing request message
type PairDeviceWithAccount struct {
	XMLName       xml.Name `xml:"PairDeviceWithAccount"`
	AccountID     string   `xml:"accountId"`
	UserAuthToken string   `xml:"userAuthToken"`
}

// UnPairDeviceWithAccount represents a device unpairing request message
type UnPairDeviceWithAccount struct {
	XMLName xml.Name `xml:"UnPairDeviceWithAccount"`
}

// SpecialMessageType represents message types that are not part of <updates>
type SpecialMessageType string

// Constants for special message types
const (
	MessageTypeSdkInfo        SpecialMessageType = "sdkInfo"
	MessageTypeUserActivity   SpecialMessageType = "userActivity"
	MessageTypeUserInactivity SpecialMessageType = "userInactivity"
)

// SoundTouchSdkInfo represents the SDK info message sent on connection
type SoundTouchSdkInfo struct {
	XMLName       xml.Name `xml:"SoundTouchSdkInfo"`
	ServerVersion string   `xml:"serverVersion,attr"`
	ServerBuild   string   `xml:"serverBuild,attr"`
}

// UserActivityUpdate represents user activity notifications
type UserActivityUpdate struct {
	XMLName  xml.Name `xml:"userActivityUpdate"`
	DeviceID string   `xml:"deviceID,attr"`
}

// UserInactivityUpdate represents user inactivity notifications
type UserInactivityUpdate struct {
	XMLName  xml.Name `xml:"userInactivityUpdate"`
	DeviceID string   `xml:"deviceID,attr"`
}

// SpecialMessage represents non-updates WebSocket messages
type SpecialMessage struct {
	Type      SpecialMessageType
	DeviceID  string
	Data      interface{}
	RawData   []byte
	Timestamp time.Time
}

// SpecialMessageHandler defines the signature for special message handlers
type SpecialMessageHandler func(message *SpecialMessage)

// EventHandler represents a function that handles WebSocket events
type EventHandler func(event *WebSocketEvent)

// TypedEventHandler represents a function that handles specific event types
type TypedEventHandler[T any] func(event T)

// WebSocketEventHandlers contains handlers for different types of WebSocket events
type WebSocketEventHandlers struct {
	OnNowPlaying          TypedEventHandler[*NowPlayingUpdatedEvent]
	OnVolumeUpdated       TypedEventHandler[*VolumeUpdatedEvent]
	OnConnectionState     TypedEventHandler[*ConnectionStateUpdatedEvent]
	OnPresetUpdated       TypedEventHandler[*PresetUpdatedEvent]
	OnZoneUpdated         TypedEventHandler[*ZoneUpdatedEvent]
	OnGroupUpdated        TypedEventHandler[*GroupUpdatedEvent]
	OnBassUpdated         TypedEventHandler[*BassUpdatedEvent]
	OnClockTimeUpdated    TypedEventHandler[*ClockTimeUpdatedEvent]
	OnClockDisplayUpdated TypedEventHandler[*ClockDisplayUpdatedEvent]
	OnNameUpdated         TypedEventHandler[*NameUpdatedEvent]
	OnErrorUpdated        TypedEventHandler[*ErrorUpdatedEvent]
	OnRecentsUpdated      TypedEventHandler[*RecentsUpdatedEvent]
	OnLanguageUpdated     TypedEventHandler[*LanguageUpdatedEvent]
	OnUnknownEvent        EventHandler
	OnSpecialMessage      SpecialMessageHandler
	// OnRawMessage fires for every received frame before any parsing
	// happens. Use it for debug/observability tooling that wants to see
	// exactly what the device sent on the wire — the typed handlers
	// above still run afterwards, independently. parseErr is the result
	// of the XML parse: nil for messages that decoded cleanly, non-nil
	// for malformed payloads. The slice is owned by the caller; copy
	// before retaining.
	OnRawMessage RawMessageHandler
}

// RawMessageHandler defines the signature for raw-frame handlers.
type RawMessageHandler func(data []byte, parseErr error)

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

func (e *WebSocketEvent) getFieldByEventType(eventType WebSocketEventType) interface{} {
	var field interface{}

	switch eventType {
	case EventTypeNowPlaying:
		field = e.NowPlayingUpdated
	case EventTypeVolumeUpdated:
		field = e.VolumeUpdated
	case EventTypeConnectionState:
		field = e.ConnectionStateUpdated
	case EventTypePresetUpdated:
		field = e.PresetUpdated
	case EventTypeZoneUpdated:
		field = e.ZoneUpdated
	case EventTypeGroupUpdated:
		field = e.GroupUpdated
	case EventTypeBassUpdated:
		field = e.BassUpdated
	case EventTypeClockTimeUpdated:
		field = e.ClockTimeUpdated
	case EventTypeClockDisplayUpdated:
		field = e.ClockDisplayUpdated
	case EventTypeNameUpdated:
		field = e.NameUpdated
	case EventTypeErrorUpdated:
		field = e.ErrorUpdated
	case EventTypeRecentsUpdated:
		field = e.RecentsUpdated
	case EventTypeLanguageUpdated:
		field = e.LanguageUpdated
	}

	// Use reflection or a type-safe check to ensure we only return non-nil interfaces
	// In Go, an interface is nil only if both its type and value are nil.
	// If e.NowPlayingUpdated is a nil pointer, field will be a non-nil interface containing a nil pointer.
	// We need to return a literal nil if the field is empty to satisfy expectations.

	if field == nil {
		return nil
	}

	// We know all these fields are pointers.
	// We can't easily check for nil pointer without reflection here in a generic way,
	// but we can restore the previous logic in a more compact way if needed.
	// Actually, the previous logic was: if e.NowPlayingUpdated != nil { return e.NowPlayingUpdated }
	// which returns a non-nil interface.

	return field
}

// isNil checks if an interface is nil or contains a nil pointer.
func isNil(i interface{}) bool {
	if i == nil {
		return true
	}

	switch v := i.(type) {
	case *NowPlayingUpdatedEvent:
		return v == nil
	case *VolumeUpdatedEvent:
		return v == nil
	case *ConnectionStateUpdatedEvent:
		return v == nil
	case *PresetUpdatedEvent:
		return v == nil
	case *ZoneUpdatedEvent:
		return v == nil
	case *GroupUpdatedEvent:
		return v == nil
	case *BassUpdatedEvent:
		return v == nil
	case *ClockTimeUpdatedEvent:
		return v == nil
	case *ClockDisplayUpdatedEvent:
		return v == nil
	case *NameUpdatedEvent:
		return v == nil
	case *ErrorUpdatedEvent:
		return v == nil
	case *RecentsUpdatedEvent:
		return v == nil
	case *LanguageUpdatedEvent:
		return v == nil
	}

	return false
}

// ParseTypedEvent attempts to parse a WebSocket event into a specific typed event
func ParseTypedEvent[T any](event *WebSocketEvent, eventType WebSocketEventType) (T, error) {
	var result T

	field := event.getFieldByEventType(eventType)
	if !isNil(field) {
		if typedResult, ok := field.(T); ok {
			return typedResult, nil
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
	case EventTypeGroupUpdated:
		return e.GroupUpdated != nil
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

	if e.GroupUpdated != nil {
		types = append(types, EventTypeGroupUpdated)
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
	eventTypes := e.GetEventTypes()
	if len(eventTypes) == 0 {
		return fmt.Sprintf("WebSocket Event [Device: %s] - No events", e.DeviceID)
	}

	if len(eventTypes) == 1 {
		return fmt.Sprintf("WebSocket Event [Device: %s] - %s", e.DeviceID, eventTypes[0].String())
	}

	return fmt.Sprintf("WebSocket Event [Device: %s] - %d events", e.DeviceID, len(eventTypes))
}

// ParseSpecialMessage parses non-updates WebSocket messages
func ParseSpecialMessage(data []byte) (*SpecialMessage, error) {
	dataStr := string(data)

	// Check for SoundTouchSdkInfo
	if strings.Contains(dataStr, "<SoundTouchSdkInfo") {
		var sdkInfo SoundTouchSdkInfo
		if err := xml.Unmarshal(data, &sdkInfo); err != nil {
			return nil, fmt.Errorf("failed to parse SoundTouchSdkInfo: %w", err)
		}

		return &SpecialMessage{
			Type:      MessageTypeSdkInfo,
			Data:      &sdkInfo,
			RawData:   data,
			Timestamp: time.Now(),
		}, nil
	}

	// Check for userActivityUpdate
	if strings.Contains(dataStr, "<userActivityUpdate") {
		var userActivity UserActivityUpdate
		if err := xml.Unmarshal(data, &userActivity); err != nil {
			return nil, fmt.Errorf("failed to parse userActivityUpdate: %w", err)
		}

		return &SpecialMessage{
			Type:      MessageTypeUserActivity,
			DeviceID:  userActivity.DeviceID,
			Data:      &userActivity,
			RawData:   data,
			Timestamp: time.Now(),
		}, nil
	}

	// Check for userInactivityUpdate
	if strings.Contains(dataStr, "<userInactivityUpdate") {
		var userInactivity UserInactivityUpdate
		if err := xml.Unmarshal(data, &userInactivity); err != nil {
			return nil, fmt.Errorf("failed to parse userInactivityUpdate: %w", err)
		}

		return &SpecialMessage{
			Type:      MessageTypeUserInactivity,
			DeviceID:  userInactivity.DeviceID,
			Data:      &userInactivity,
			RawData:   data,
			Timestamp: time.Now(),
		}, nil
	}

	return nil, fmt.Errorf("unknown special message type: %s", dataStr)
}

// GetSdkInfo returns the parsed SdkInfo data if the message is of that type
func (sm *SpecialMessage) GetSdkInfo() *SoundTouchSdkInfo {
	if sm.Type == MessageTypeSdkInfo {
		if sdkInfo, ok := sm.Data.(*SoundTouchSdkInfo); ok {
			return sdkInfo
		}
	}

	return nil
}

// GetUserActivity returns the parsed UserActivity data if the message is of that type
func (sm *SpecialMessage) GetUserActivity() *UserActivityUpdate {
	if sm.Type == MessageTypeUserActivity {
		if userActivity, ok := sm.Data.(*UserActivityUpdate); ok {
			return userActivity
		}
	}

	return nil
}

// GetUserInactivity returns the parsed UserInactivity data if the message is of that type
func (sm *SpecialMessage) GetUserInactivity() *UserInactivityUpdate {
	if sm.Type == MessageTypeUserInactivity {
		if userInactivity, ok := sm.Data.(*UserInactivityUpdate); ok {
			return userInactivity
		}
	}

	return nil
}

// String returns a string representation of the special message
func (sm *SpecialMessage) String() string {
	switch sm.Type {
	case MessageTypeSdkInfo:
		if sdkInfo := sm.GetSdkInfo(); sdkInfo != nil {
			return fmt.Sprintf("SoundTouch SDK Info - Version: %s, Build: %s", sdkInfo.ServerVersion, sdkInfo.ServerBuild)
		}
	case MessageTypeUserActivity:
		return fmt.Sprintf("User Activity [Device: %s]", sm.DeviceID)
	case MessageTypeUserInactivity:
		return fmt.Sprintf("User Inactivity [Device: %s]", sm.DeviceID)
	}

	return fmt.Sprintf("Unknown Special Message - Type: %s", sm.Type)
}
