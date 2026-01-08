package models

import (
	"encoding/xml"
	"fmt"
	"testing"
)

func TestNewVolumeRequest(t *testing.T) {
	tests := []struct {
		name   string
		volume int
		want   int
	}{
		{
			name:   "zero volume",
			volume: 0,
			want:   0,
		},
		{
			name:   "medium volume",
			volume: 50,
			want:   50,
		},
		{
			name:   "max volume",
			volume: 100,
			want:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := NewVolumeRequest(tt.volume)
			if req.Value != tt.want {
				t.Errorf("NewVolumeRequest() value = %d, want %d", req.Value, tt.want)
			}
		})
	}
}

func TestVolumeRequestXMLMarshal(t *testing.T) {
	tests := []struct {
		name        string
		volume      int
		expectedXML string
	}{
		{
			name:        "zero volume",
			volume:      0,
			expectedXML: `<volume>0</volume>`,
		},
		{
			name:        "medium volume",
			volume:      50,
			expectedXML: `<volume>50</volume>`,
		},
		{
			name:        "max volume",
			volume:      100,
			expectedXML: `<volume>100</volume>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := NewVolumeRequest(tt.volume)
			xmlData, err := xml.Marshal(req)
			if err != nil {
				t.Fatalf("Failed to marshal XML: %v", err)
			}

			if string(xmlData) != tt.expectedXML {
				t.Errorf("Expected XML %q, got %q", tt.expectedXML, string(xmlData))
			}
		})
	}
}

func TestVolumeXMLUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		xmlData string
		want    Volume
	}{
		{
			name:    "normal volume response",
			xmlData: `<volume deviceID="12345"><targetvolume>50</targetvolume><actualvolume>50</actualvolume><muteenabled>false</muteenabled></volume>`,
			want: Volume{
				DeviceID:     "12345",
				TargetVolume: 50,
				ActualVolume: 50,
				MuteEnabled:  false,
			},
		},
		{
			name:    "muted volume response",
			xmlData: `<volume deviceID="67890"><targetvolume>0</targetvolume><actualvolume>0</actualvolume><muteenabled>true</muteenabled></volume>`,
			want: Volume{
				DeviceID:     "67890",
				TargetVolume: 0,
				ActualVolume: 0,
				MuteEnabled:  true,
			},
		},
		{
			name:    "volume adjusting",
			xmlData: `<volume deviceID="54321"><targetvolume>75</targetvolume><actualvolume>70</actualvolume><muteenabled>false</muteenabled></volume>`,
			want: Volume{
				DeviceID:     "54321",
				TargetVolume: 75,
				ActualVolume: 70,
				MuteEnabled:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var volume Volume
			err := xml.Unmarshal([]byte(tt.xmlData), &volume)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if volume.DeviceID != tt.want.DeviceID {
				t.Errorf("DeviceID = %q, want %q", volume.DeviceID, tt.want.DeviceID)
			}
			if volume.TargetVolume != tt.want.TargetVolume {
				t.Errorf("TargetVolume = %d, want %d", volume.TargetVolume, tt.want.TargetVolume)
			}
			if volume.ActualVolume != tt.want.ActualVolume {
				t.Errorf("ActualVolume = %d, want %d", volume.ActualVolume, tt.want.ActualVolume)
			}
			if volume.MuteEnabled != tt.want.MuteEnabled {
				t.Errorf("MuteEnabled = %v, want %v", volume.MuteEnabled, tt.want.MuteEnabled)
			}
		})
	}
}

func TestVolumeGetLevel(t *testing.T) {
	volume := Volume{ActualVolume: 75}
	if got := volume.GetLevel(); got != 75 {
		t.Errorf("GetLevel() = %d, want 75", got)
	}
}

func TestVolumeGetTargetLevel(t *testing.T) {
	volume := Volume{TargetVolume: 60}
	if got := volume.GetTargetLevel(); got != 60 {
		t.Errorf("GetTargetLevel() = %d, want 60", got)
	}
}

func TestVolumeIsMuted(t *testing.T) {
	tests := []struct {
		name        string
		muteEnabled bool
		want        bool
	}{
		{
			name:        "muted",
			muteEnabled: true,
			want:        true,
		},
		{
			name:        "not muted",
			muteEnabled: false,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volume := Volume{MuteEnabled: tt.muteEnabled}
			if got := volume.IsMuted(); got != tt.want {
				t.Errorf("IsMuted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeIsVolumeSync(t *testing.T) {
	tests := []struct {
		name         string
		targetVolume int
		actualVolume int
		want         bool
	}{
		{
			name:         "synchronized",
			targetVolume: 50,
			actualVolume: 50,
			want:         true,
		},
		{
			name:         "not synchronized",
			targetVolume: 75,
			actualVolume: 70,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volume := Volume{
				TargetVolume: tt.targetVolume,
				ActualVolume: tt.actualVolume,
			}
			if got := volume.IsVolumeSync(); got != tt.want {
				t.Errorf("IsVolumeSync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVolumeGetVolumeString(t *testing.T) {
	tests := []struct {
		name        string
		volume      Volume
		expectedStr string
	}{
		{
			name:        "muted",
			volume:      Volume{ActualVolume: 0, MuteEnabled: true},
			expectedStr: "Muted",
		},
		{
			name:        "unmuted with volume",
			volume:      Volume{ActualVolume: 75, MuteEnabled: false},
			expectedStr: "75",
		},
		{
			name:        "zero volume but not muted",
			volume:      Volume{ActualVolume: 0, MuteEnabled: false},
			expectedStr: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.volume.GetVolumeString(); got != tt.expectedStr {
				t.Errorf("GetVolumeString() = %q, want %q", got, tt.expectedStr)
			}
		})
	}
}

func TestValidateVolumeLevel(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  bool
	}{
		{
			name:  "valid min",
			level: 0,
			want:  true,
		},
		{
			name:  "valid max",
			level: 100,
			want:  true,
		},
		{
			name:  "valid middle",
			level: 50,
			want:  true,
		},
		{
			name:  "invalid negative",
			level: -1,
			want:  false,
		},
		{
			name:  "invalid too high",
			level: 101,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateVolumeLevel(tt.level); got != tt.want {
				t.Errorf("ValidateVolumeLevel(%d) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestClampVolumeLevel(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  int
	}{
		{
			name:  "within range",
			level: 50,
			want:  50,
		},
		{
			name:  "below min",
			level: -10,
			want:  0,
		},
		{
			name:  "above max",
			level: 150,
			want:  100,
		},
		{
			name:  "at min boundary",
			level: 0,
			want:  0,
		},
		{
			name:  "at max boundary",
			level: 100,
			want:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampVolumeLevel(tt.level); got != tt.want {
				t.Errorf("ClampVolumeLevel(%d) = %d, want %d", tt.level, got, tt.want)
			}
		})
	}
}

func TestGetVolumeLevelName(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  string
	}{
		{
			name:  "mute",
			level: 0,
			want:  "Mute",
		},
		{
			name:  "very quiet",
			level: 5,
			want:  "Very Quiet",
		},
		{
			name:  "quiet boundary",
			level: 10,
			want:  "Very Quiet",
		},
		{
			name:  "quiet",
			level: 20,
			want:  "Quiet",
		},
		{
			name:  "quiet boundary",
			level: 25,
			want:  "Quiet",
		},
		{
			name:  "medium",
			level: 40,
			want:  "Medium",
		},
		{
			name:  "medium boundary",
			level: 50,
			want:  "Medium",
		},
		{
			name:  "high",
			level: 65,
			want:  "High",
		},
		{
			name:  "high boundary",
			level: 75,
			want:  "High",
		},
		{
			name:  "loud",
			level: 90,
			want:  "Loud",
		},
		{
			name:  "max loud",
			level: 100,
			want:  "Loud",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetVolumeLevelName(tt.level); got != tt.want {
				t.Errorf("GetVolumeLevelName(%d) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestVolumeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"VolumeMin", VolumeMin, 0},
		{"VolumeMax", VolumeMax, 100},
		{"VolumeMute", VolumeMute, 0},
		{"VolumeQuiet", VolumeQuiet, 10},
		{"VolumeLow", VolumeLow, 25},
		{"VolumeMedium", VolumeMedium, 50},
		{"VolumeHigh", VolumeHigh, 75},
		{"VolumeLoud", VolumeLoud, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkNewVolumeRequest(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewVolumeRequest(50)
	}
}

func BenchmarkVolumeXMLMarshal(b *testing.B) {
	req := NewVolumeRequest(50)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		xml.Marshal(req)
	}
}

func BenchmarkValidateVolumeLevel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidateVolumeLevel(50)
	}
}

func BenchmarkClampVolumeLevel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ClampVolumeLevel(150)
	}
}

func BenchmarkGetVolumeLevelName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetVolumeLevelName(50)
	}
}

// Example tests
func ExampleValidateVolumeLevel() {
	valid := ValidateVolumeLevel(50)
	invalid := ValidateVolumeLevel(150)

	fmt.Printf("Volume 50 is valid: %v\n", valid)
	fmt.Printf("Volume 150 is valid: %v\n", invalid)

	// Output:
	// Volume 50 is valid: true
	// Volume 150 is valid: false
}

func ExampleClampVolumeLevel() {
	clamped1 := ClampVolumeLevel(150)
	clamped2 := ClampVolumeLevel(-10)
	clamped3 := ClampVolumeLevel(50)

	fmt.Printf("150 clamped: %d\n", clamped1)
	fmt.Printf("-10 clamped: %d\n", clamped2)
	fmt.Printf("50 clamped: %d\n", clamped3)

	// Output:
	// 150 clamped: 100
	// -10 clamped: 0
	// 50 clamped: 50
}

func ExampleGetVolumeLevelName() {
	fmt.Printf("Volume 0: %s\n", GetVolumeLevelName(0))
	fmt.Printf("Volume 25: %s\n", GetVolumeLevelName(25))
	fmt.Printf("Volume 50: %s\n", GetVolumeLevelName(50))
	fmt.Printf("Volume 100: %s\n", GetVolumeLevelName(100))

	// Output:
	// Volume 0: Mute
	// Volume 25: Quiet
	// Volume 50: Medium
	// Volume 100: Loud
}
