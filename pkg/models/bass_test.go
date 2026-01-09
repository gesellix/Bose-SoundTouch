package models

import (
	"encoding/xml"
	"testing"
)

func TestNewBassRequest(t *testing.T) {
	tests := []struct {
		name      string
		level     int
		wantError bool
		wantLevel int
	}{
		{
			name:      "Valid bass level 0",
			level:     0,
			wantError: false,
			wantLevel: 0,
		},
		{
			name:      "Valid bass level +9",
			level:     9,
			wantError: false,
			wantLevel: 9,
		},
		{
			name:      "Valid bass level -9",
			level:     -9,
			wantError: false,
			wantLevel: -9,
		},
		{
			name:      "Valid bass level +3",
			level:     3,
			wantError: false,
			wantLevel: 3,
		},
		{
			name:      "Valid bass level -3",
			level:     -3,
			wantError: false,
			wantLevel: -3,
		},
		{
			name:      "Invalid bass level +10",
			level:     10,
			wantError: true,
		},
		{
			name:      "Invalid bass level -10",
			level:     -10,
			wantError: true,
		},
		{
			name:      "Invalid bass level +100",
			level:     100,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewBassRequest(tt.level)
			if tt.wantError {
				if err == nil {
					t.Errorf("NewBassRequest() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewBassRequest() unexpected error: %v", err)
				}
				if req.Level != tt.wantLevel {
					t.Errorf("NewBassRequest() level = %d, want %d", req.Level, tt.wantLevel)
				}
			}
		})
	}
}

func TestValidateBassLevel(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  bool
	}{
		{
			name:  "Valid minimum level",
			level: -9,
			want:  true,
		},
		{
			name:  "Valid maximum level",
			level: 9,
			want:  true,
		},
		{
			name:  "Valid zero level",
			level: 0,
			want:  true,
		},
		{
			name:  "Valid positive level",
			level: 5,
			want:  true,
		},
		{
			name:  "Valid negative level",
			level: -5,
			want:  true,
		},
		{
			name:  "Invalid too high",
			level: 10,
			want:  false,
		},
		{
			name:  "Invalid too low",
			level: -10,
			want:  false,
		},
		{
			name:  "Invalid way too high",
			level: 100,
			want:  false,
		},
		{
			name:  "Invalid way too low",
			level: -100,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateBassLevel(tt.level); got != tt.want {
				t.Errorf("ValidateBassLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClampBassLevel(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  int
	}{
		{
			name:  "Valid level unchanged",
			level: 0,
			want:  0,
		},
		{
			name:  "Valid positive level unchanged",
			level: 5,
			want:  5,
		},
		{
			name:  "Valid negative level unchanged",
			level: -5,
			want:  -5,
		},
		{
			name:  "Maximum level unchanged",
			level: 9,
			want:  9,
		},
		{
			name:  "Minimum level unchanged",
			level: -9,
			want:  -9,
		},
		{
			name:  "Too high clamped to max",
			level: 10,
			want:  9,
		},
		{
			name:  "Too low clamped to min",
			level: -10,
			want:  -9,
		},
		{
			name:  "Way too high clamped to max",
			level: 100,
			want:  9,
		},
		{
			name:  "Way too low clamped to min",
			level: -100,
			want:  -9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampBassLevel(tt.level); got != tt.want {
				t.Errorf("ClampBassLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBassLevelName(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  string
	}{
		{
			name:  "Very low bass",
			level: -9,
			want:  "Very Low",
		},
		{
			name:  "Low bass",
			level: -6,
			want:  "Low",
		},
		{
			name:  "Slightly low bass",
			level: -2,
			want:  "Slightly Low",
		},
		{
			name:  "Neutral bass",
			level: 0,
			want:  "Neutral",
		},
		{
			name:  "Slightly high bass",
			level: 2,
			want:  "Slightly High",
		},
		{
			name:  "High bass",
			level: 6,
			want:  "High",
		},
		{
			name:  "Very high bass",
			level: 9,
			want:  "Very High",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetBassLevelName(tt.level); got != tt.want {
				t.Errorf("GetBassLevelName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBassLevelCategory(t *testing.T) {
	tests := []struct {
		name  string
		level int
		want  string
	}{
		{
			name:  "Bass cut negative",
			level: -5,
			want:  "Bass Cut",
		},
		{
			name:  "Bass cut minimum",
			level: -9,
			want:  "Bass Cut",
		},
		{
			name:  "Flat bass",
			level: 0,
			want:  "Flat",
		},
		{
			name:  "Bass boost positive",
			level: 5,
			want:  "Bass Boost",
		},
		{
			name:  "Bass boost maximum",
			level: 9,
			want:  "Bass Boost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetBassLevelCategory(tt.level); got != tt.want {
				t.Errorf("GetBassLevelCategory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBass_GetMethods(t *testing.T) {
	bass := &Bass{
		TargetBass: 5,
		ActualBass: 3,
		DeviceID:   "1234567890AB",
	}

	if got := bass.GetLevel(); got != 5 {
		t.Errorf("GetLevel() = %v, want %v", got, 5)
	}

	if got := bass.GetActualLevel(); got != 3 {
		t.Errorf("GetActualLevel() = %v, want %v", got, 3)
	}

	if got := bass.IsAtTarget(); got != false {
		t.Errorf("IsAtTarget() = %v, want %v", got, false)
	}

	if got := bass.GetBassChangeNeeded(); got != 2 {
		t.Errorf("GetBassChangeNeeded() = %v, want %v", got, 2)
	}
}

func TestBass_BooleanMethods(t *testing.T) {
	tests := []struct {
		name         string
		bass         *Bass
		wantBoost    bool
		wantCut      bool
		wantFlat     bool
		wantAtTarget bool
	}{
		{
			name:         "Bass boost",
			bass:         &Bass{TargetBass: 5, ActualBass: 5},
			wantBoost:    true,
			wantCut:      false,
			wantFlat:     false,
			wantAtTarget: true,
		},
		{
			name:         "Bass cut",
			bass:         &Bass{TargetBass: -3, ActualBass: -3},
			wantBoost:    false,
			wantCut:      true,
			wantFlat:     false,
			wantAtTarget: true,
		},
		{
			name:         "Flat bass",
			bass:         &Bass{TargetBass: 0, ActualBass: 0},
			wantBoost:    false,
			wantCut:      false,
			wantFlat:     true,
			wantAtTarget: true,
		},
		{
			name:         "Not at target",
			bass:         &Bass{TargetBass: 5, ActualBass: 2},
			wantBoost:    true,
			wantCut:      false,
			wantFlat:     false,
			wantAtTarget: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.bass.IsBassBoost(); got != tt.wantBoost {
				t.Errorf("IsBassBoost() = %v, want %v", got, tt.wantBoost)
			}
			if got := tt.bass.IsBassCut(); got != tt.wantCut {
				t.Errorf("IsBassCut() = %v, want %v", got, tt.wantCut)
			}
			if got := tt.bass.IsFlat(); got != tt.wantFlat {
				t.Errorf("IsFlat() = %v, want %v", got, tt.wantFlat)
			}
			if got := tt.bass.IsAtTarget(); got != tt.wantAtTarget {
				t.Errorf("IsAtTarget() = %v, want %v", got, tt.wantAtTarget)
			}
		})
	}
}

func TestBass_String(t *testing.T) {
	bass := &Bass{
		TargetBass: 3,
		ActualBass: 3,
	}

	expected := "Bass: 3 (Slightly High)"
	if got := bass.String(); got != expected {
		t.Errorf("String() = %v, want %v", got, expected)
	}
}

func TestBass_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name      string
		xmlData   string
		wantError bool
		want      Bass
	}{
		{
			name: "Valid bass XML",
			xmlData: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>3</targetbass>
  <actualbass>3</actualbass>
</bass>`,
			wantError: false,
			want: Bass{
				DeviceID:   "1234567890AB",
				TargetBass: 3,
				ActualBass: 3,
			},
		},
		{
			name: "Valid negative bass XML",
			xmlData: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>-5</targetbass>
  <actualbass>-5</actualbass>
</bass>`,
			wantError: false,
			want: Bass{
				DeviceID:   "1234567890AB",
				TargetBass: -5,
				ActualBass: -5,
			},
		},
		{
			name: "Valid zero bass XML",
			xmlData: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>0</targetbass>
  <actualbass>0</actualbass>
</bass>`,
			wantError: false,
			want: Bass{
				DeviceID:   "1234567890AB",
				TargetBass: 0,
				ActualBass: 0,
			},
		},
		{
			name: "Invalid target bass too high",
			xmlData: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>15</targetbass>
  <actualbass>5</actualbass>
</bass>`,
			wantError: true,
		},
		{
			name: "Invalid actual bass too low",
			xmlData: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>5</targetbass>
  <actualbass>-15</actualbass>
</bass>`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bass Bass
			err := xml.Unmarshal([]byte(tt.xmlData), &bass)

			if tt.wantError {
				if err == nil {
					t.Errorf("UnmarshalXML() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("UnmarshalXML() unexpected error: %v", err)
				}
				if bass.DeviceID != tt.want.DeviceID {
					t.Errorf("DeviceID = %v, want %v", bass.DeviceID, tt.want.DeviceID)
				}
				if bass.TargetBass != tt.want.TargetBass {
					t.Errorf("TargetBass = %v, want %v", bass.TargetBass, tt.want.TargetBass)
				}
				if bass.ActualBass != tt.want.ActualBass {
					t.Errorf("ActualBass = %v, want %v", bass.ActualBass, tt.want.ActualBass)
				}
			}
		})
	}
}

func TestBass_MarshalXML(t *testing.T) {
	tests := []struct {
		name      string
		bass      Bass
		wantError bool
	}{
		{
			name: "Valid bass marshal",
			bass: Bass{
				DeviceID:   "1234567890AB",
				TargetBass: 3,
				ActualBass: 3,
			},
			wantError: false,
		},
		{
			name: "Valid negative bass marshal",
			bass: Bass{
				DeviceID:   "1234567890AB",
				TargetBass: -5,
				ActualBass: -5,
			},
			wantError: false,
		},
		{
			name: "Valid bass marshal with high values",
			bass: Bass{
				DeviceID:   "1234567890AB",
				TargetBass: 9,
				ActualBass: 8,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := xml.Marshal(tt.bass)

			if tt.wantError {
				if err == nil {
					t.Errorf("MarshalXML() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("MarshalXML() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBassRequest_MarshalXML(t *testing.T) {
	req := &BassRequest{
		Level: 5,
	}

	data, err := xml.Marshal(req)
	if err != nil {
		t.Errorf("MarshalXML() unexpected error: %v", err)
	}

	expected := "<bass>5</bass>"
	if string(data) != expected {
		t.Errorf("MarshalXML() = %v, want %v", string(data), expected)
	}
}

func TestBassConstants(t *testing.T) {
	if BassLevelMin != -9 {
		t.Errorf("BassLevelMin = %v, want %v", BassLevelMin, -9)
	}
	if BassLevelMax != 9 {
		t.Errorf("BassLevelMax = %v, want %v", BassLevelMax, 9)
	}
	if BassLevelDefault != 0 {
		t.Errorf("BassLevelDefault = %v, want %v", BassLevelDefault, 0)
	}
}

func TestBassLevelEdgeCases(t *testing.T) {
	// Test boundary values
	t.Run("Minimum boundary", func(t *testing.T) {
		if !ValidateBassLevel(-9) {
			t.Error("ValidateBassLevel(-9) should be true")
		}
		if ValidateBassLevel(-10) {
			t.Error("ValidateBassLevel(-10) should be false")
		}
	})

	t.Run("Maximum boundary", func(t *testing.T) {
		if !ValidateBassLevel(9) {
			t.Error("ValidateBassLevel(9) should be true")
		}
		if ValidateBassLevel(10) {
			t.Error("ValidateBassLevel(10) should be false")
		}
	})

	t.Run("Zero boundary", func(t *testing.T) {
		if !ValidateBassLevel(0) {
			t.Error("ValidateBassLevel(0) should be true")
		}
	})
}
