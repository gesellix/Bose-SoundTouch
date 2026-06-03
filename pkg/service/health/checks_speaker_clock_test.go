package health

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// newSpeakerClockDatastore creates a temporary datastore populated with a
// single device. ip may be "" to test the empty-IP skip behaviour.
func newSpeakerClockDatastore(t *testing.T, accountID, deviceID, ip string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "speaker-clock-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	if err := ds.SaveDeviceInfo(accountID, deviceID, &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		AccountID: accountID,
		Name:      "Speaker-" + deviceID,
		IPAddress: ip,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func TestSpeakerClock_InSync(t *testing.T) {
	now := time.Unix(1748908800, 0) // fixed reference: 2025-06-03 00:00:00 UTC
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 5, now.Unix() - 30, true // 5s skew, recent NTP sync
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 0 {
		t.Errorf("expected no findings for in-sync clock, got %+v", got)
	}
}

func TestSpeakerClock_InfoTier_90sSkew(t *testing.T) {
	now := time.Unix(1748908800, 0)
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 90, now.Unix() - 30, true
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding for 90s skew, got %+v", got)
	}
	if got[0].Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo, got %s", got[0].Severity)
	}
}

func TestSpeakerClock_WarningTier_10mSkew(t *testing.T) {
	now := time.Unix(1748908800, 0)
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 600, now.Unix() - 30, true // 10 minutes
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding for 10m skew, got %+v", got)
	}
	if got[0].Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %s", got[0].Severity)
	}
	if !strings.Contains(got[0].Message, "10m") {
		t.Errorf("expected duration in message, got %q", got[0].Message)
	}
}

func TestSpeakerClock_ErrorTier_33daysSkew(t *testing.T) {
	// The #345 case: speaker clock ~33 days behind service time.
	now := time.Unix(1748908800, 0)
	skewSecs := int64(33 * 24 * 60 * 60)
	speakerUTC := now.Unix() - skewSecs

	ds := newSpeakerClockDatastore(t, "1000001", "606405FE97AE", "192.0.2.20")

	clockFn := func(string) (int64, int64, bool) {
		return speakerUTC, 0, true // sync==0 means never NTP-synced
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding for 33d skew, got %+v", got)
	}
	if got[0].Severity != SeverityError {
		t.Errorf("expected SeverityError, got %s", got[0].Severity)
	}

	// Message must name both the speaker time and the service time.
	speakerTimeStr := time.Unix(speakerUTC, 0).UTC().Format(time.RFC3339)
	serviceTimeStr := now.UTC().Format(time.RFC3339)

	if !strings.Contains(got[0].Message, speakerTimeStr) {
		t.Errorf("expected speaker time %q in message, got %q", speakerTimeStr, got[0].Message)
	}
	if !strings.Contains(got[0].Message, serviceTimeStr) {
		t.Errorf("expected service time %q in message, got %q", serviceTimeStr, got[0].Message)
	}
	if !strings.Contains(strings.ToLower(got[0].Message), "tls") {
		t.Errorf("expected TLS mention in message, got %q", got[0].Message)
	}
}

func TestSpeakerClock_ErrorTier_PlausibilityWindow(t *testing.T) {
	// Speaker epoch is just inside year 2000 boundary (but within a minute of it),
	// which is outside the plausibility window.
	now := time.Unix(1748908800, 0)
	speakerUTC := int64(946684800 + 60) // year 2000 + 60s: inside window but barely

	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return speakerUTC, 0, true
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one error finding for year-2000 epoch, got %+v", got)
	}
	if got[0].Severity != SeverityError {
		t.Errorf("expected SeverityError (>24h skew), got %s", got[0].Severity)
	}
}

func TestSpeakerClock_ErrorTier_BeforePlausibilityMin(t *testing.T) {
	// Epoch before year 2000 (e.g. Unix 0 = 1970).
	now := time.Unix(1748908800, 0)

	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return 0, 0, true // epoch 0 = 1970, outside plausibility window
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one error finding for epoch 0, got %+v", got)
	}
	if got[0].Severity != SeverityError {
		t.Errorf("expected SeverityError for implausible epoch, got %s", got[0].Severity)
	}
	if !strings.Contains(got[0].Message, "implausible") {
		t.Errorf("expected 'implausible' in message, got %q", got[0].Message)
	}
}

func TestSpeakerClock_OkFalse_Skipped(t *testing.T) {
	now := time.Unix(1748908800, 0)
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return 0, 0, false // unreachable
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 0 {
		t.Errorf("expected no findings when clockFn returns ok=false, got %+v", got)
	}
}

func TestSpeakerClock_EmptyIP_Skipped(t *testing.T) {
	now := time.Unix(1748908800, 0)
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "") // empty IP

	called := false
	clockFn := func(string) (int64, int64, bool) {
		called = true
		return now.Unix(), now.Unix() - 30, true
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 0 {
		t.Errorf("expected no findings for device with empty IP, got %+v", got)
	}
	if called {
		t.Error("clockFn should not be called for a device with empty IP")
	}
}

func TestSpeakerClock_StaleNTPSync_DetailsNote(t *testing.T) {
	now := time.Unix(1748908800, 0)
	// Skew of 90s (Info tier) but NTP sync was 2 hours ago.
	lastSync := now.Unix() - int64(2*time.Hour/time.Second)

	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 90, lastSync, true
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if !strings.Contains(got[0].Details, "NTP is likely failing") {
		t.Errorf("expected NTP staleness note in Details, got %q", got[0].Details)
	}
}

func TestSpeakerClock_ZeroSync_DetailsNote(t *testing.T) {
	now := time.Unix(1748908800, 0)
	// Skew of 90s (Info tier) with zero sync (never synced).
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 90, 0, true // sync == 0
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if !strings.Contains(got[0].Details, "never") {
		t.Errorf("expected 'never' in NTP staleness note in Details, got %q", got[0].Details)
	}
	if !strings.Contains(got[0].Details, "NTP is likely failing") {
		t.Errorf("expected 'NTP is likely failing' in Details, got %q", got[0].Details)
	}
}

// --- QuickFix descriptor tests ---

func TestSpeakerClock_ErrorFinding_CarriesSetClockQuickFix(t *testing.T) {
	now := time.Unix(1748908800, 0)
	skewSecs := int64(33 * 24 * 60 * 60)

	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - skewSecs, 0, true
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != SeverityError {
		t.Fatalf("expected SeverityError, got %s", got[0].Severity)
	}

	hasSetClock := false
	for _, qf := range got[0].QuickFixes {
		if qf.ID == "set_clock" {
			hasSetClock = true
		}
	}
	if !hasSetClock {
		t.Errorf("expected set_clock QuickFix on Error finding, got %+v", got[0].QuickFixes)
	}
}

func TestSpeakerClock_WarningFinding_CarriesSetClockQuickFix(t *testing.T) {
	now := time.Unix(1748908800, 0)

	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 600, now.Unix() - 30, true // 10 minutes skew
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != SeverityWarning {
		t.Fatalf("expected SeverityWarning, got %s", got[0].Severity)
	}

	hasSetClock := false
	for _, qf := range got[0].QuickFixes {
		if qf.ID == "set_clock" {
			hasSetClock = true
		}
	}
	if !hasSetClock {
		t.Errorf("expected set_clock QuickFix on Warning finding, got %+v", got[0].QuickFixes)
	}
}

func TestSpeakerClock_InfoFinding_NoSetClockQuickFix(t *testing.T) {
	now := time.Unix(1748908800, 0)

	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	clockFn := func(string) (int64, int64, bool) {
		return now.Unix() - 90, now.Unix() - 30, true // 90s skew = Info tier
	}

	got := runSpeakerClockCheck(ds, clockFn, now)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}
	if got[0].Severity != SeverityInfo {
		t.Fatalf("expected SeverityInfo, got %s", got[0].Severity)
	}

	for _, qf := range got[0].QuickFixes {
		if qf.ID == "set_clock" {
			t.Errorf("Info finding must not carry set_clock QuickFix, but got one")
		}
	}
}

// --- RunFix dispatch tests ---

// newSpeakerClockRegistry creates a Registry with RegisterSpeakerClockCheck
// wired up using the provided setFn stub.
func newSpeakerClockRegistry(t *testing.T, ds *datastore.DataStore, setFn func(string) error) *Registry {
	t.Helper()

	r := NewRegistry()
	clockFn := func(string) (int64, int64, bool) {
		return time.Now().Unix(), time.Now().Unix() - 30, true
	}
	RegisterSpeakerClockCheck(r, ds, clockFn, setFn)

	return r
}

func TestSpeakerClock_RunFix_Success(t *testing.T) {
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	var calledIP string
	setFn := func(ip string) error {
		calledIP = ip
		return nil
	}

	r := newSpeakerClockRegistry(t, ds, setFn)

	msg, _, err := r.RunFix(CheckIDSpeakerClock, "set_clock", Target{Device: "DEVICE01"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledIP != "192.0.2.10" {
		t.Errorf("setFn called with IP %q, want 192.0.2.10", calledIP)
	}
	if !strings.Contains(msg, "Set clock on") {
		t.Errorf("expected 'Set clock on' in message, got %q", msg)
	}
	if !strings.Contains(msg, "restoring time sync is the durable fix") {
		t.Errorf("expected NTP band-aid note in message, got %q", msg)
	}
}

func TestSpeakerClock_RunFix_SetFnError_Propagates(t *testing.T) {
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	setFn := func(string) error {
		return errors.New("connection refused")
	}

	r := newSpeakerClockRegistry(t, ds, setFn)

	_, _, err := r.RunFix(CheckIDSpeakerClock, "set_clock", Target{Device: "DEVICE01"})
	if err == nil {
		t.Fatal("expected error from failing setFn, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected original error in message, got %q", err.Error())
	}
}

func TestSpeakerClock_RunFix_UnknownDevice_ReturnsError(t *testing.T) {
	ds := newSpeakerClockDatastore(t, "1000001", "DEVICE01", "192.0.2.10")

	setFn := func(string) error { return nil }

	r := newSpeakerClockRegistry(t, ds, setFn)

	_, _, err := r.RunFix(CheckIDSpeakerClock, "set_clock", Target{Device: "NOSUCHDEVICE"})
	if err == nil {
		t.Fatal("expected error for unknown device, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}
