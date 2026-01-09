package models

import (
	"encoding/xml"
	"testing"
)

func TestNewKey(t *testing.T) {
	key := NewKey(KeyPlay)

	if key.State != KeyStatePress {
		t.Errorf("Expected state %q, got %q", KeyStatePress, key.State)
	}

	if key.Sender != "Gabbo" {
		t.Errorf("Expected sender %q, got %q", "Gabbo", key.Sender)
	}

	if key.Value != KeyPlay {
		t.Errorf("Expected value %q, got %q", KeyPlay, key.Value)
	}
}

func TestNewKeyPress(t *testing.T) {
	key := NewKeyPress(KeyPause)

	if key.State != KeyStatePress {
		t.Errorf("Expected state %q, got %q", KeyStatePress, key.State)
	}

	if key.Value != KeyPause {
		t.Errorf("Expected value %q, got %q", KeyPause, key.Value)
	}
}

func TestNewKeyRelease(t *testing.T) {
	key := NewKeyRelease(KeyStop)

	if key.State != KeyStateRelease {
		t.Errorf("Expected state %q, got %q", KeyStateRelease, key.State)
	}

	if key.Sender != "Gabbo" {
		t.Errorf("Expected sender %q, got %q", "Gabbo", key.Sender)
	}

	if key.Value != KeyStop {
		t.Errorf("Expected value %q, got %q", KeyStop, key.Value)
	}
}

func TestKeyXMLMarshal(t *testing.T) {
	tests := []struct {
		name        string
		key         *Key
		expectedXML string
	}{
		{
			name:        "PLAY key press",
			key:         NewKey(KeyPlay),
			expectedXML: `<key state="press" sender="Gabbo">PLAY</key>`,
		},
		{
			name:        "PAUSE key press",
			key:         NewKey(KeyPause),
			expectedXML: `<key state="press" sender="Gabbo">PAUSE</key>`,
		},
		{
			name:        "STOP key release",
			key:         NewKeyRelease(KeyStop),
			expectedXML: `<key state="release" sender="Gabbo">STOP</key>`,
		},
		{
			name:        "VOLUME_UP key press",
			key:         NewKey(KeyVolumeUp),
			expectedXML: `<key state="press" sender="Gabbo">VOLUME_UP</key>`,
		},
		{
			name:        "PRESET_1 key press",
			key:         NewKey(KeyPreset1),
			expectedXML: `<key state="press" sender="Gabbo">PRESET_1</key>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xmlData, err := xml.Marshal(tt.key)
			if err != nil {
				t.Fatalf("Failed to marshal XML: %v", err)
			}

			if string(xmlData) != tt.expectedXML {
				t.Errorf("Expected XML %q, got %q", tt.expectedXML, string(xmlData))
			}
		})
	}
}

func TestKeyXMLUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		xmlData string
		want    Key
	}{
		{
			name:    "PLAY key press",
			xmlData: `<key state="press" sender="Gabbo">PLAY</key>`,
			want: Key{
				State:  KeyStatePress,
				Sender: "Gabbo",
				Value:  KeyPlay,
			},
		},
		{
			name:    "PAUSE key release",
			xmlData: `<key state="release" sender="TestSender">PAUSE</key>`,
			want: Key{
				State:  KeyStateRelease,
				Sender: "TestSender",
				Value:  KeyPause,
			},
		},
		{
			name:    "VOLUME_DOWN key press",
			xmlData: `<key state="press" sender="WebApp">VOLUME_DOWN</key>`,
			want: Key{
				State:  KeyStatePress,
				Sender: "WebApp",
				Value:  KeyVolumeDown,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var key Key
			err := xml.Unmarshal([]byte(tt.xmlData), &key)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if key.State != tt.want.State {
				t.Errorf("Expected state %q, got %q", tt.want.State, key.State)
			}

			if key.Sender != tt.want.Sender {
				t.Errorf("Expected sender %q, got %q", tt.want.Sender, key.Sender)
			}

			if key.Value != tt.want.Value {
				t.Errorf("Expected value %q, got %q", tt.want.Value, key.Value)
			}
		})
	}
}

func TestIsValidKey(t *testing.T) {
	validKeys := []string{
		// Playback Controls
		KeyPlay, KeyPause, KeyStop,
		KeyPrevTrack, KeyNextTrack,

		// Rating and Bookmark Controls
		KeyThumbsUp, KeyThumbsDown, KeyBookmark,

		// Power and System Controls
		KeyPower, KeyMute,

		// Volume Controls
		KeyVolumeUp, KeyVolumeDown,

		// Preset Controls
		KeyPreset1, KeyPreset2, KeyPreset3,
		KeyPreset4, KeyPreset5, KeyPreset6,

		// Input Controls
		KeyAuxInput,

		// Shuffle Controls
		KeyShuffleOff, KeyShuffleOn,

		// Repeat Controls
		KeyRepeatOff, KeyRepeatOne, KeyRepeatAll,
	}

	invalidKeys := []string{
		"INVALID_KEY", "play", "PAUSE_BUTTON",
		"PRESET_7", "PRESET_0", "", "VOLUME",
	}

	for _, key := range validKeys {
		t.Run("valid_"+key, func(t *testing.T) {
			if !IsValidKey(key) {
				t.Errorf("Expected %q to be valid", key)
			}
		})
	}

	for _, key := range invalidKeys {
		t.Run("invalid_"+key, func(t *testing.T) {
			if IsValidKey(key) {
				t.Errorf("Expected %q to be invalid", key)
			}
		})
	}
}

func TestGetAllValidKeys(t *testing.T) {
	keys := GetAllValidKeys()

	expectedKeys := []string{
		// Playback Controls
		KeyPlay, KeyPause, KeyStop,
		KeyPrevTrack, KeyNextTrack,

		// Rating and Bookmark Controls
		KeyThumbsUp, KeyThumbsDown, KeyBookmark,

		// Power and System Controls
		KeyPower, KeyMute,

		// Volume Controls
		KeyVolumeUp, KeyVolumeDown,

		// Preset Controls
		KeyPreset1, KeyPreset2, KeyPreset3,
		KeyPreset4, KeyPreset5, KeyPreset6,

		// Input Controls
		KeyAuxInput,

		// Shuffle Controls
		KeyShuffleOff, KeyShuffleOn,

		// Repeat Controls
		KeyRepeatOff, KeyRepeatOne, KeyRepeatAll,
	}

	if len(keys) != len(expectedKeys) {
		t.Errorf("Expected %d keys, got %d", len(expectedKeys), len(keys))
	}

	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	for _, expectedKey := range expectedKeys {
		if !keyMap[expectedKey] {
			t.Errorf("Expected key %q not found in result", expectedKey)
		}
	}

	// Verify all returned keys are valid
	for _, key := range keys {
		if !IsValidKey(key) {
			t.Errorf("GetAllValidKeys returned invalid key: %q", key)
		}
	}
}

func TestKeyConstants(t *testing.T) {
	// Test that all key constants are defined and non-empty
	keyTests := []struct {
		name  string
		value string
	}{
		{"KeyPlay", KeyPlay},
		{"KeyPause", KeyPause},
		{"KeyStop", KeyStop},
		{"KeyPrevTrack", KeyPrevTrack},
		{"KeyNextTrack", KeyNextTrack},
		{"KeyVolumeUp", KeyVolumeUp},
		{"KeyVolumeDown", KeyVolumeDown},
		{"KeyPreset1", KeyPreset1},
		{"KeyPreset2", KeyPreset2},
		{"KeyPreset3", KeyPreset3},
		{"KeyPreset4", KeyPreset4},
		{"KeyPreset5", KeyPreset5},
		{"KeyPreset6", KeyPreset6},
	}

	for _, tt := range keyTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("Expected %s to be non-empty", tt.name)
			}
		})
	}
}

func TestKeyStateConstants(t *testing.T) {
	if KeyStatePress == "" {
		t.Error("KeyStatePress should not be empty")
	}

	if KeyStateRelease == "" {
		t.Error("KeyStateRelease should not be empty")
	}

	if KeyStatePress == KeyStateRelease {
		t.Error("KeyStatePress and KeyStateRelease should be different")
	}
}

// Benchmark tests
func BenchmarkNewKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewKey(KeyPlay)
	}
}

func BenchmarkKeyXMLMarshal(b *testing.B) {
	key := NewKey(KeyPlay)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		xml.Marshal(key)
	}
}

func BenchmarkIsValidKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsValidKey(KeyPlay)
	}
}

// Test that demonstrates the press+release pattern from API documentation
func TestKeyPressReleasePattern(t *testing.T) {
	// According to API docs, we should send press followed by release
	keyValue := KeyPlay

	// Create press command
	keyPress := NewKey(keyValue)
	if keyPress.State != KeyStatePress {
		t.Errorf("Expected press state, got %s", keyPress.State)
	}
	if keyPress.Value != keyValue {
		t.Errorf("Expected key value %s, got %s", keyValue, keyPress.Value)
	}
	if keyPress.Sender != "Gabbo" {
		t.Errorf("Expected sender 'Gabbo', got %s", keyPress.Sender)
	}

	// Create release command
	keyRelease := NewKeyRelease(keyValue)
	if keyRelease.State != KeyStateRelease {
		t.Errorf("Expected release state, got %s", keyRelease.State)
	}
	if keyRelease.Value != keyValue {
		t.Errorf("Expected key value %s, got %s", keyValue, keyRelease.Value)
	}
	if keyRelease.Sender != "Gabbo" {
		t.Errorf("Expected sender 'Gabbo', got %s", keyRelease.Sender)
	}

	// Test XML marshaling for both
	pressXML, err := xml.Marshal(keyPress)
	if err != nil {
		t.Fatalf("Failed to marshal press XML: %v", err)
	}
	expectedPressXML := `<key state="press" sender="Gabbo">PLAY</key>`
	if string(pressXML) != expectedPressXML {
		t.Errorf("Press XML: got %s, want %s", string(pressXML), expectedPressXML)
	}

	releaseXML, err := xml.Marshal(keyRelease)
	if err != nil {
		t.Fatalf("Failed to marshal release XML: %v", err)
	}
	expectedReleaseXML := `<key state="release" sender="Gabbo">PLAY</key>`
	if string(releaseXML) != expectedReleaseXML {
		t.Errorf("Release XML: got %s, want %s", string(releaseXML), expectedReleaseXML)
	}
}
