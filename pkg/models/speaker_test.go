package models

import (
	"encoding/xml"
	"testing"
)

func TestNewPlayInfo(t *testing.T) {
	playInfo := NewPlayInfo("https://example.com/audio.mp3", "test-key", "Test Service", "Test Message", "Test Reason")

	if playInfo.URL != "https://example.com/audio.mp3" {
		t.Errorf("Expected URL 'https://example.com/audio.mp3', got '%s'", playInfo.URL)
	}

	if playInfo.AppKey != "test-key" {
		t.Errorf("Expected AppKey 'test-key', got '%s'", playInfo.AppKey)
	}

	if playInfo.Service != "Test Service" {
		t.Errorf("Expected Service 'Test Service', got '%s'", playInfo.Service)
	}

	if playInfo.Message != "Test Message" {
		t.Errorf("Expected Message 'Test Message', got '%s'", playInfo.Message)
	}

	if playInfo.Reason != "Test Reason" {
		t.Errorf("Expected Reason 'Test Reason', got '%s'", playInfo.Reason)
	}

	if playInfo.Volume != nil {
		t.Errorf("Expected Volume to be nil, got %v", *playInfo.Volume)
	}
}

func TestNewTTSPlayInfo(t *testing.T) {
	// Test without volume
	playInfo := NewTTSPlayInfo("Hello World", "test-key", "EN")

	expectedURL := "http://translate.google.com/translate_tts?ie=UTF-8&tl=EN&client=tw-ob&q=Hello+World"
	if playInfo.URL != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, playInfo.URL)
	}

	if playInfo.AppKey != "test-key" {
		t.Errorf("Expected AppKey 'test-key', got '%s'", playInfo.AppKey)
	}

	if playInfo.Service != "TTS Notification" {
		t.Errorf("Expected Service 'TTS Notification', got '%s'", playInfo.Service)
	}

	if playInfo.Message != "Google TTS" {
		t.Errorf("Expected Message 'Google TTS', got '%s'", playInfo.Message)
	}

	if playInfo.Reason != "Hello World" {
		t.Errorf("Expected Reason 'Hello World', got '%s'", playInfo.Reason)
	}

	if playInfo.Volume != nil {
		t.Errorf("Expected Volume to be nil, got %v", *playInfo.Volume)
	}

	// Test with volume
	playInfoWithVolume := NewTTSPlayInfo("Hello World", "test-key", "EN", 50)
	if playInfoWithVolume.Volume == nil || *playInfoWithVolume.Volume != 50 {
		t.Errorf("Expected Volume to be 50, got %v", playInfoWithVolume.Volume)
	}
}

func TestNewURLPlayInfo(t *testing.T) {
	// Test without volume
	playInfo := NewURLPlayInfo(
		"https://example.com/audio.mp3",
		"test-key",
		"Music Service",
		"Song Title",
		"Artist Name",
	)

	if playInfo.URL != "https://example.com/audio.mp3" {
		t.Errorf("Expected URL 'https://example.com/audio.mp3', got '%s'", playInfo.URL)
	}

	if playInfo.Service != "Music Service" {
		t.Errorf("Expected Service 'Music Service', got '%s'", playInfo.Service)
	}

	if playInfo.Message != "Song Title" {
		t.Errorf("Expected Message 'Song Title', got '%s'", playInfo.Message)
	}

	if playInfo.Reason != "Artist Name" {
		t.Errorf("Expected Reason 'Artist Name', got '%s'", playInfo.Reason)
	}

	// Test with volume
	playInfoWithVolume := NewURLPlayInfo(
		"https://example.com/audio.mp3",
		"test-key",
		"Music Service",
		"Song Title",
		"Artist Name",
		75,
	)

	if playInfoWithVolume.Volume == nil || *playInfoWithVolume.Volume != 75 {
		t.Errorf("Expected Volume to be 75, got %v", playInfoWithVolume.Volume)
	}
}

func TestSetVolume(t *testing.T) {
	playInfo := NewPlayInfo("https://example.com/audio.mp3", "test-key", "Service", "Message", "Reason")

	// Set volume and check fluent interface
	result := playInfo.SetVolume(60)

	// Check that it returns the same instance (fluent interface)
	if result != playInfo {
		t.Error("SetVolume should return the same instance for fluent interface")
	}

	// Check that volume was set correctly
	if playInfo.Volume == nil || *playInfo.Volume != 60 {
		t.Errorf("Expected Volume to be 60, got %v", playInfo.Volume)
	}
}

func TestPlayInfoValidate(t *testing.T) {
	tests := []struct {
		name        string
		playInfo    *PlayInfo
		expectedErr error
	}{
		{
			name: "valid PlayInfo",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "test-key",
				Service: "Test Service",
				Message: "Test Message",
				Reason:  "Test Reason",
			},
			expectedErr: nil,
		},
		{
			name: "empty URL",
			playInfo: &PlayInfo{
				URL:     "",
				AppKey:  "test-key",
				Service: "Test Service",
			},
			expectedErr: ErrInvalidURL,
		},
		{
			name: "empty AppKey",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "",
				Service: "Test Service",
			},
			expectedErr: ErrInvalidAppKey,
		},
		{
			name: "empty Service",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "test-key",
				Service: "",
			},
			expectedErr: ErrInvalidService,
		},
		{
			name: "negative volume",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "test-key",
				Service: "Test Service",
				Volume:  intPtr(-1),
			},
			expectedErr: ErrInvalidVolume,
		},
		{
			name: "volume too high",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "test-key",
				Service: "Test Service",
				Volume:  intPtr(101),
			},
			expectedErr: ErrInvalidVolume,
		},
		{
			name: "valid volume at boundary",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "test-key",
				Service: "Test Service",
				Volume:  intPtr(100),
			},
			expectedErr: nil,
		},
		{
			name: "valid volume at zero boundary",
			playInfo: &PlayInfo{
				URL:     "https://example.com/audio.mp3",
				AppKey:  "test-key",
				Service: "Test Service",
				Volume:  intPtr(0),
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.playInfo.Validate()
			if (err != nil && tt.expectedErr == nil) || (err == nil && tt.expectedErr != nil) || (err != nil && tt.expectedErr != nil && err.Error() != tt.expectedErr.Error()) {
				t.Errorf("Expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestPlayInfoString(t *testing.T) {
	// Test without volume
	playInfo := &PlayInfo{
		Service: "Test Service",
		Message: "Test Message",
	}

	expected := "Service: Test Service, Message: Test Message, Volume: current"
	result := playInfo.String()

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// Test with volume
	playInfo.SetVolume(75)

	expectedWithVolume := "Service: Test Service, Message: Test Message, Volume: K" // K is ASCII 75
	resultWithVolume := playInfo.String()

	if resultWithVolume != expectedWithVolume {
		t.Errorf("Expected '%s', got '%s'", expectedWithVolume, resultWithVolume)
	}
}

func TestPlayInfoXMLMarshaling(t *testing.T) {
	// Test XML marshaling
	playInfo := &PlayInfo{
		URL:     "https://example.com/audio.mp3",
		AppKey:  "test-key",
		Service: "Test Service",
		Message: "Test Message",
		Reason:  "Test Reason",
		Volume:  intPtr(50),
	}

	xmlData, err := xml.Marshal(playInfo)
	if err != nil {
		t.Fatalf("Failed to marshal PlayInfo to XML: %v", err)
	}

	expectedXML := `<play_info><url>https://example.com/audio.mp3</url><app_key>test-key</app_key><service>Test Service</service><message>Test Message</message><reason>Test Reason</reason><volume>50</volume></play_info>`
	if string(xmlData) != expectedXML {
		t.Errorf("Expected XML '%s', got '%s'", expectedXML, string(xmlData))
	}

	// Test XML unmarshaling
	var unmarshaled PlayInfo

	err = xml.Unmarshal(xmlData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal PlayInfo from XML: %v", err)
	}

	if unmarshaled.URL != playInfo.URL {
		t.Errorf("Expected URL '%s', got '%s'", playInfo.URL, unmarshaled.URL)
	}

	if unmarshaled.AppKey != playInfo.AppKey {
		t.Errorf("Expected AppKey '%s', got '%s'", playInfo.AppKey, unmarshaled.AppKey)
	}

	if unmarshaled.Service != playInfo.Service {
		t.Errorf("Expected Service '%s', got '%s'", playInfo.Service, unmarshaled.Service)
	}

	if unmarshaled.Volume == nil || *unmarshaled.Volume != *playInfo.Volume {
		t.Errorf("Expected Volume %v, got %v", playInfo.Volume, unmarshaled.Volume)
	}
}

func TestSpeakerResponse(t *testing.T) {
	// Test XML marshaling
	response := &SpeakerResponse{
		Value: "/speaker",
	}

	xmlData, err := xml.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal SpeakerResponse to XML: %v", err)
	}

	expectedXML := `<status>/speaker</status>`
	if string(xmlData) != expectedXML {
		t.Errorf("Expected XML '%s', got '%s'", expectedXML, string(xmlData))
	}

	// Test XML unmarshaling
	xmlInput := `<?xml version="1.0" encoding="UTF-8" ?><status>/speaker</status>`

	var unmarshaled SpeakerResponse

	err = xml.Unmarshal([]byte(xmlInput), &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal SpeakerResponse from XML: %v", err)
	}

	if unmarshaled.Value != "/speaker" {
		t.Errorf("Expected Value '/speaker', got '%s'", unmarshaled.Value)
	}
}

// Helper function to create int pointer for tests
func intPtr(i int) *int {
	return &i
}
