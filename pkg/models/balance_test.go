package models

import (
	"encoding/xml"
	"strings"
	"testing"
)

// capturedBalanceResponse is the exact document a SoundTouch 10 returns as the
// master of a stereo pair (variant=rhino, moduleType=sm2, FW 27.0.6, hardware
// run 2026-09-08). Every assertion in this file is anchored to it.
//
// The previous model assumed lowercase <targetbalance>/<actualbalance> and a
// -50..50 range, copied from /bass with the range widened, and the tests
// asserted that invented shape back at it — which is exactly why it went
// unnoticed for so long (GH-699).
const capturedBalanceResponse = `<?xml version="1.0" encoding="UTF-8" ?>
<balance deviceID="DEVICEID01">
  <balanceAvailable>true</balanceAvailable>
  <balanceMin>-7</balanceMin>
  <balanceMax>7</balanceMax>
  <balanceDefault>0</balanceDefault>
  <targetBalance>-3</targetBalance>
  <actualBalance>-3</actualBalance>
</balance>`

// capturedUnavailableBalance is what an UNPAIRED speaker reports, captured
// from hardware after a pair was torn down (2026-09-09).
//
// Only unpaired: both members of a live pair report Available true, including
// the right-hand one, contrary to third-party notes.
//
// Note what it is not: it is a successful response, not an error, and it still
// carries the full range AND the target the speaker held while it was paired.
// So Available is the only field that says whether the control applies — a
// caller that checks "is the range non-zero" or "is the target set" would
// wrongly conclude balance is usable here.
const capturedUnavailableBalance = `<balance deviceID="DEVICEID01">
  <balanceAvailable>false</balanceAvailable>
  <balanceMin>-7</balanceMin>
  <balanceMax>7</balanceMax>
  <balanceDefault>0</balanceDefault>
  <targetBalance>-3</targetBalance>
  <actualBalance>-3</actualBalance>
</balance>`

func TestBalanceUnmarshalCapturedResponse(t *testing.T) {
	var balance Balance
	if err := xml.Unmarshal([]byte(capturedBalanceResponse), &balance); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	checks := []struct {
		name string
		got  int
		want int
	}{
		{"Min", balance.Min, -7},
		{"Max", balance.Max, 7},
		{"Default", balance.Default, 0},
		{"Target", balance.Target, -3},
		{"Actual", balance.Actual, -3},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}

	if !balance.Available {
		t.Error("Available = false, want true for a paired master")
	}

	if balance.DeviceID != "DEVICEID01" {
		t.Errorf("DeviceID = %q, want DEVICEID01", balance.DeviceID)
	}
}

// TestBalanceRejectsLowercaseElements guards the casing specifically: the old
// model's <targetbalance> is not what the speaker sends, and a document using
// it must not silently decode as a centered balance.
func TestBalanceRejectsLowercaseElements(t *testing.T) {
	legacy := `<balance deviceID="DEVICEID01">` +
		`<targetbalance>5</targetbalance><actualbalance>5</actualbalance></balance>`

	var balance Balance
	if err := xml.Unmarshal([]byte(legacy), &balance); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if balance.Target != 0 || balance.Actual != 0 {
		t.Errorf("lowercase elements decoded as %d/%d; the wire format is camelCase",
			balance.Target, balance.Actual)
	}
}

func TestBalanceUnavailable(t *testing.T) {
	var balance Balance
	if err := xml.Unmarshal([]byte(capturedUnavailableBalance), &balance); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if balance.Available {
		t.Error("Available = true, want false")
	}

	if err := balance.Validate(0); err == nil {
		t.Error("Validate accepted a level on a speaker reporting balanceAvailable=false")
	}

	if got := balance.String(); !strings.Contains(got, "unavailable") {
		t.Errorf("String() = %q, want it to say the balance is unavailable", got)
	}

	// The stale values are still there, which is exactly the trap: only
	// Available may be used to decide whether the control applies.
	if balance.Max != 7 || balance.Target != -3 {
		t.Errorf("got range %d..%d target %d; an unavailable reading still carries them",
			balance.Min, balance.Max, balance.Target)
	}
}

// TestBalanceOutOfRangeStillParses pins that an unexpected value yields data
// rather than a parse failure. The old model validated inside UnmarshalXML, so
// a number outside its assumed range turned the whole reading into an error —
// an unexpected number became NO data at all.
func TestBalanceOutOfRangeStillParses(t *testing.T) {
	odd := `<balance deviceID="DEVICEID01"><balanceAvailable>true</balanceAvailable>` +
		`<balanceMin>-7</balanceMin><balanceMax>7</balanceMax>` +
		`<targetBalance>99</targetBalance><actualBalance>99</actualBalance></balance>`

	var balance Balance
	if err := xml.Unmarshal([]byte(odd), &balance); err != nil {
		t.Fatalf("an out-of-range value must still parse, got %v", err)
	}

	if balance.Target != 99 {
		t.Errorf("Target = %d, want the value as reported (99)", balance.Target)
	}

	// It is out of range, and Validate is where that is reported.
	if err := balance.Validate(balance.Target); err == nil {
		t.Error("Validate accepted 99 against a -7..7 range")
	}
}

// TestBothPairMembersReportBalance records what the hardware actually does,
// against a claim we repeated from third-party notes and then disproved:
// balance is a property of the PAIR, and both members answer for it.
//
// Measured on a live ST-10 pair (FW 27.0.6, 2026-09-09): the LEFT/master and
// the RIGHT member both reported Available true, and writing 4 to the RIGHT
// member left both reporting target=4 within a second.
func TestBothPairMembersReportBalance(t *testing.T) {
	// The right-hand member's own response — not an unavailable one.
	rightMember := `<balance deviceID="DEVICEID02">` +
		`<balanceAvailable>true</balanceAvailable>` +
		`<balanceMin>-7</balanceMin><balanceMax>7</balanceMax><balanceDefault>0</balanceDefault>` +
		`<targetBalance>0</targetBalance><actualBalance>0</actualBalance></balance>`

	var balance Balance
	if err := xml.Unmarshal([]byte(rightMember), &balance); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !balance.Available {
		t.Error("Available = false; both members of a pair report balance")
	}

	if err := balance.Validate(4); err != nil {
		t.Errorf("Validate(4) on the right-hand member = %v, want nil", err)
	}
}

func TestBalanceValidateUsesDeviceRange(t *testing.T) {
	balance := &Balance{Available: true, Min: -7, Max: 7}

	for _, level := range []int{-7, -1, 0, 1, 7} {
		if err := balance.Validate(level); err != nil {
			t.Errorf("Validate(%d) = %v, want nil", level, err)
		}
	}

	for _, level := range []int{-8, 8, -50, 50} {
		if err := balance.Validate(level); err == nil {
			t.Errorf("Validate(%d) = nil; the device range is -7..7", level)
		}
	}
}

func TestBalanceClamp(t *testing.T) {
	balance := &Balance{Available: true, Min: -7, Max: 7}

	tests := []struct{ in, want int }{
		{-50, -7}, {-8, -7}, {-7, -7}, {0, 0}, {7, 7}, {8, 7}, {50, 7},
	}

	for _, tt := range tests {
		if got := balance.Clamp(tt.in); got != tt.want {
			t.Errorf("Clamp(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestBalanceLevelNameScalesToRange pins that the descriptive name grades
// against the DEVICE's range. On a -7..7 speaker, 7 is "Far Right"; the old
// implementation hardcoded thresholds for a ±50 scale, which made every
// reachable value on real hardware "Slightly" something.
func TestBalanceLevelNameScalesToRange(t *testing.T) {
	balance := &Balance{Available: true, Min: -7, Max: 7}

	tests := []struct {
		level int
		want  string
	}{
		{0, "Center"},
		{7, "Far Right"},
		{-7, "Far Left"},
		{3, "Right"},
		{-3, "Left"},
		{1, "Slightly Right"},
		{-1, "Slightly Left"},
	}

	for _, tt := range tests {
		if got := balance.LevelName(tt.level); got != tt.want {
			t.Errorf("LevelName(%d) = %q, want %q", tt.level, got, tt.want)
		}
	}

	// A degenerate range must not divide by zero or mislabel.
	empty := &Balance{}
	if got := empty.LevelName(1); got != "Right" {
		t.Errorf("LevelName on a zero range = %q, want %q", got, "Right")
	}
}

func TestBalanceLeftRightPercentage(t *testing.T) {
	balance := &Balance{Available: true, Min: -7, Max: 7}

	tests := []struct {
		target       int
		wantL, wantR int
	}{
		{0, 50, 50},
		{-7, 100, 0},
		{7, 0, 100},
	}

	for _, tt := range tests {
		balance.Target = tt.target

		left, right := balance.LeftRightPercentage()
		if left != tt.wantL || right != tt.wantR {
			t.Errorf("LeftRightPercentage() at %d = %d/%d, want %d/%d",
				tt.target, left, right, tt.wantL, tt.wantR)
		}
	}
}

// TestNewBalanceRequestMarshalsCapturedWriteBody pins the body that the
// speaker actually accepts, taken from the working WebSocket write.
func TestNewBalanceRequestMarshalsCapturedWriteBody(t *testing.T) {
	bounds := &Balance{Available: true, Min: -7, Max: 7}

	request, err := NewBalanceRequest(-3, bounds)
	if err != nil {
		t.Fatalf("NewBalanceRequest: %v", err)
	}

	encoded, err := xml.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	want := `<balance><targetBalance>-3</targetBalance></balance>`
	if string(encoded) != want {
		t.Errorf("\n got: %s\nwant: %s", encoded, want)
	}
}

func TestNewBalanceRequestRejectsOutOfRange(t *testing.T) {
	bounds := &Balance{Available: true, Min: -7, Max: 7}

	if _, err := NewBalanceRequest(15, bounds); err == nil {
		t.Error("NewBalanceRequest accepted 15 against a -7..7 range")
	}

	// A nil bounds means "no reading to check against"; the device decides.
	if _, err := NewBalanceRequest(15, nil); err != nil {
		t.Errorf("NewBalanceRequest with nil bounds = %v, want nil", err)
	}
}

func TestBalanceHelpers(t *testing.T) {
	balance := &Balance{Available: true, Min: -7, Max: 7, Target: -3, Actual: -2}

	if !balance.IsLeftBalance() || balance.IsRightBalance() || balance.IsBalanced() {
		t.Errorf("side helpers disagree for target %d", balance.Target)
	}

	if got := balance.GetBalanceChangeNeeded(); got != -1 {
		t.Errorf("GetBalanceChangeNeeded() = %d, want -1", got)
	}

	if balance.IsAtTarget() {
		t.Error("IsAtTarget() = true, but target and actual differ")
	}

	if got := GetBalanceLevelCategory(-3); got != "Left Channel" {
		t.Errorf("GetBalanceLevelCategory(-3) = %q", got)
	}
}
