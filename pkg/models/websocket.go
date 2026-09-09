package models

import (
	"bytes"
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
	// EventTypeBalanceUpdated indicates a stereo-pair balance change
	EventTypeBalanceUpdated WebSocketEventType = "balanceUpdated"
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
	case EventTypeBalanceUpdated:
		return "Balance Updated"
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
	BalanceUpdated         *BalanceUpdatedEvent         `xml:"balanceUpdated,omitempty"`
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

	if e.BalanceUpdated != nil {
		events = append(events, e.BalanceUpdated)
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

// ConnectionStateUpdatedEvent represents a connection state update event.
//
// The speaker puts everything in attributes on the element itself. Captured
// on a SoundTouch 10 (variant=rhino, moduleType=sm2, FW 27.0.6):
//
//	<updates deviceID="DEVICEID01">
//	  <connectionStateUpdated state="NETWORK_WIFI_CONNECTED" up="true"
//	                          signal="EXCELLENT_SIGNAL" />
//	</updates>
//
// There is no nested <connectionState> child. This type used to model one,
// which is why State and Signal were always empty (GH-701). The shape is not
// in Bose's published Web API document; it only exists in captures.
type ConnectionStateUpdatedEvent struct {
	XMLName  xml.Name `xml:"connectionStateUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	State    string   `xml:"state,attr"`
	Up       bool     `xml:"up,attr"`
	Signal   string   `xml:"signal,attr"`
}

// ConnectionStateType represents connection state values
type ConnectionStateType string

const (
	// ConnectionStateConnected indicates the device is connected.
	//
	// Kept for compatibility: no captured frame has ever carried this bare
	// value. Real speakers report transport-qualified states such as
	// ConnectionStateWiFiConnected, which is why IsConnected reads the
	// up attribute rather than comparing against these constants.
	ConnectionStateConnected ConnectionStateType = "CONNECTED"
	// ConnectionStateDisconnected indicates the device is disconnected
	ConnectionStateDisconnected ConnectionStateType = "DISCONNECTED"
	// ConnectionStateWiFiConnected is the state a Wi-Fi-attached speaker
	// actually reports (field-observed, FW 27.0.6).
	ConnectionStateWiFiConnected ConnectionStateType = "NETWORK_WIFI_CONNECTED"
)

// IsConnected reports whether the speaker considers its network link up.
//
// This reads the up attribute rather than matching State against
// ConnectionStateConnected: the device sends transport-qualified state names
// ("NETWORK_WIFI_CONNECTED"), so a string comparison would never match, while
// up is an unambiguous boolean the firmware sets on every frame we captured.
func (e *ConnectionStateUpdatedEvent) IsConnected() bool {
	return e.Up
}

// GetSignalStrength returns the signal strength as a string
func (e *ConnectionStateUpdatedEvent) GetSignalStrength() string {
	return e.Signal
}

// PresetUpdatedEvent represents a preset update event.
//
// Presets is OPTIONAL. The speaker sends this element both ways: with the full
// list, and as a bare <presetsUpdated/> carrying nothing (3 of 9 occurrences
// across the reference captures). A nil Presets means "re-read /presets", NOT
// "the speaker has no presets" — treating the empty case as data blanks a
// perfectly good preset list.
type PresetUpdatedEvent struct {
	XMLName  xml.Name `xml:"presetsUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Presets  *Presets `xml:"presets"`
}

// HasPayload reports whether this frame carried a preset list rather than
// being a bare re-read signal.
func (e *PresetUpdatedEvent) HasPayload() bool {
	return e != nil && e.Presets != nil
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

// BassUpdatedEvent represents a bass setting update event.
//
// Bass is OPTIONAL, and in practice absent: every bassUpdated frame in the
// reference captures is empty — <bassUpdated></bassUpdated>, 4 of 4 — and the
// live traces agree. It is a "re-read /bass" signal, the same shape as
// balanceUpdated.
//
// A nil Bass therefore means "no value was sent". Reading a value out of an
// empty frame yields a fabricated level 0, which then overwrites the real
// setting; the Player's bass slider snapping to 0 on every change was exactly
// that.
type BassUpdatedEvent struct {
	XMLName  xml.Name `xml:"bassUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Bass     *Bass    `xml:"bass"`
}

// HasPayload reports whether this frame carried a bass value rather than being
// a bare re-read signal.
func (e *BassUpdatedEvent) HasPayload() bool {
	return e != nil && e.Bass != nil
}

// BalanceUpdatedEvent signals that the stereo pair's balance changed.
//
// It carries NO payload and NO attributes — the captured frame is exactly
//
//	<updates deviceID="DEVICEID01"><balanceUpdated></balanceUpdated></updates>
//
// so it is a "re-read /balance" trigger, not a value. Confirmed on hardware
// after both an HTTP and a WebSocket write (FW 27.0.6).
//
// There is deliberately no DeviceID field: the device ID lives on the parent
// <updates> element (WebSocketEvent.DeviceID), and a deviceID attribute here
// would be permanently empty — the same shape of bug as the unpopulated
// ConnectionState this package used to carry (GH-701).
type BalanceUpdatedEvent struct {
	XMLName xml.Name `xml:"balanceUpdated"`
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

// ErrorUpdatedEvent represents an <errorUpdated> child of <updates>.
//
// No capture has ever contained this element. Real speakers report errors as
// a root-level <errorUpdate> frame (present tense, outside <updates>) — see
// ErrorUpdate, which is the shape that actually arrives on the wire. This type
// is retained because the past-tense name appears in third-party API notes and
// costs nothing to keep parsing.
type ErrorUpdatedEvent struct {
	XMLName  xml.Name `xml:"errorUpdated"`
	DeviceID string   `xml:"deviceID,attr"`
	Error    Error    `xml:"error"`
}

// Error represents an error state.
//
// Value is the numeric code, Name its symbolic form (e.g.
// "STORED_MUSIC_AP_TIMEOUT"), Severity one of "Unrecoverable" / "Unknown" as
// observed on FW 27.0.6, and Text the device's human-readable detail.
type Error struct {
	XMLName  xml.Name `xml:"error"`
	Value    string   `xml:"value,attr"`
	Name     string   `xml:"name,attr"`
	Severity string   `xml:"severity,attr"`
	Text     string   `xml:",chardata"`
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
	MessageTypeErrorUpdate    SpecialMessageType = "errorUpdate"
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

// ErrorUpdate is a root-level device-error notification.
//
// Note the present tense: this is NOT ErrorUpdatedEvent, which models an
// <errorUpdated> child of <updates> and has never been seen on the wire. The
// speaker sends errors unwrapped, at the top level (FW 27.0.6):
//
//	<errorUpdate deviceID="DEVICEID01">
//	  <error value="1654" name="STORED_MUSIC_AP_TIMEOUT"
//	         severity="Unrecoverable">APServer: Timeout</error>
//	</errorUpdate>
//
// These frames name a failure precisely — numeric code, symbolic name,
// severity — which makes them the most useful diagnostic the speaker offers
// when playback goes wrong. Before GH-701 they were dropped as an unknown
// special message type.
type ErrorUpdate struct {
	XMLName  xml.Name `xml:"errorUpdate"`
	DeviceID string   `xml:"deviceID,attr"`
	Error    Error    `xml:"error"`
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
	OnBalanceUpdated      TypedEventHandler[*BalanceUpdatedEvent]
	OnClockTimeUpdated    TypedEventHandler[*ClockTimeUpdatedEvent]
	OnClockDisplayUpdated TypedEventHandler[*ClockDisplayUpdatedEvent]
	OnNameUpdated         TypedEventHandler[*NameUpdatedEvent]
	OnErrorUpdated        TypedEventHandler[*ErrorUpdatedEvent]
	OnRecentsUpdated      TypedEventHandler[*RecentsUpdatedEvent]
	OnLanguageUpdated     TypedEventHandler[*LanguageUpdatedEvent]
	OnUnknownEvent        EventHandler
	// OnDeviceError fires for root-level <errorUpdate> frames — the
	// speaker's own error reports. OnSpecialMessage still fires for the
	// same frame; this handler exists so callers that only care about
	// device errors don't have to type-switch.
	OnDeviceError    TypedEventHandler[*ErrorUpdate]
	OnSpecialMessage SpecialMessageHandler
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

// UnmarshalXML decodes an <updates> frame and copies its deviceID down to the
// child events.
//
// The device ID appears ONLY on <updates>: across the reference captures, 226
// child elements carry no deviceID attribute and none carries one. Each child
// event type declares the field anyway, so without this every DeviceID handed
// to a typed handler is the empty string — which is what printed the bare "[]"
// in the CLI's event output.
//
// Typed handlers receive only the child, so the parent's value is the only
// place the device identity can come from.
func (e *WebSocketEvent) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// A distinct type avoids recursing back into this method.
	type rawEvent WebSocketEvent

	var raw rawEvent
	if err := d.DecodeElement(&raw, &start); err != nil {
		return err
	}

	*e = WebSocketEvent(raw)
	e.propagateDeviceID()

	return nil
}

// propagateDeviceID fills in each present child's DeviceID from the parent,
// leaving alone any that somehow arrived with one of its own.
func (e *WebSocketEvent) propagateDeviceID() {
	if e.DeviceID == "" {
		return
	}

	targets := []*string{}

	if e.NowPlayingUpdated != nil {
		targets = append(targets, &e.NowPlayingUpdated.DeviceID)
	}

	if e.VolumeUpdated != nil {
		targets = append(targets, &e.VolumeUpdated.DeviceID)
	}

	if e.ConnectionStateUpdated != nil {
		targets = append(targets, &e.ConnectionStateUpdated.DeviceID)
	}

	if e.PresetUpdated != nil {
		targets = append(targets, &e.PresetUpdated.DeviceID)
	}

	if e.ZoneUpdated != nil {
		targets = append(targets, &e.ZoneUpdated.DeviceID)
	}

	if e.GroupUpdated != nil {
		targets = append(targets, &e.GroupUpdated.DeviceID)
	}

	if e.BassUpdated != nil {
		targets = append(targets, &e.BassUpdated.DeviceID)
	}

	if e.ClockTimeUpdated != nil {
		targets = append(targets, &e.ClockTimeUpdated.DeviceID)
	}

	if e.ClockDisplayUpdated != nil {
		targets = append(targets, &e.ClockDisplayUpdated.DeviceID)
	}

	if e.NameUpdated != nil {
		targets = append(targets, &e.NameUpdated.DeviceID)
	}

	if e.ErrorUpdated != nil {
		targets = append(targets, &e.ErrorUpdated.DeviceID)
	}

	if e.RecentsUpdated != nil {
		targets = append(targets, &e.RecentsUpdated.DeviceID)
	}

	if e.LanguageUpdated != nil {
		targets = append(targets, &e.LanguageUpdated.DeviceID)
	}

	for _, target := range targets {
		if *target == "" {
			*target = e.DeviceID
		}
	}
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
	case EventTypeBalanceUpdated:
		field = e.BalanceUpdated
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
	case *BalanceUpdatedEvent:
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
	case EventTypeBalanceUpdated:
		return e.BalanceUpdated != nil
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

	if e.BalanceUpdated != nil {
		types = append(types, EventTypeBalanceUpdated)
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

// RootElementName returns the local name of an XML document's root element,
// skipping any prolog or leading whitespace.
//
// Frame dispatch keys off this rather than substring tests: element names
// share prefixes ("errorUpdate" / "errorUpdated"), attributes may wrap onto
// the next line, and an element may be self-closing — all of which defeat a
// needle like "<errorUpdate ".
func RootElementName(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}

		if start, ok := token.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
}

// ParseSpecialMessage parses non-updates WebSocket messages
func ParseSpecialMessage(data []byte) (*SpecialMessage, error) {
	dataStr := string(data)

	root, rootErr := RootElementName(data)

	// Check for errorUpdate. Matched on the root element rather than a
	// substring: "<errorUpdate" is also a prefix of "<errorUpdated", the
	// (never observed) <updates> child modelled by ErrorUpdatedEvent.
	if rootErr == nil && root == "errorUpdate" {
		var errorUpdate ErrorUpdate
		if err := xml.Unmarshal(data, &errorUpdate); err != nil {
			return nil, fmt.Errorf("failed to parse errorUpdate: %w", err)
		}

		return &SpecialMessage{
			Type:      MessageTypeErrorUpdate,
			DeviceID:  errorUpdate.DeviceID,
			Data:      &errorUpdate,
			RawData:   data,
			Timestamp: time.Now(),
		}, nil
	}

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

// GetErrorUpdate returns the parsed ErrorUpdate data if the message is of that type
func (sm *SpecialMessage) GetErrorUpdate() *ErrorUpdate {
	if sm.Type == MessageTypeErrorUpdate {
		if errorUpdate, ok := sm.Data.(*ErrorUpdate); ok {
			return errorUpdate
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
	case MessageTypeErrorUpdate:
		if errorUpdate := sm.GetErrorUpdate(); errorUpdate != nil {
			return fmt.Sprintf(
				"Device Error [Device: %s] - %s (%s), severity %s: %s",
				sm.DeviceID,
				errorUpdate.Error.Name,
				errorUpdate.Error.Value,
				errorUpdate.Error.Severity,
				strings.TrimSpace(errorUpdate.Error.Text),
			)
		}
	}

	return fmt.Sprintf("Unknown Special Message - Type: %s", sm.Type)
}
