package models

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestClockTime_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlData  string
		expected ClockTime
	}{
		{
			name:    "Basic clock time with UTC",
			xmlData: `<clockTime zone="UTC" utc="1609459200">2021-01-01 00:00:00</clockTime>`,
			expected: ClockTime{
				Zone:  "UTC",
				UTC:   1609459200,
				Value: "2021-01-01 00:00:00",
			},
		},
		{
			name:    "Clock time with timezone",
			xmlData: `<clockTime zone="America/New_York" utc="1609459200">2021-01-01 00:00:00</clockTime>`,
			expected: ClockTime{
				Zone:  "America/New_York",
				UTC:   1609459200,
				Value: "2021-01-01 00:00:00",
			},
		},
		{
			name:    "Clock time without UTC",
			xmlData: `<clockTime zone="UTC">2021-01-01 12:30:45</clockTime>`,
			expected: ClockTime{
				Zone:  "UTC",
				Value: "2021-01-01 12:30:45",
			},
		},
		{
			name:     "Empty clock time",
			xmlData:  `<clockTime></clockTime>`,
			expected: ClockTime{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var clockTime ClockTime
			err := xml.Unmarshal([]byte(tt.xmlData), &clockTime)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if clockTime.Zone != tt.expected.Zone {
				t.Errorf("Expected Zone %q, got %q", tt.expected.Zone, clockTime.Zone)
			}
			if clockTime.UTC != tt.expected.UTC {
				t.Errorf("Expected UTC %d, got %d", tt.expected.UTC, clockTime.UTC)
			}
			if clockTime.Value != tt.expected.Value {
				t.Errorf("Expected Value %q, got %q", tt.expected.Value, clockTime.Value)
			}
		})
	}
}

func TestClockTime_GetTime(t *testing.T) {
	tests := []struct {
		name      string
		clockTime ClockTime
		wantTime  time.Time
		wantErr   bool
	}{
		{
			name: "UTC timestamp",
			clockTime: ClockTime{
				UTC: 1609459200, // 2021-01-01 00:00:00 UTC
			},
			wantTime: time.Unix(1609459200, 0),
			wantErr:  false,
		},
		{
			name: "RFC3339 format",
			clockTime: ClockTime{
				Value: "2021-01-01T12:00:00Z",
			},
			wantTime: time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name: "Standard format",
			clockTime: ClockTime{
				Value: "2021-01-01 12:00:00",
			},
			wantTime: time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name: "Time only format",
			clockTime: ClockTime{
				Value: "15:30:45",
			},
			wantTime: time.Date(0, 1, 1, 15, 30, 45, 0, time.UTC),
			wantErr:  false,
		},
		{
			name: "Invalid format",
			clockTime: ClockTime{
				Value: "invalid-time",
			},
			wantErr: true,
		},
		{
			name:      "Empty clock time",
			clockTime: ClockTime{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTime, err := tt.clockTime.GetTime()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !gotTime.Equal(tt.wantTime) {
				t.Errorf("Expected time %v, got %v", tt.wantTime, gotTime)
			}
		})
	}
}

func TestClockTime_GetUTC(t *testing.T) {
	clockTime := ClockTime{UTC: 1609459200}

	if got := clockTime.GetUTC(); got != 1609459200 {
		t.Errorf("Expected UTC %d, got %d", 1609459200, got)
	}
}

func TestClockTime_GetZone(t *testing.T) {
	clockTime := ClockTime{Zone: "America/New_York"}

	if got := clockTime.GetZone(); got != "America/New_York" {
		t.Errorf("Expected Zone %q, got %q", "America/New_York", got)
	}
}

func TestClockTime_GetTimeString(t *testing.T) {
	tests := []struct {
		name      string
		clockTime ClockTime
		expected  string
	}{
		{
			name: "Valid UTC time",
			clockTime: ClockTime{
				UTC: 1609459200, // 2021-01-01 00:00:00 UTC
			},
			expected: time.Unix(1609459200, 0).UTC().Format("2006-01-02 15:04:05"),
		},
		{
			name: "Value fallback",
			clockTime: ClockTime{
				Value: "2021-12-25 15:30:00",
			},
			expected: "2021-12-25 15:30:00",
		},
		{
			name: "Invalid time returns value",
			clockTime: ClockTime{
				Value: "invalid-time",
			},
			expected: "invalid-time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockTime.GetTimeString(); got != tt.expected {
				t.Errorf("Expected time string %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestClockTime_IsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		clockTime ClockTime
		expected  bool
	}{
		{
			name:      "Empty clock time",
			clockTime: ClockTime{},
			expected:  true,
		},
		{
			name: "Clock time with UTC",
			clockTime: ClockTime{
				UTC: 1609459200,
			},
			expected: false,
		},
		{
			name: "Clock time with value",
			clockTime: ClockTime{
				Value: "2021-01-01 00:00:00",
			},
			expected: false,
		},
		{
			name: "Clock time with both",
			clockTime: ClockTime{
				UTC:   1609459200,
				Value: "2021-01-01 00:00:00",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clockTime.IsEmpty(); got != tt.expected {
				t.Errorf("Expected IsEmpty() %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestClockTime_SetTime(t *testing.T) {
	clockTime := ClockTime{}
	testTime := time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC)

	clockTime.SetTime(testTime)

	if clockTime.UTC != testTime.Unix() {
		t.Errorf("Expected UTC %d, got %d", testTime.Unix(), clockTime.UTC)
	}
	if clockTime.Value != "2021-01-01 12:00:00" {
		t.Errorf("Expected Value %q, got %q", "2021-01-01 12:00:00", clockTime.Value)
	}
	if clockTime.Zone != "UTC" {
		t.Errorf("Expected Zone %q, got %q", "UTC", clockTime.Zone)
	}
}

func TestClockTime_SetUTC(t *testing.T) {
	clockTime := ClockTime{}
	utcTimestamp := int64(1609459200) // 2021-01-01 00:00:00 UTC

	clockTime.SetUTC(utcTimestamp)

	if clockTime.UTC != utcTimestamp {
		t.Errorf("Expected UTC %d, got %d", utcTimestamp, clockTime.UTC)
	}
	expectedValue := time.Unix(utcTimestamp, 0).UTC().Format("2006-01-02 15:04:05")
	if clockTime.Value != expectedValue {
		t.Errorf("Expected Value %q, got %q", expectedValue, clockTime.Value)
	}
}

func TestNewClockTimeRequest(t *testing.T) {
	testTime := time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC)

	request := NewClockTimeRequest(testTime)

	if request.UTC != testTime.Unix() {
		t.Errorf("Expected UTC %d, got %d", testTime.Unix(), request.UTC)
	}
	if request.Value != "2021-01-01 12:00:00" {
		t.Errorf("Expected Value %q, got %q", "2021-01-01 12:00:00", request.Value)
	}
	if request.Zone != "UTC" {
		t.Errorf("Expected Zone %q, got %q", "UTC", request.Zone)
	}
}

func TestNewClockTimeRequestUTC(t *testing.T) {
	utcTimestamp := int64(1609459200) // 2021-01-01 00:00:00 UTC

	request := NewClockTimeRequestUTC(utcTimestamp)

	if request.UTC != utcTimestamp {
		t.Errorf("Expected UTC %d, got %d", utcTimestamp, request.UTC)
	}
	expectedValue := time.Unix(utcTimestamp, 0).UTC().Format("2006-01-02 15:04:05")
	if request.Value != expectedValue {
		t.Errorf("Expected Value %q, got %q", expectedValue, request.Value)
	}
}

func TestClockTimeRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request ClockTimeRequest
		wantErr bool
	}{
		{
			name: "Valid UTC request",
			request: ClockTimeRequest{
				UTC: 1609459200,
			},
			wantErr: false,
		},
		{
			name: "Valid value request",
			request: ClockTimeRequest{
				Value: "2021-01-01 12:00:00",
			},
			wantErr: false,
		},
		{
			name: "Valid request with both",
			request: ClockTimeRequest{
				UTC:   1609459200,
				Value: "2021-01-01 12:00:00",
			},
			wantErr: false,
		},
		{
			name:    "Empty request",
			request: ClockTimeRequest{},
			wantErr: true,
		},
		{
			name: "UTC too old",
			request: ClockTimeRequest{
				UTC: 946684799, // Before year 2000
			},
			wantErr: true,
		},
		{
			name: "UTC too far in future",
			request: ClockTimeRequest{
				UTC: 4102444801, // After year 2100
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

func TestClockTimeRequest_MarshalXML(t *testing.T) {
	request := ClockTimeRequest{
		Zone:  "UTC",
		UTC:   1609459200,
		Value: "2021-01-01 00:00:00",
	}

	data, err := xml.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal XML: %v", err)
	}

	expected := `<clockTime zone="UTC" utc="1609459200">2021-01-01 00:00:00</clockTime>`
	if string(data) != expected {
		t.Errorf("Expected XML %q, got %q", expected, string(data))
	}
}
