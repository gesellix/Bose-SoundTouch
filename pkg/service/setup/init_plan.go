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
	plan, err := applyInitPlanDefaults(plan, m.ServerURL)
	if err != nil {
		return plan, err
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

	if rewriteErr := m.runURLRewrite(plan, emit); rewriteErr != nil {
		return plan, rewriteErr
	}

	plan, err = m.resolveAccountID(plan, info, emit)
	if err != nil {
		return plan, err
	}

	emit(StepDialWebSocket, "dial websocket", StatusRunning, nil)

	if m.NewSession == nil {
		nilErr := errors.New("Manager.NewSession is nil — call NewManager or set it explicitly")
		emit(StepDialWebSocket, "dial websocket", StatusFailed, nilErr)

		return plan, nilErr
	}

	session, err := m.NewSession(plan.DeviceIP, info.DeviceID, plan.StepTimeout)
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

		if stepErr := st.fn(ctx); stepErr != nil {
			emit(st.kind, st.name, StatusFailed, stepErr)
			return plan, fmt.Errorf("%s: %w", st.name, stepErr)
		}

		emit(st.kind, st.name, StatusOK, nil)
	}

	if err := m.verifyPairing(plan, emit); err != nil {
		return plan, err
	}

	return plan, nil
}

// applyInitPlanDefaults validates required fields and fills in defaults
// from Manager.ServerURL / sysLanguage 2 / DefaultMargeAuthToken.
func applyInitPlanDefaults(plan InitPlan, serverURL string) (InitPlan, error) {
	if plan.DeviceIP == "" {
		return plan, errors.New("InitPlan.DeviceIP is required")
	}

	if plan.ServiceURL == "" {
		plan.ServiceURL = serverURL
	}

	if plan.ServiceURL == "" {
		return plan, errors.New("InitPlan.ServiceURL is required (and Manager.ServerURL is empty)")
	}

	if plan.Language == 0 {
		plan.Language = LanguageEnglish
	}

	if plan.AuthToken == "" {
		plan.AuthToken = DefaultMargeAuthToken
	}

	return plan, nil
}

// runURLRewrite applies the telnet envswitch URL rewrite step unless the
// caller asked to skip it.
func (m *Manager) runURLRewrite(plan InitPlan, emit func(StepKind, string, StepStatus, error)) error {
	if plan.SkipURLRewrite {
		emit(StepURLRewrite, "telnet URL rewrite", StatusSkipped, nil)
		return nil
	}

	emit(StepURLRewrite, "telnet URL rewrite", StatusRunning, nil)

	urls := defaultTelnetURLs(plan.ServiceURL)
	if _, rwErr := m.migrateViaTelnet(plan.DeviceIP, plan.ServiceURL, urls); rwErr != nil {
		emit(StepURLRewrite, "telnet URL rewrite", StatusFailed, rwErr)
		return fmt.Errorf("URL rewrite: %w", rwErr)
	}

	emit(StepURLRewrite, "telnet URL rewrite", StatusOK, nil)

	return nil
}

// resolveAccountID populates plan.AccountID — reusing the device's
// existing margeAccountUUID, generating a fresh non-colliding 7-digit
// ID, or validating a user-supplied value.
func (m *Manager) resolveAccountID(plan InitPlan, info *DeviceInfoXML, emit func(StepKind, string, StepStatus, error)) (InitPlan, error) {
	if plan.AccountID != "" {
		if !IsValidAccountID(plan.AccountID) {
			invalidErr := fmt.Errorf("invalid AccountID %q: must be exactly 7 digits", plan.AccountID)
			emit(StepGenerateAccountID, "validate account ID", StatusFailed, invalidErr)

			return plan, invalidErr
		}

		return plan, nil
	}

	if info.MargeAccountUUID != "" && IsValidAccountID(info.MargeAccountUUID) {
		plan.AccountID = info.MargeAccountUUID
		emit(StepGenerateAccountID, "reuse existing margeAccountUUID="+plan.AccountID, StatusOK, nil)

		return plan, nil
	}

	emit(StepGenerateAccountID, "generate account ID", StatusRunning, nil)

	id, genErr := GenerateAccountID(listKnownAccountIDs(m))
	if genErr != nil {
		emit(StepGenerateAccountID, "generate account ID", StatusFailed, genErr)
		return plan, fmt.Errorf("generate account ID: %w", genErr)
	}

	plan.AccountID = id

	emit(StepGenerateAccountID, "generate account ID="+id, StatusOK, nil)

	return plan, nil
}

// verifyPairing re-reads /info after the state machine finished and
// confirms the device's margeAccountUUID matches what we asked for.
func (m *Manager) verifyPairing(plan InitPlan, emit func(StepKind, string, StepStatus, error)) error {
	emit(StepVerify, "verify /info margeAccountUUID", StatusRunning, nil)

	verify, err := m.GetLiveDeviceInfo(plan.DeviceIP)
	if err != nil {
		emit(StepVerify, "verify /info", StatusFailed, err)
		return fmt.Errorf("verify /info: %w", err)
	}

	if verify.MargeAccountUUID != plan.AccountID {
		mismatchErr := fmt.Errorf("post-init /info shows margeAccountUUID=%q, want %q", verify.MargeAccountUUID, plan.AccountID)
		emit(StepVerify, "verify /info", StatusFailed, mismatchErr)

		return mismatchErr
	}

	emit(StepVerify, "verify /info margeAccountUUID="+plan.AccountID, StatusOK, nil)

	return nil
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
