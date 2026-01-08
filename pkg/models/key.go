package models

import "encoding/xml"

// Key represents a key press command for the /key endpoint
type Key struct {
	XMLName xml.Name `xml:"key"`
	State   string   `xml:"state,attr"`
	Sender  string   `xml:"sender,attr"`
	Value   string   `xml:",chardata"`
}

// KeyState constants for key press states
const (
	KeyStatePress   = "press"
	KeyStateRelease = "release"
)

// KeyValue constants for available keys
const (
	KeyPlay       = "PLAY"
	KeyPause      = "PAUSE"
	KeyStop       = "STOP"
	KeyPrevTrack  = "PREV_TRACK"
	KeyNextTrack  = "NEXT_TRACK"
	KeyVolumeUp   = "VOLUME_UP"
	KeyVolumeDown = "VOLUME_DOWN"
	KeyPreset1    = "PRESET_1"
	KeyPreset2    = "PRESET_2"
	KeyPreset3    = "PRESET_3"
	KeyPreset4    = "PRESET_4"
	KeyPreset5    = "PRESET_5"
	KeyPreset6    = "PRESET_6"
)

// NewKey creates a new key press command
func NewKey(keyValue string) *Key {
	return &Key{
		State:  KeyStatePress,
		Sender: "Gabbo",
		Value:  keyValue,
	}
}

// NewKeyPress creates a new key press command (alias for NewKey)
func NewKeyPress(keyValue string) *Key {
	return NewKey(keyValue)
}

// NewKeyRelease creates a new key release command
func NewKeyRelease(keyValue string) *Key {
	return &Key{
		State:  KeyStateRelease,
		Sender: "Gabbo",
		Value:  keyValue,
	}
}

// IsValidKey checks if the key value is valid
func IsValidKey(keyValue string) bool {
	validKeys := map[string]bool{
		KeyPlay:       true,
		KeyPause:      true,
		KeyStop:       true,
		KeyPrevTrack:  true,
		KeyNextTrack:  true,
		KeyVolumeUp:   true,
		KeyVolumeDown: true,
		KeyPreset1:    true,
		KeyPreset2:    true,
		KeyPreset3:    true,
		KeyPreset4:    true,
		KeyPreset5:    true,
		KeyPreset6:    true,
	}
	return validKeys[keyValue]
}

// GetAllValidKeys returns a slice of all valid key values
func GetAllValidKeys() []string {
	return []string{
		KeyPlay,
		KeyPause,
		KeyStop,
		KeyPrevTrack,
		KeyNextTrack,
		KeyVolumeUp,
		KeyVolumeDown,
		KeyPreset1,
		KeyPreset2,
		KeyPreset3,
		KeyPreset4,
		KeyPreset5,
		KeyPreset6,
	}
}
