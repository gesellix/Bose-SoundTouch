package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeSession is a StateMachine that records the order of
// invocations and lets each test inject per-step errors.
type fakeSession struct {
	calls  []string
	errors map[string]error
	closed bool
}

func (f *fakeSession) record(name string) error {
	if e, ok := f.errors[name]; ok && e != nil {
		return e
	}

	f.calls = append(f.calls, name)

	return nil
}

func (f *fakeSession) Start(_ context.Context) error { return f.record("Start") }
func (f *fakeSession) Enter(_ context.Context) error { return f.record("Enter") }
func (f *fakeSession) Leave(_ context.Context) error { return f.record("Leave") }
func (f *fakeSession) IdentifyLeave(_ context.Context) error {
	return f.record("IdentifyLeave")
}

func (f *fakeSession) IdentifyEnter(_ context.Context, timeoutMs int) error {
	return f.record(fmt.Sprintf("IdentifyEnter(%d)", timeoutMs))
}

func (f *fakeSession) SetLanguage(_ context.Context, code int) error {
	return f.record(fmt.Sprintf("SetLanguage(%d)", code))
}

func (f *fakeSession) SetName(_ context.Context, name string) error {
	return f.record("SetName(" + name + ")")
}

func (f *fakeSession) SetMargeAccount(_ context.Context, accountID, token string) error {
	return f.record(fmt.Sprintf("SetMargeAccount(%s,%s)", accountID, token))
}

func (f *fakeSession) PushCustomerSupportInfo(_ context.Context) error {
	return f.record("PushCustomerSupportInfo")
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

// fakeInfoResponder produces an http.Response carrying canned /info XML.
// pairedAccount toggles between "unpaired" and "paired with this UUID."
type fakeInfoResponder struct {
	deviceID       string
	paired         string // empty = unpaired
	postInitPaired string // /info reading after the plan ran
	reads          int
}

func (f *fakeInfoResponder) get(_ string) (*http.Response, error) {
	f.reads++

	acct := f.paired
	if f.reads >= 2 && f.postInitPaired != "" {
		acct = f.postInitPaired
	}

	body := fmt.Sprintf(
		`<info deviceID="%s"><name>Test</name><margeAccountUUID>%s</margeAccountUUID></info>`,
		f.deviceID, acct,
	)

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}, nil
}

func newTestManagerWithFakes(t *testing.T, info *fakeInfoResponder, sess *fakeSession) *Manager {
	t.Helper()

	m := &Manager{
		ServerURL: "http://aftertouch.local:8000",
		HTTPGet:   info.get,
		NewSession: func(_, _ string, _ time.Duration) (StateMachine, error) {
			return sess, nil
		},
	}

	return m
}

func TestExecuteInitPlan_FactoryReset_GeneratesAccountAndRunsAllSteps(t *testing.T) {
	info := &fakeInfoResponder{
		deviceID:       "AABBCCDDEEFF",
		paired:         "",
		postInitPaired: "", // filled below after we know which ID was generated
	}
	sess := &fakeSession{}
	m := newTestManagerWithFakes(t, info, sess)

	// Intercept the generated account ID so we can prime the post-init
	// /info read to return it. Easiest way: pre-supply a known AccountID.
	plan := InitPlan{
		DeviceIP:       "192.0.2.10",
		AccountID:      "1234567",
		DeviceName:     "Living Room",
		SkipURLRewrite: true,
	}
	info.postInitPaired = "1234567"

	var events []StepEvent

	got, err := m.ExecuteInitPlan(context.Background(), plan, func(e StepEvent) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("ExecuteInitPlan: %v", err)
	}

	if got.AccountID != "1234567" {
		t.Errorf("AccountID = %q, want 1234567", got.AccountID)
	}

	if got.Language != LanguageEnglish {
		t.Errorf("Language = %d, want %d (default English)", got.Language, LanguageEnglish)
	}

	wantCalls := []string{
		"Start",
		"IdentifyEnter(300000)",
		"SetLanguage(2)",
		"Enter",
		"IdentifyLeave",
		"SetName(Living Room)",
		"SetMargeAccount(1234567," + DefaultMargeAuthToken + ")",
		"Leave",
		"PushCustomerSupportInfo",
	}
	if got, want := strings.Join(sess.calls, "|"), strings.Join(wantCalls, "|"); got != want {
		t.Errorf("call order mismatch\n got: %s\nwant: %s", got, want)
	}

	if !sess.closed {
		t.Error("expected session to be closed")
	}

	// Verify the URL-rewrite event was emitted as Skipped, not silently dropped.
	if !hasEvent(events, StepURLRewrite, StatusSkipped) {
		t.Errorf("expected StepURLRewrite Skipped event, got %v", eventSummary(events))
	}

	// Final verify step must report OK.
	if !hasEvent(events, StepVerify, StatusOK) {
		t.Errorf("expected StepVerify OK, got %v", eventSummary(events))
	}
}

func TestExecuteInitPlan_ReusesExistingAccountUUID(t *testing.T) {
	info := &fakeInfoResponder{
		deviceID:       "AABBCCDDEEFF",
		paired:         "9876543",
		postInitPaired: "9876543",
	}
	sess := &fakeSession{}
	m := newTestManagerWithFakes(t, info, sess)

	plan := InitPlan{
		DeviceIP:       "192.0.2.10",
		SkipURLRewrite: true,
	}

	got, err := m.ExecuteInitPlan(context.Background(), plan, nil)
	if err != nil {
		t.Fatalf("ExecuteInitPlan: %v", err)
	}

	if got.AccountID != "9876543" {
		t.Errorf("AccountID = %q, want 9876543 (the device's existing UUID)", got.AccountID)
	}
}

func TestExecuteInitPlan_GeneratesAccountWhenDeviceUUIDInvalid(t *testing.T) {
	// Devices that report a non-7-digit UUID (e.g. a stale local value) must
	// not be reused — we treat them as factory-reset for ID purposes.
	info := &fakeInfoResponder{
		deviceID:       "AABBCCDDEEFF",
		paired:         "not-7-digits",
		postInitPaired: "", // we'll learn the generated ID from the result
	}
	sess := &fakeSession{}
	m := newTestManagerWithFakes(t, info, sess)

	// Pre-generate so the post-init /info knows what to return.
	plan := InitPlan{DeviceIP: "192.0.2.10", SkipURLRewrite: true}

	// Track the generated AccountID and feed it back as the post-init /info value.
	progress := func(e StepEvent) {
		if e.Kind == StepGenerateAccountID && e.Status == StatusOK && strings.Contains(e.Name, "generate account ID=") {
			info.postInitPaired = strings.TrimPrefix(e.Name, "generate account ID=")
		}
	}

	got, err := m.ExecuteInitPlan(context.Background(), plan, progress)
	if err != nil {
		t.Fatalf("ExecuteInitPlan: %v", err)
	}

	if !IsValidAccountID(got.AccountID) {
		t.Errorf("got.AccountID = %q, want a valid 7-digit ID", got.AccountID)
	}

	if got.AccountID == "not-7-digits" {
		t.Error("orchestrator should not reuse an invalid UUID")
	}
}

func TestExecuteInitPlan_RejectsInvalidSuppliedAccountID(t *testing.T) {
	info := &fakeInfoResponder{deviceID: "X", paired: ""}
	sess := &fakeSession{}
	m := newTestManagerWithFakes(t, info, sess)

	plan := InitPlan{
		DeviceIP:       "192.0.2.10",
		AccountID:      "abc",
		SkipURLRewrite: true,
	}

	_, err := m.ExecuteInitPlan(context.Background(), plan, nil)
	if err == nil {
		t.Fatal("expected error for invalid AccountID")
	}

	if !strings.Contains(err.Error(), "invalid AccountID") {
		t.Errorf("err = %v, want to mention invalid AccountID", err)
	}

	if len(sess.calls) != 0 {
		t.Errorf("expected zero WS calls after rejection, got %v", sess.calls)
	}
}

func TestExecuteInitPlan_StopsAtFirstFailedStep(t *testing.T) {
	info := &fakeInfoResponder{deviceID: "X", paired: "", postInitPaired: "1234567"}
	sess := &fakeSession{
		errors: map[string]error{
			"Enter": errors.New("device dropped the SETUP_ENTER frame"),
		},
	}
	m := newTestManagerWithFakes(t, info, sess)

	plan := InitPlan{
		DeviceIP:       "192.0.2.10",
		AccountID:      "1234567",
		SkipURLRewrite: true,
	}

	var events []StepEvent

	_, err := m.ExecuteInitPlan(context.Background(), plan, func(e StepEvent) { events = append(events, e) })
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "SETUP_ENTER") {
		t.Errorf("err = %v, want to mention SETUP_ENTER", err)
	}

	// Steps after the failed one must not have been called.
	for _, c := range sess.calls {
		if c == "Leave" || c == "PushCustomerSupportInfo" {
			t.Errorf("unexpected post-failure call %q", c)
		}
	}

	if !hasEvent(events, StepSetupEnter, StatusFailed) {
		t.Errorf("expected StepSetupEnter Failed event, got %v", eventSummary(events))
	}
}

func TestExecuteInitPlan_EmptyDeviceNameSkipsNameStep(t *testing.T) {
	info := &fakeInfoResponder{deviceID: "X", paired: "", postInitPaired: "1234567"}
	sess := &fakeSession{}
	m := newTestManagerWithFakes(t, info, sess)

	plan := InitPlan{
		DeviceIP:       "192.0.2.10",
		AccountID:      "1234567",
		SkipURLRewrite: true,
		// DeviceName intentionally empty
	}

	if _, err := m.ExecuteInitPlan(context.Background(), plan, nil); err != nil {
		t.Fatalf("ExecuteInitPlan: %v", err)
	}

	for _, c := range sess.calls {
		if strings.HasPrefix(c, "SetName(") {
			t.Errorf("SetName should be skipped when DeviceName is empty, but was called: %q", c)
		}
	}
}

func TestExecuteInitPlan_RequiresDeviceIP(t *testing.T) {
	m := &Manager{ServerURL: "http://aftertouch.local:8000"}

	_, err := m.ExecuteInitPlan(context.Background(), InitPlan{}, nil)
	if err == nil || !strings.Contains(err.Error(), "DeviceIP") {
		t.Errorf("err = %v, want to mention DeviceIP", err)
	}
}

func TestExecuteInitPlan_RequiresServiceURL(t *testing.T) {
	m := &Manager{}

	_, err := m.ExecuteInitPlan(context.Background(), InitPlan{DeviceIP: "192.0.2.10"}, nil)
	if err == nil || !strings.Contains(err.Error(), "ServiceURL") {
		t.Errorf("err = %v, want to mention ServiceURL", err)
	}
}

func TestExecuteInitPlan_FailsOnPostInitVerifyMismatch(t *testing.T) {
	// Device's post-init /info still reports the old account — surface
	// that as a verification failure rather than a silent success.
	info := &fakeInfoResponder{
		deviceID:       "X",
		paired:         "",
		postInitPaired: "9999999", // not equal to plan.AccountID
	}
	sess := &fakeSession{}
	m := newTestManagerWithFakes(t, info, sess)

	plan := InitPlan{
		DeviceIP:       "192.0.2.10",
		AccountID:      "1234567",
		SkipURLRewrite: true,
	}

	_, err := m.ExecuteInitPlan(context.Background(), plan, nil)
	if err == nil {
		t.Fatal("expected verification error")
	}

	if !strings.Contains(err.Error(), "margeAccountUUID") {
		t.Errorf("err = %v, want to mention margeAccountUUID mismatch", err)
	}
}

func hasEvent(events []StepEvent, kind StepKind, status StepStatus) bool {
	for _, e := range events {
		if e.Kind == kind && e.Status == status {
			return true
		}
	}

	return false
}

func eventSummary(events []StepEvent) string {
	parts := make([]string, 0, len(events))
	for _, e := range events {
		parts = append(parts, fmt.Sprintf("%d/%s", e.Kind, e.Status))
	}

	return strings.Join(parts, ",")
}
