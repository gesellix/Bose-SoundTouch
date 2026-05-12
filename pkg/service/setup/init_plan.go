package setup

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// InitPlan describes everything required to take a factory-reset (or
// freshly-joined) speaker from "on the Wi-Fi" to "fully paired with a
// usable margeAccountUUID, pointing at AfterTouch."
//
// All fields are gathered upfront so the orchestrator can validate the
// plan before touching the device. AccountID may be left empty — the
// orchestrator either reuses the device's existing UUID (if it already
// has one) or generates a fresh 7-digit ID via GenerateAccountID.
type InitPlan struct {
	DeviceIP   string
	ServiceURL string
	AccountID  string
	Language   int
	DeviceName string
	AuthToken  string

	// SkipURLRewrite skips the telnet envswitch step. The caller asserts
	// the device's runtime marge URL already points at AfterTouch (e.g. a
	// prior migration run, or a controlled test environment).
	SkipURLRewrite bool

	// StepTimeout overrides the per-WebSocket-step deadline.
	StepTimeout time.Duration
}

// StepKind identifies a step for progress reporting.
type StepKind int

// Step kinds emitted by ExecuteInitPlan. Numbered explicitly so the wire
// format is stable for any future UI/JSON consumer.
const (
	StepReadDeviceInfo    StepKind = 1
	StepURLRewrite        StepKind = 2
	StepGenerateAccountID StepKind = 3
	StepDialWebSocket     StepKind = 4
	StepSetupStart        StepKind = 5
	StepIdentifyEnter     StepKind = 6
	StepLanguage          StepKind = 7
	StepSetupEnter        StepKind = 8
	StepIdentifyLeave     StepKind = 9
	StepName              StepKind = 10
	StepPairAccount       StepKind = 11
	StepSetupLeave        StepKind = 12
	StepPushTelemetry     StepKind = 13
	StepVerify            StepKind = 14
)

// StepStatus is the per-step outcome surfaced via StepEvent.Status.
type StepStatus string

// Step statuses. "skipped" covers both caller-requested skips (e.g.
// SkipURLRewrite) and naturally-empty steps (e.g. SetName with no
// DeviceName change).
const (
	StatusRunning StepStatus = "running"
	StatusOK      StepStatus = "ok"
	StatusSkipped StepStatus = "skipped"
	StatusFailed  StepStatus = "failed"
)

// StepEvent is emitted before and after each step so callers can drive a UI.
type StepEvent struct {
	Kind   StepKind
	Name   string
	Status StepStatus
	Err    error
}

// ProgressFunc receives StepEvents as the plan executes. May be nil.
type ProgressFunc func(StepEvent)

// ExecuteInitPlan runs the full speaker-initialization sequence described
// in docs/reference/DEVICE-PAIRING-FLOW.md:
//
//  1. read /info (so we know the device ID and current pairing state)
//  2. rewrite URLs via telnet envswitch (so the device's downstream POST
//     after setMargeAccount lands on AfterTouch instead of dead Bose cloud)
//  3. resolve an account ID — reuse an existing margeAccountUUID, otherwise
//     generate a fresh non-colliding 7-digit ID
//  4. open the WebSocket setup session
//  5. drive the state machine: SETUP_START → IDENTIFY_ENTER → language →
//     SETUP_ENTER → IDENTIFY_LEAVE → name → setMargeAccount → SETUP_LEAVE
//     → pushCustomerSupportInfoToMarge
//  6. verify by re-reading /info
//
// The returned InitPlan reflects any defaulting that happened (generated
// account ID, defaulted language, etc.) so callers can persist it.
func (m *Manager) ExecuteInitPlan(ctx context.Context, plan InitPlan, progress ProgressFunc) (InitPlan, error) {
	if plan.DeviceIP == "" {
		return plan, errors.New("InitPlan.DeviceIP is required")
	}

	if plan.ServiceURL == "" {
		plan.ServiceURL = m.ServerURL
	}

	if plan.ServiceURL == "" {
		return plan, errors.New("InitPlan.ServiceURL is required (and Manager.ServerURL is empty)")
	}

	if plan.Language == 0 {
		plan.Language = LanguageEnglish
	}

	if plan.AuthToken == "" {
		plan.AuthToken = "Bearer aftertouch"
	}

	emit := func(kind StepKind, name string, status StepStatus, err error) {
		if progress != nil {
			progress(StepEvent{Kind: kind, Name: name, Status: status, Err: err})
		}
	}

	emit(StepReadDeviceInfo, "read /info", StatusRunning, nil)

	info, err := m.GetLiveDeviceInfo(plan.DeviceIP)
	if err != nil {
		emit(StepReadDeviceInfo, "read /info", StatusFailed, err)
		return plan, fmt.Errorf("read /info: %w", err)
	}

	emit(StepReadDeviceInfo, "read /info", StatusOK, nil)

	if plan.SkipURLRewrite {
		emit(StepURLRewrite, "telnet URL rewrite", StatusSkipped, nil)
	} else {
		emit(StepURLRewrite, "telnet URL rewrite", StatusRunning, nil)

		urls := defaultTelnetURLs(plan.ServiceURL)
		if _, err := m.migrateViaTelnet(plan.DeviceIP, plan.ServiceURL, urls); err != nil {
			emit(StepURLRewrite, "telnet URL rewrite", StatusFailed, err)
			return plan, fmt.Errorf("URL rewrite: %w", err)
		}

		emit(StepURLRewrite, "telnet URL rewrite", StatusOK, nil)
	}

	if plan.AccountID == "" {
		if info.MargeAccountUUID != "" && IsValidAccountID(info.MargeAccountUUID) {
			plan.AccountID = info.MargeAccountUUID
			emit(StepGenerateAccountID, "reuse existing margeAccountUUID="+plan.AccountID, StatusOK, nil)
		} else {
			emit(StepGenerateAccountID, "generate account ID", StatusRunning, nil)

			known := listKnownAccountIDs(m)

			id, err := GenerateAccountID(known)
			if err != nil {
				emit(StepGenerateAccountID, "generate account ID", StatusFailed, err)
				return plan, fmt.Errorf("generate account ID: %w", err)
			}

			plan.AccountID = id

			emit(StepGenerateAccountID, "generate account ID="+id, StatusOK, nil)
		}
	} else if !IsValidAccountID(plan.AccountID) {
		err := fmt.Errorf("invalid AccountID %q: must be exactly 7 digits", plan.AccountID)
		emit(StepGenerateAccountID, "validate account ID", StatusFailed, err)

		return plan, err
	}

	emit(StepDialWebSocket, "dial websocket", StatusRunning, nil)

	if m.NewSetupSession == nil {
		err := errors.New("Manager.NewSetupSession is nil — call NewManager or set it explicitly")
		emit(StepDialWebSocket, "dial websocket", StatusFailed, err)

		return plan, err
	}

	session, err := m.NewSetupSession(plan.DeviceIP, info.DeviceID, plan.StepTimeout)
	if err != nil {
		emit(StepDialWebSocket, "dial websocket", StatusFailed, err)
		return plan, fmt.Errorf("dial websocket: %w", err)
	}

	defer func() { _ = session.Close() }()

	emit(StepDialWebSocket, "dial websocket", StatusOK, nil)

	type stepDef struct {
		kind StepKind
		name string
		skip bool
		fn   func(context.Context) error
	}

	steps := []stepDef{
		{kind: StepSetupStart, name: "SETUP_START", fn: session.Start},
		{kind: StepIdentifyEnter, name: "SETUP_IDENTIFY_DEVICE_ENTER", fn: func(ctx context.Context) error {
			// 300_000 ms matches the value captured from the official Bose
			// app; the device flashes/beeps for that long while the user
			// confirms identity. We pass it explicitly so the wire value
			// is decided here rather than inside the session helper.
			return session.IdentifyEnter(ctx, 300000)
		}},
		{kind: StepLanguage, name: fmt.Sprintf("sysLanguage=%d", plan.Language), fn: func(ctx context.Context) error {
			return session.SetLanguage(ctx, plan.Language)
		}},
		{kind: StepSetupEnter, name: "SETUP_ENTER", fn: session.Enter},
		{kind: StepIdentifyLeave, name: "SETUP_IDENTIFY_DEVICE_LEAVE", fn: session.IdentifyLeave},
		{kind: StepName, name: "name=" + plan.DeviceName, skip: plan.DeviceName == "", fn: func(ctx context.Context) error {
			return session.SetName(ctx, plan.DeviceName)
		}},
		{kind: StepPairAccount, name: "setMargeAccount=" + plan.AccountID, fn: func(ctx context.Context) error {
			return session.SetMargeAccount(ctx, plan.AccountID, plan.AuthToken)
		}},
		{kind: StepSetupLeave, name: "SETUP_LEAVE", fn: session.Leave},
		{kind: StepPushTelemetry, name: "pushCustomerSupportInfoToMarge", fn: session.PushCustomerSupportInfo},
	}

	for _, st := range steps {
		if st.skip {
			emit(st.kind, st.name+" (no change)", StatusSkipped, nil)
			continue
		}

		emit(st.kind, st.name, StatusRunning, nil)

		if err := st.fn(ctx); err != nil {
			emit(st.kind, st.name, StatusFailed, err)
			return plan, fmt.Errorf("%s: %w", st.name, err)
		}

		emit(st.kind, st.name, StatusOK, nil)
	}

	emit(StepVerify, "verify /info margeAccountUUID", StatusRunning, nil)

	verify, err := m.GetLiveDeviceInfo(plan.DeviceIP)
	if err != nil {
		emit(StepVerify, "verify /info", StatusFailed, err)
		return plan, fmt.Errorf("verify /info: %w", err)
	}

	if verify.MargeAccountUUID != plan.AccountID {
		err := fmt.Errorf("post-init /info shows margeAccountUUID=%q, want %q", verify.MargeAccountUUID, plan.AccountID)
		emit(StepVerify, "verify /info", StatusFailed, err)

		return plan, err
	}

	emit(StepVerify, "verify /info margeAccountUUID="+plan.AccountID, StatusOK, nil)

	return plan, nil
}

// listKnownAccountIDs collects account IDs already known to the local
// datastore so GenerateAccountID can avoid collisions. Returns nil when
// no datastore is configured or it errors — uniqueness is best-effort.
func listKnownAccountIDs(m *Manager) []string {
	if m.DataStore == nil {
		return nil
	}

	ids, err := m.DataStore.ListAccounts()
	if err != nil {
		return nil
	}

	return ids
}
