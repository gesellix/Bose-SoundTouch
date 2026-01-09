package models

import (
	"encoding/xml"
	"testing"
)

func TestClockDisplay_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlData  string
		expected ClockDisplay
	}{
		{
			name:    "Full clock display configuration",
			xmlData: `<clockDisplay deviceID="A81B6A536A98" enabled="true" format="24" brightness="75" autoDim="true" timeZone="America/New_York">Clock Display</clockDisplay>`,
			expected: ClockDisplay{
				DeviceID:   "A81B6A536A98",
				Enabled:    true,
				Format:     "24",
				Brightness: 75,
				AutoDim:    true,
				TimeZone:   "America/New_York",
				Value:      "Clock Display",
			},
		},
		{
			name:    "Minimal configuration",
			xmlData: `<clockDisplay enabled="false" format="12"></clockDisplay>`,
			expected: ClockDisplay{
				Enabled: false,
				Format:  "12",
			},
		},
		{
			name:    "12-hour format",
			xmlData: `<clockDisplay enabled="true" format="12" brightness="50"></clockDisplay>`,
			expected: ClockDisplay{
				Enabled:    true,
				Format:     "12",
				Brightness: 50,
			},
		},
		{
			name:    "Auto format",
			xmlData: `<clockDisplay enabled="true" format="auto" brightness="25" autoDim="false"></clockDisplay>`,
			expected: ClockDisplay{
				Enabled:    true,
				Format:     "auto",
				Brightness: 25,
				AutoDim:    false,
			},
		},
		{
			name:     "Empty clock display",
			xmlData:  `<clockDisplay></clockDisplay>`,
			expected: ClockDisplay{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clockDisplay ClockDisplay
			err := xml.Unmarshal([]byte(tt.xmlData), &clockDisplay)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if clockDisplay.DeviceID != tt.expected.DeviceID {
				t.Errorf("Expected DeviceID %q, got %q", tt.expected.DeviceID, clockDisplay.DeviceID)
			}
			if clockDisplay.Enabled != tt.expected.Enabled {
				t.Errorf("Expected Enabled %v, got %v", tt.expected.Enabled, clockDisplay.Enabled)
			}
			if clockDisplay.Format != tt.expected.Format {
				t.Errorf("Expected Format %q, got %q", tt.expected.Format, clockDisplay.Format)
			}
			if clockDisplay.Brightness != tt.expected.Brightness {
				t.Errorf("Expected Brightness %d, got %d", tt.expected.Brightness, clockDisplay.Brightness)
			}
			if clockDisplay.AutoDim != tt.expected.AutoDim {
				t.Errorf("Expected AutoDim %v, got %v", tt.expected.AutoDim, clockDisplay.AutoDim)
			}
			if clockDisplay.TimeZone != tt.expected.TimeZone {
				t.Errorf("Expected TimeZone %q, got %q", tt.expected.TimeZone, clockDisplay.TimeZone)
			}
			if clockDisplay.Value != tt.expected.Value {
				t.Errorf("Expected Value %q, got %q", tt.expected.Value, clockDisplay.Value)
			}
		})
	}
}

func TestClockDisplay_IsEnabled(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     bool
	}{
		{
			name:         "Enabled clock display",
			clockDisplay: ClockDisplay{Enabled: true},
			expected:     true,
		},
		{
			name:         "Disabled clock display",
			clockDisplay: ClockDisplay{Enabled: false},
			expected:     false,
		},
		{
			name:         "Default (false) clock display",
			clockDisplay: ClockDisplay{},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.IsEnabled(); got != tt.expected {
				t.Errorf("Expected IsEnabled() %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestClockDisplay_GetFormat(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     string
	}{
		{
			name:         "12-hour format",
			clockDisplay: ClockDisplay{Format: "12"},
			expected:     "12",
		},
		{
			name:         "24-hour format",
			clockDisplay: ClockDisplay{Format: "24"},
			expected:     "24",
		},
		{
			name:         "Auto format",
			clockDisplay: ClockDisplay{Format: "auto"},
			expected:     "auto",
		},
		{
			name:         "Empty format (default)",
			clockDisplay: ClockDisplay{},
			expected:     "12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.GetFormat(); got != tt.expected {
				t.Errorf("Expected GetFormat() %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestClockDisplay_GetFormatDescription(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     string
	}{
		{
			name:         "12-hour format",
			clockDisplay: ClockDisplay{Format: "12"},
			expected:     "12-hour format (AM/PM)",
		},
		{
			name:         "24-hour format",
			clockDisplay: ClockDisplay{Format: "24"},
			expected:     "24-hour format",
		},
		{
			name:         "Auto format",
			clockDisplay: ClockDisplay{Format: "auto"},
			expected:     "Auto format (system default)",
		},
		{
			name:         "Unknown format",
			clockDisplay: ClockDisplay{Format: "unknown"},
			expected:     "12-hour format (AM/PM)",
		},
		{
			name:         "Empty format",
			clockDisplay: ClockDisplay{},
			expected:     "12-hour format (AM/PM)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.GetFormatDescription(); got != tt.expected {
				t.Errorf("Expected GetFormatDescription() %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestClockDisplay_GetBrightness(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     int
	}{
		{
			name:         "Normal brightness",
			clockDisplay: ClockDisplay{Brightness: 75},
			expected:     75,
		},
		{
			name:         "Minimum brightness",
			clockDisplay: ClockDisplay{Brightness: 0},
			expected:     0,
		},
		{
			name:         "Maximum brightness",
			clockDisplay: ClockDisplay{Brightness: 100},
			expected:     100,
		},
		{
			name:         "Below minimum brightness (clamped)",
			clockDisplay: ClockDisplay{Brightness: -10},
			expected:     0,
		},
		{
			name:         "Above maximum brightness (clamped)",
			clockDisplay: ClockDisplay{Brightness: 150},
			expected:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.GetBrightness(); got != tt.expected {
				t.Errorf("Expected GetBrightness() %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestClockDisplay_GetBrightnessLevel(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     string
	}{
		{
			name:         "Off brightness",
			clockDisplay: ClockDisplay{Brightness: 0},
			expected:     "Off",
		},
		{
			name:         "Low brightness",
			clockDisplay: ClockDisplay{Brightness: 20},
			expected:     "Low",
		},
		{
			name:         "Medium brightness",
			clockDisplay: ClockDisplay{Brightness: 50},
			expected:     "Medium",
		},
		{
			name:         "High brightness",
			clockDisplay: ClockDisplay{Brightness: 75},
			expected:     "High",
		},
		{
			name:         "Maximum brightness",
			clockDisplay: ClockDisplay{Brightness: 100},
			expected:     "Maximum",
		},
		{
			name:         "Edge case - 25 (Low boundary)",
			clockDisplay: ClockDisplay{Brightness: 25},
			expected:     "Low",
		},
		{
			name:         "Edge case - 26 (Medium boundary)",
			clockDisplay: ClockDisplay{Brightness: 26},
			expected:     "Medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.GetBrightnessLevel(); got != tt.expected {
				t.Errorf("Expected GetBrightnessLevel() %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestClockDisplay_IsAutoDimEnabled(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     bool
	}{
		{
			name:         "Auto-dim enabled",
			clockDisplay: ClockDisplay{AutoDim: true},
			expected:     true,
		},
		{
			name:         "Auto-dim disabled",
			clockDisplay: ClockDisplay{AutoDim: false},
			expected:     false,
		},
		{
			name:         "Default auto-dim",
			clockDisplay: ClockDisplay{},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.IsAutoDimEnabled(); got != tt.expected {
				t.Errorf("Expected IsAutoDimEnabled() %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestClockDisplay_IsEmpty(t *testing.T) {
	tests := []struct {
		name         string
		clockDisplay ClockDisplay
		expected     bool
	}{
		{
			name:         "Completely empty",
			clockDisplay: ClockDisplay{},
			expected:     true,
		},
		{
			name:         "Empty but disabled",
			clockDisplay: ClockDisplay{Enabled: false},
			expected:     true,
		},
		{
			name:         "Has enabled flag",
			clockDisplay: ClockDisplay{Enabled: true},
			expected:     false,
		},
		{
			name:         "Has format",
			clockDisplay: ClockDisplay{Format: "24"},
			expected:     false,
		},
		{
			name:         "Has brightness",
			clockDisplay: ClockDisplay{Brightness: 50},
			expected:     false,
		},
		{
			name:         "Has timezone",
			clockDisplay: ClockDisplay{TimeZone: "UTC"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockDisplay.IsEmpty(); got != tt.expected {
				t.Errorf("Expected IsEmpty() %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestNewClockDisplayRequest(t *testing.T) {
	request := NewClockDisplayRequest()

	if request == nil {
		t.Error("Expected non-nil request")
	}

	if request.Enabled != nil {
		t.Error("Expected Enabled to be nil")
	}
	if request.Format != "" {
		t.Error("Expected Format to be empty")
	}
	if request.Brightness != nil {
		t.Error("Expected Brightness to be nil")
	}
	if request.AutoDim != nil {
		t.Error("Expected AutoDim to be nil")
	}
	if request.TimeZone != "" {
		t.Error("Expected TimeZone to be empty")
	}
}

func TestClockDisplayRequest_SetMethods(t *testing.T) {
	request := NewClockDisplayRequest()

	// Test chaining
	result := request.SetEnabled(true).
		SetFormat(ClockFormat24Hour).
		SetBrightness(75).
		SetAutoDim(true).
		SetTimeZone("UTC")

	if result != request {
		t.Error("Expected method chaining to return same instance")
	}

	if request.Enabled == nil || *request.Enabled != true {
		t.Error("Expected Enabled to be true")
	}
	if request.Format != "24" {
		t.Errorf("Expected Format to be '24', got %q", request.Format)
	}
	if request.Brightness == nil || *request.Brightness != 75 {
		t.Error("Expected Brightness to be 75")
	}
	if request.AutoDim == nil || *request.AutoDim != true {
		t.Error("Expected AutoDim to be true")
	}
	if request.TimeZone != "UTC" {
		t.Errorf("Expected TimeZone to be 'UTC', got %q", request.TimeZone)
	}
}

func TestClockDisplayRequest_SetBrightness_Clamping(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "Normal value",
			input:    50,
			expected: 50,
		},
		{
			name:     "Below minimum",
			input:    -10,
			expected: 0,
		},
		{
			name:     "Above maximum",
			input:    150,
			expected: 100,
		},
		{
			name:     "Minimum boundary",
			input:    0,
			expected: 0,
		},
		{
			name:     "Maximum boundary",
			input:    100,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := NewClockDisplayRequest()
			request.SetBrightness(tt.input)

			if request.Brightness == nil {
				t.Error("Expected Brightness to be set")
				return
			}

			if *request.Brightness != tt.expected {
				t.Errorf("Expected brightness %d, got %d", tt.expected, *request.Brightness)
			}
		})
	}
}

func TestClockDisplayRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request ClockDisplayRequest
		wantErr bool
	}{
		{
			name:    "Valid empty request",
			request: ClockDisplayRequest{},
			wantErr: false,
		},
		{
			name: "Valid 12-hour format",
			request: ClockDisplayRequest{
				Format: "12",
			},
			wantErr: false,
		},
		{
			name: "Valid 24-hour format",
			request: ClockDisplayRequest{
				Format: "24",
			},
			wantErr: false,
		},
		{
			name: "Valid auto format",
			request: ClockDisplayRequest{
				Format: "auto",
			},
			wantErr: false,
		},
		{
			name: "Invalid format",
			request: ClockDisplayRequest{
				Format: "invalid",
			},
			wantErr: true,
		},
		{
			name: "Valid brightness minimum",
			request: ClockDisplayRequest{
				Brightness: &[]int{0}[0],
			},
			wantErr: false,
		},
		{
			name: "Valid brightness maximum",
			request: ClockDisplayRequest{
				Brightness: &[]int{100}[0],
			},
			wantErr: false,
		},
		{
			name: "Invalid brightness below minimum",
			request: ClockDisplayRequest{
				Brightness: &[]int{-1}[0],
			},
			wantErr: true,
		},
		{
			name: "Invalid brightness above maximum",
			request: ClockDisplayRequest{
				Brightness: &[]int{101}[0],
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if tt.wantErr && err == nil {
				t.Error("Expected error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClockDisplayRequest_HasChanges(t *testing.T) {
	tests := []struct {
		name     string
		request  ClockDisplayRequest
		expected bool
	}{
		{
			name:     "Empty request",
			request:  ClockDisplayRequest{},
			expected: false,
		},
		{
			name: "Has enabled change",
			request: ClockDisplayRequest{
				Enabled: &[]bool{true}[0],
			},
			expected: true,
		},
		{
			name: "Has format change",
			request: ClockDisplayRequest{
				Format: "24",
			},
			expected: true,
		},
		{
			name: "Has brightness change",
			request: ClockDisplayRequest{
				Brightness: &[]int{50}[0],
			},
			expected: true,
		},
		{
			name: "Has auto-dim change",
			request: ClockDisplayRequest{
				AutoDim: &[]bool{true}[0],
			},
			expected: true,
		},
		{
			name: "Has timezone change",
			request: ClockDisplayRequest{
				TimeZone: "UTC",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.request.HasChanges(); got != tt.expected {
				t.Errorf("Expected HasChanges() %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestClockDisplayRequest_MarshalXML(t *testing.T) {
	request := ClockDisplayRequest{
		Enabled:    &[]bool{true}[0],
		Format:     "24",
		Brightness: &[]int{75}[0],
		AutoDim:    &[]bool{false}[0],
		TimeZone:   "America/New_York",
	}

	data, err := xml.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal XML: %v", err)
	}

	expected := `<clockDisplay enabled="true" format="24" brightness="75" autoDim="false" timeZone="America/New_York"></clockDisplay>`
	if string(data) != expected {
		t.Errorf("Expected XML %q, got %q", expected, string(data))
	}
}
