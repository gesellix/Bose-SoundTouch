package models

import (
	"encoding/xml"
	"fmt"
)

// Bass represents the response from /bass endpoint
type Bass struct {
	XMLName    xml.Name `xml:"bass"`
	DeviceID   string   `xml:"deviceID,attr"`
	TargetBass int      `xml:"targetbass"`
	ActualBass int      `xml:"actualbass"`
}

// BassRequest represents the request for POST /bass endpoint
type BassRequest struct {
	XMLName xml.Name `xml:"bass"`
	Level   int      `xml:",chardata"`
}

// Bass level constants
const (
	BassLevelMin     = -9
	BassLevelMax     = 9
	BassLevelDefault = 0
)

// NewBassRequest creates a new bass request with validation
func NewBassRequest(level int) (*BassRequest, error) {
	if !ValidateBassLevel(level) {
		return nil, fmt.Errorf("invalid bass level: %d (must be between %d and %d)", level, BassLevelMin, BassLevelMax)
	}

	return &BassRequest{
		Level: level,
	}, nil
}

// ValidateBassLevel validates that a bass level is within the allowed range
func ValidateBassLevel(level int) bool {
	return level >= BassLevelMin && level <= BassLevelMax
}

// ClampBassLevel clamps a bass level to the valid range
func ClampBassLevel(level int) int {
	if level < BassLevelMin {
		return BassLevelMin
	}
	if level > BassLevelMax {
		return BassLevelMax
	}
	return level
}

// GetLevel returns the target bass level
func (b *Bass) GetLevel() int {
	return b.TargetBass
}

// GetActualLevel returns the actual bass level
func (b *Bass) GetActualLevel() int {
	return b.ActualBass
}

// IsAtTarget returns true if actual bass matches target bass
func (b *Bass) IsAtTarget() bool {
	return b.TargetBass == b.ActualBass
}

// GetLevelName returns a descriptive name for the bass level
func GetBassLevelName(level int) string {
	switch {
	case level < -6:
		return "Very Low"
	case level < -3:
		return "Low"
	case level < 0:
		return "Slightly Low"
	case level == 0:
		return "Neutral"
	case level <= 3:
		return "Slightly High"
	case level <= 6:
		return "High"
	default:
		return "Very High"
	}
}

// GetBassLevelCategory returns the bass category
func GetBassLevelCategory(level int) string {
	switch {
	case level < 0:
		return "Bass Cut"
	case level == 0:
		return "Flat"
	default:
		return "Bass Boost"
	}
}

// String returns a human-readable string representation
func (b *Bass) String() string {
	return fmt.Sprintf("Bass: %d (%s)", b.GetLevel(), GetBassLevelName(b.GetLevel()))
}

// UnmarshalXML implements custom XML unmarshaling with validation
func (b *Bass) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// Use a temporary struct to avoid infinite recursion
	type TempBass Bass
	temp := (*TempBass)(b)

	if err := d.DecodeElement(temp, &start); err != nil {
		return err
	}

	// Validate bass levels are within acceptable range
	if !ValidateBassLevel(b.TargetBass) {
		return fmt.Errorf("invalid target bass level: %d", b.TargetBass)
	}

	if !ValidateBassLevel(b.ActualBass) {
		return fmt.Errorf("invalid actual bass level: %d", b.ActualBass)
	}

	return nil
}

// MarshalXML implements custom XML marshaling
func (b *Bass) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	type TempBass Bass
	temp := (*TempBass)(b)
	return e.EncodeElement(temp, start)
}

// IsBassBoost returns true if bass is boosted (positive level)
func (b *Bass) IsBassBoost() bool {
	return b.GetLevel() > 0
}

// IsBassCut returns true if bass is cut (negative level)
func (b *Bass) IsBassCut() bool {
	return b.GetLevel() < 0
}

// IsFlat returns true if bass is neutral (zero level)
func (b *Bass) IsFlat() bool {
	return b.GetLevel() == 0
}

// GetBassChangeNeeded returns the amount of change needed to reach target from actual
func (b *Bass) GetBassChangeNeeded() int {
	return b.TargetBass - b.ActualBass
}
