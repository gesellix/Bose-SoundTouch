package models

import (
	"encoding/xml"
	"fmt"
)

// Balance represents the response from /balance endpoint
type Balance struct {
	XMLName       xml.Name `xml:"balance"`
	DeviceID      string   `xml:"deviceID,attr"`
	TargetBalance int      `xml:"targetbalance"`
	ActualBalance int      `xml:"actualbalance"`
}

// BalanceRequest represents the request for POST /balance endpoint
type BalanceRequest struct {
	XMLName xml.Name `xml:"balance"`
	Level   int      `xml:",chardata"`
}

// Balance level constants
const (
	BalanceLevelMin     = -50
	BalanceLevelMax     = 50
	BalanceLevelDefault = 0
)

// NewBalanceRequest creates a new balance request with validation
func NewBalanceRequest(level int) (*BalanceRequest, error) {
	if !ValidateBalanceLevel(level) {
		return nil, fmt.Errorf("invalid balance level: %d (must be between %d and %d)", level, BalanceLevelMin, BalanceLevelMax)
	}

	return &BalanceRequest{
		Level: level,
	}, nil
}

// ValidateBalanceLevel validates that a balance level is within the allowed range
func ValidateBalanceLevel(level int) bool {
	return level >= BalanceLevelMin && level <= BalanceLevelMax
}

// ClampBalanceLevel clamps a balance level to the valid range
func ClampBalanceLevel(level int) int {
	if level < BalanceLevelMin {
		return BalanceLevelMin
	}
	if level > BalanceLevelMax {
		return BalanceLevelMax
	}
	return level
}

// GetLevel returns the target balance level
func (b *Balance) GetLevel() int {
	return b.TargetBalance
}

// GetActualLevel returns the actual balance level
func (b *Balance) GetActualLevel() int {
	return b.ActualBalance
}

// IsAtTarget returns true if actual balance matches target balance
func (b *Balance) IsAtTarget() bool {
	return b.TargetBalance == b.ActualBalance
}

// GetBalanceLevelName returns a descriptive name for the balance level
func GetBalanceLevelName(level int) string {
	switch {
	case level < -30:
		return "Far Left"
	case level < -10:
		return "Left"
	case level < 0:
		return "Slightly Left"
	case level == 0:
		return "Center"
	case level <= 10:
		return "Slightly Right"
	case level <= 30:
		return "Right"
	default:
		return "Far Right"
	}
}

// GetBalanceLevelCategory returns the balance category
func GetBalanceLevelCategory(level int) string {
	switch {
	case level < 0:
		return "Left Channel"
	case level == 0:
		return "Balanced"
	default:
		return "Right Channel"
	}
}

// String returns a human-readable string representation
func (b *Balance) String() string {
	return fmt.Sprintf("Balance: %d (%s)", b.GetLevel(), GetBalanceLevelName(b.GetLevel()))
}

// UnmarshalXML implements custom XML unmarshaling with validation
func (b *Balance) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	// Use a temporary struct to avoid infinite recursion
	type TempBalance Balance
	temp := (*TempBalance)(b)

	if err := d.DecodeElement(temp, &start); err != nil {
		return err
	}

	// Validate balance levels are within acceptable range
	if !ValidateBalanceLevel(b.TargetBalance) {
		return fmt.Errorf("invalid target balance level: %d", b.TargetBalance)
	}

	if !ValidateBalanceLevel(b.ActualBalance) {
		return fmt.Errorf("invalid actual balance level: %d", b.ActualBalance)
	}

	return nil
}

// MarshalXML implements custom XML marshaling
func (b *Balance) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	type TempBalance Balance
	temp := (*TempBalance)(b)
	return e.EncodeElement(temp, start)
}

// IsLeftBalance returns true if balance favors left channel (negative level)
func (b *Balance) IsLeftBalance() bool {
	return b.GetLevel() < 0
}

// IsRightBalance returns true if balance favors right channel (positive level)
func (b *Balance) IsRightBalance() bool {
	return b.GetLevel() > 0
}

// IsBalanced returns true if balance is centered (zero level)
func (b *Balance) IsBalanced() bool {
	return b.GetLevel() == 0
}

// GetBalanceChangeNeeded returns the amount of change needed to reach target from actual
func (b *Balance) GetBalanceChangeNeeded() int {
	return b.TargetBalance - b.ActualBalance
}

// GetLeftRightPercentage returns the balance as left/right percentages
func (b *Balance) GetLeftRightPercentage() (left, right int) {
	level := b.GetLevel()
	if level <= 0 {
		// Left emphasis or center
		left = 50 + (-level / 2)
		right = 50 - (-level / 2)
	} else {
		// Right emphasis
		left = 50 - (level / 2)
		right = 50 + (level / 2)
	}
	return left, right
}
