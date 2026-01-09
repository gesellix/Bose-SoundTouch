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
	// Playback Controls
	KeyPlay      = "PLAY"
	KeyPause     = "PAUSE"
	KeyStop      = "STOP"
	KeyPrevTrack = "PREV_TRACK"
	KeyNextTrack = "NEXT_TRACK"

	// Rating and Bookmark Controls
	KeyThumbsUp   = "THUMBS_UP"
	KeyThumbsDown = "THUMBS_DOWN"
	KeyBookmark   = "BOOKMARK"

	// Power and System Controls
	KeyPower = "POWER"
	KeyMute  = "MUTE"

	// Volume Controls
	KeyVolumeUp   = "VOLUME_UP"
	KeyVolumeDown = "VOLUME_DOWN"

	// Preset Controls
	KeyPreset1 = "PRESET_1"
	KeyPreset2 = "PRESET_2"
	KeyPreset3 = "PRESET_3"
	KeyPreset4 = "PRESET_4"
	KeyPreset5 = "PRESET_5"
	KeyPreset6 = "PRESET_6"

	// Input Controls
	KeyAuxInput = "AUX_INPUT"

	// Shuffle Controls
	KeyShuffleOff = "SHUFFLE_OFF"
	KeyShuffleOn  = "SHUFFLE_ON"

	// Repeat Controls
	KeyRepeatOff = "REPEAT_OFF"
	KeyRepeatOne = "REPEAT_ONE"
	KeyRepeatAll = "REPEAT_ALL"
)

// NewKey creates a new key press command
// Note: For proper key simulation, use client.SendKey() which sends both press and release
func NewKey(keyValue string) *Key {
	return &Key{
		State:  KeyStatePress,
		Sender: "Gabbo",
		Value:  keyValue,
	}
}

// NewKeyPress creates a new key press command (alias for NewKey)
// Note: This creates only the press state. For complete key simulation, use client.SendKey()
func NewKeyPress(keyValue string) *Key {
	return NewKey(keyValue)
}

// NewKeyRelease creates a new key release command
// Note: This creates only the release state. For complete key simulation, use client.SendKey()
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
		// Playback Controls
		KeyPlay:      true,
		KeyPause:     true,
		KeyStop:      true,
		KeyPrevTrack: true,
		KeyNextTrack: true,

		// Rating and Bookmark Controls
		KeyThumbsUp:   true,
		KeyThumbsDown: true,
		KeyBookmark:   true,

		// Power and System Controls
		KeyPower: true,
		KeyMute:  true,

		// Volume Controls
		KeyVolumeUp:   true,
		KeyVolumeDown: true,

		// Preset Controls
		KeyPreset1: true,
		KeyPreset2: true,
		KeyPreset3: true,
		KeyPreset4: true,
		KeyPreset5: true,
		KeyPreset6: true,

		// Input Controls
		KeyAuxInput: true,

		// Shuffle Controls
		KeyShuffleOff: true,
		KeyShuffleOn:  true,

		// Repeat Controls
		KeyRepeatOff: true,
		KeyRepeatOne: true,
		KeyRepeatAll: true,
	}
	return validKeys[keyValue]
}

// GetAllValidKeys returns a slice of all valid key values
func GetAllValidKeys() []string {
	return []string{
		// Playback Controls
		KeyPlay,
		KeyPause,
		KeyStop,
		KeyPrevTrack,
		KeyNextTrack,

		// Rating and Bookmark Controls
		KeyThumbsUp,
		KeyThumbsDown,
		KeyBookmark,

		// Power and System Controls
		KeyPower,
		KeyMute,

		// Volume Controls
		KeyVolumeUp,
		KeyVolumeDown,

		// Preset Controls
		KeyPreset1,
		KeyPreset2,
		KeyPreset3,
		KeyPreset4,
		KeyPreset5,
		KeyPreset6,

		// Input Controls
		KeyAuxInput,

		// Shuffle Controls
		KeyShuffleOff,
		KeyShuffleOn,

		// Repeat Controls
		KeyRepeatOff,
		KeyRepeatOne,
		KeyRepeatAll,
	}
}
