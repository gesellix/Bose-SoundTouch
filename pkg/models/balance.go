// Package models provides data structures and types for the Bose SoundTouch API.
package models

import (
	"encoding/xml"
	"fmt"
)

// Balance represents the response from GET /balance.
//
// Balance is a property of a stereo PAIR (a /getGroup group of two SoundTouch
// 10s taking the LEFT and RIGHT channel), not of a multiroom zone.
//
// It belongs to the pair, not to one speaker: measured on hardware, BOTH
// members report Available true and the same value, and a write to either is
// reflected by both within a second. Addressing the master is still the
// conventional choice (it is what the app Bose ships on the speaker does), but
// it is not a requirement. Third-party notes claiming the right-hand member
// reports Available false do not match FW 27.0.6.
//
// An unpaired speaker reports Available false.
//
// Shape confirmed on hardware (SoundTouch 10, variant=rhino, moduleType=sm2,
// FW 27.0.6, paired as master, 2026-09-08):
//
//	<balance deviceID="DEVICEID01">
//	  <balanceAvailable>true</balanceAvailable>
//	  <balanceMin>-7</balanceMin>
//	  <balanceMax>7</balanceMax>
//	  <balanceDefault>0</balanceDefault>
//	  <targetBalance>0</targetBalance>
//	  <actualBalance>0</actualBalance>
//	</balance>
//
// The element names are camelCase and the range is -7..7. An earlier version of
// this type assumed lowercase <targetbalance>/<actualbalance> and -50..50,
// copied from the /bass implementation with the range widened; none of it was
// ever backed by a capture (GH-699).
//
// Bounds come from the device. Do not hardcode them: Min/Max/Default are what
// the speaker reports, and validation should go through Clamp or Validate on
// the fetched value rather than a package-level constant.
type Balance struct {
	XMLName   xml.Name `xml:"balance"`
	DeviceID  string   `xml:"deviceID,attr"`
	Available bool     `xml:"balanceAvailable"`
	Min       int      `xml:"balanceMin"`
	Max       int      `xml:"balanceMax"`
	Default   int      `xml:"balanceDefault"`
	Target    int      `xml:"targetBalance"`
	Actual    int      `xml:"actualBalance"`
}

// BalanceRequest is the body of a balance write.
//
// Note this is NOT sent over HTTP. POST /balance did not work in field tests —
// the write hung rather than being refused — and the app Bose ships on the
// speaker itself only ever writes balance over the WebSocket. Use
// WebSocketClient.SetBalance, which wraps this in the <msg> envelope with
// mainNode="balanceSet". The type stays here because the body it produces is
// the same either way.
type BalanceRequest struct {
	XMLName xml.Name `xml:"balance"`
	Target  int      `xml:"targetBalance"`
}

// NewBalanceRequest builds a balance write body for the given level.
//
// bounds carries the device-reported range; pass the Balance from a preceding
// read. A nil bounds skips the range check, for callers that have no reading to
// validate against.
func NewBalanceRequest(level int, bounds *Balance) (*BalanceRequest, error) {
	if bounds != nil {
		if err := bounds.Validate(level); err != nil {
			return nil, err
		}
	}

	return &BalanceRequest{Target: level}, nil
}

// Validate reports whether level is within the range this device announced.
func (b *Balance) Validate(level int) error {
	if !b.Available {
		return fmt.Errorf("balance is not available on device %q (it reports balanceAvailable=false; balance exists only while the speaker is in a stereo pair)", b.DeviceID)
	}

	if level < b.Min || level > b.Max {
		return fmt.Errorf("invalid balance level: %d (device reports a range of %d to %d)", level, b.Min, b.Max)
	}

	return nil
}

// Clamp limits level to the range this device announced.
func (b *Balance) Clamp(level int) int {
	if level < b.Min {
		return b.Min
	}

	if level > b.Max {
		return b.Max
	}

	return level
}

// Range returns the device-reported bounds and default.
func (b *Balance) Range() (lowest, highest, fallback int) {
	return b.Min, b.Max, b.Default
}

// GetLevel returns the target balance level
func (b *Balance) GetLevel() int {
	return b.Target
}

// GetActualLevel returns the actual balance level
func (b *Balance) GetActualLevel() int {
	return b.Actual
}

// IsAtTarget returns true if actual balance matches target balance
func (b *Balance) IsAtTarget() bool {
	return b.Target == b.Actual
}

// LevelName returns a descriptive name for level, scaled to the device's own
// range rather than an assumed one — "Far Left" means the end of the range the
// speaker reports, whatever that range is.
func (b *Balance) LevelName(level int) string {
	switch {
	case level == 0:
		return "Center"
	case level < 0:
		return sideName("Left", -level, -b.Min)
	default:
		return sideName("Right", level, b.Max)
	}
}

// sideName grades magnitude against the extent available on that side.
func sideName(side string, magnitude, extent int) string {
	if extent <= 0 {
		return side
	}

	switch {
	case magnitude*3 >= extent*2:
		return "Far " + side
	case magnitude*3 >= extent:
		return side
	default:
		return "Slightly " + side
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
	if !b.Available {
		return "Balance: unavailable (the speaker is not in a stereo pair)"
	}

	return fmt.Sprintf("Balance: %d (%s), range %d..%d", b.GetLevel(), b.LevelName(b.GetLevel()), b.Min, b.Max)
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
	return b.Target - b.Actual
}

// LeftRightPercentage expresses the current level as a left/right split,
// scaled to the device-reported range. A level at Min is 100/0.
func (b *Balance) LeftRightPercentage() (left, right int) {
	level := b.GetLevel()

	switch {
	case level < 0 && b.Min < 0:
		left = 50 + (-level*50)/(-b.Min)
	case level > 0 && b.Max > 0:
		left = 50 - (level*50)/b.Max
	default:
		left = 50
	}

	return left, 100 - left
}
