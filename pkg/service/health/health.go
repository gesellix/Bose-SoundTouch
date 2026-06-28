// Package health provides an extensible registry of operator-facing
// health checks for the AfterTouch service. Checks inspect datastore
// state (and, in future, speaker reachability or config) and emit
// Findings that the admin UI renders under the Health tab. Each
// Finding may carry one or more QuickFix descriptors; the
// remediation itself is dispatched through a separate fix registry
// keyed by (checkID, fixID), so the HTTP layer can never reference
// a fix that isn't actually registered.
package health

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Severity classifies a Finding's urgency. The UI sorts errors
// first, then warnings, then info. An entire check with zero
// findings is reported back at severity SeverityOK so the admin UI
// can show a positive "✓ check passed" line instead of hiding the
// check altogether.
type Severity string

// Recognised Severity values. The UI renders findings sorted with
// errors first; the registry rolls up a CheckResult's severity to
// the highest among its Findings (SeverityOK when there are none).
const (
	SeverityOK      Severity = "ok"
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Target identifies what a Finding is about. Account and Device are
// optional: a service-wide finding leaves both empty, a
// per-account finding fills Account only, and the common
// per-device case fills both. The UI displays the populated fields
// as a small label next to the finding.
//
// Name and IP are display-only conveniences for per-device findings,
// filled in centrally by EnrichTargets after the checks run (the
// checks themselves don't all have them in scope). Fixes match on
// Account+Device only, so these never affect RunFix dispatch.
type Target struct {
	Account string `json:"account,omitempty"`
	Device  string `json:"device,omitempty"`
	Name    string `json:"name,omitempty"`
	IP      string `json:"ip,omitempty"`
}

// QuickFix is a remediation a user can trigger from the UI with a
// single click. The ID is the registry key used to look up the
// FixFunc at POST time. Label is what the button displays. Confirm
// is optional UI guidance ("This will overwrite the existing
// file"); empty string means no confirmation needed.
type QuickFix struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Confirm string `json:"confirm,omitempty"`
}

// ManualCommand is a shell command the UI offers as a copy-paste
// affordance, typically when a probe couldn't reach a LAN target
// from the service host. Label is shown next to the copy button;
// Command is the verbatim string the operator should run. Hint is
// optional UI guidance ("run this on a machine on the speaker's
// network").
type ManualCommand struct {
	Label   string `json:"label"`
	Command string `json:"command"`
	Hint    string `json:"hint,omitempty"`
}

// Finding is the unit of output from a check. Severity should be
// SeverityWarning or SeverityError for findings that need
// attention; SeverityInfo is for things the operator might want to
// notice but doesn't need to act on. Target locates the finding
// (per-device / per-account / service-wide). QuickFixes and
// ManualCommands are both optional; the former drive
// /setup/health/fix calls, the latter render as copy-paste blocks.
type Finding struct {
	Severity       Severity        `json:"severity"`
	Target         Target          `json:"target"`
	Message        string          `json:"message"`
	Details        string          `json:"details,omitempty"`
	QuickFixes     []QuickFix      `json:"quickFixes,omitempty"`
	ManualCommands []ManualCommand `json:"manualCommands,omitempty"`
}

// RunFunc executes a check and returns its Findings. Returning a
// nil or empty slice means the check passed; the registry will
// then report severity SeverityOK for the check as a whole.
type RunFunc func() []Finding

// Check describes a registered check. ID is the stable identifier
// used in API responses and in the FixFunc registry. Title is the
// human-readable label shown in the UI.
type Check struct {
	ID    string
	Title string
	Run   RunFunc
}

// FixFunc executes a quick-fix on the given target and returns an
// optional user-facing message describing what was done. An
// error result is propagated to the UI as a fix failure.
type FixFunc func(target Target) (string, error)

// CheckResult is the per-check entry in the GET /setup/health
// response. Severity rolls up from the contained Findings: error
// > warning > info; with no findings the severity is SeverityOK.
type CheckResult struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Severity Severity  `json:"severity"`
	Findings []Finding `json:"findings"`
}

// ErrFixNotFound is returned by RunFix when no FixFunc is
// registered under the (checkID, fixID) pair.
var ErrFixNotFound = errors.New("quick fix not registered")

// fixEntry pairs a FixFunc with its refresh policy. refresh=true
// means the UI should re-run fetchHealth after the fix succeeds so
// resolved findings disappear from the list. refresh=false is used
// for persistent affordances (e.g. play_ding) that never change check
// state — no re-render is needed and the brief "Loading…" flash is
// avoided.
type fixEntry struct {
	fn      FixFunc
	refresh bool
}

// Registry owns the set of checks and fixes for one Server
// instance. The default zero value is not usable; construct via
// NewRegistry.
type Registry struct {
	mu     sync.RWMutex
	checks []Check
	fixes  map[string]fixEntry // key: "<checkID>/<fixID>"
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{fixes: map[string]fixEntry{}}
}

// Register adds a check to the registry. Duplicate IDs replace
// the prior entry; this is intentional so tests can override a
// built-in check with a stub.
func (r *Registry) Register(c Check) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.checks {
		if r.checks[i].ID == c.ID {
			r.checks[i] = c
			return
		}
	}

	r.checks = append(r.checks, c)
}

// RegisterFix associates a FixFunc with the given (checkID, fixID)
// pair. A QuickFix with that ID can be advertised by any Finding
// emitted by the matching check. After a successful run the UI will
// re-fetch health so resolved findings disappear from the list.
func (r *Registry) RegisterFix(checkID, fixID string, fn FixFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fixes[fixKey(checkID, fixID)] = fixEntry{fn: fn, refresh: true}
}

// RegisterFixNoRefresh is like RegisterFix but signals the UI that
// re-fetching health after the fix runs is unnecessary. Use this for
// persistent operator affordances (e.g. play_ding) whose success
// doesn't change any check state — skipping the re-fetch avoids a
// distracting "Loading…" flash with no benefit.
func (r *Registry) RegisterFixNoRefresh(checkID, fixID string, fn FixFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fixes[fixKey(checkID, fixID)] = fixEntry{fn: fn, refresh: false}
}

// RunAll executes every registered check and returns the results
// in registration order. Each result's Severity is the highest
// severity among its Findings, or SeverityOK when there are none.
func (r *Registry) RunAll() []CheckResult {
	r.mu.RLock()
	checks := make([]Check, len(r.checks))
	copy(checks, r.checks)
	r.mu.RUnlock()

	out := make([]CheckResult, 0, len(checks))

	for _, c := range checks {
		findings := []Finding{}
		if c.Run != nil {
			findings = c.Run()
		}

		out = append(out, CheckResult{
			ID:       c.ID,
			Title:    c.Title,
			Severity: rollupSeverity(findings),
			Findings: sortFindings(findings),
		})
	}

	return out
}

// EnrichTargets fills the display-only Name and IP on every finding
// Target that carries a Device but doesn't already have them, using
// resolve. resolve should return ("", "") for an unknown device, which
// leaves the Target unchanged. A nil resolve is a no-op. This is a
// post-processing pass so individual checks don't each have to look up
// the device record just to label their findings.
func EnrichTargets(results []CheckResult, resolve func(deviceID string) (name, ip string)) {
	if resolve == nil {
		return
	}

	for i := range results {
		for j := range results[i].Findings {
			t := &results[i].Findings[j].Target
			if t.Device == "" || (t.Name != "" && t.IP != "") {
				continue
			}

			name, ip := resolve(t.Device)
			if t.Name == "" {
				t.Name = name
			}

			if t.IP == "" {
				t.IP = ip
			}
		}
	}
}

// RunFix dispatches to the FixFunc registered for (checkID, fixID).
// Returns the user-facing success message, whether the UI should
// re-fetch health afterwards, and any execution error.
// ErrFixNotFound is returned when no fix is registered.
func (r *Registry) RunFix(checkID, fixID string, target Target) (string, bool, error) {
	r.mu.RLock()
	entry, ok := r.fixes[fixKey(checkID, fixID)]
	r.mu.RUnlock()

	if !ok {
		return "", false, fmt.Errorf("%w: %s/%s", ErrFixNotFound, checkID, fixID)
	}

	msg, err := entry.fn(target)

	return msg, entry.refresh, err
}

func fixKey(checkID, fixID string) string {
	return checkID + "/" + fixID
}

func rollupSeverity(findings []Finding) Severity {
	if len(findings) == 0 {
		return SeverityOK
	}

	rank := map[Severity]int{
		SeverityOK:      0,
		SeverityInfo:    1,
		SeverityWarning: 2,
		SeverityError:   3,
	}

	worst := SeverityInfo
	for i := range findings {
		if rank[findings[i].Severity] > rank[worst] {
			worst = findings[i].Severity
		}
	}

	return worst
}

func sortFindings(findings []Finding) []Finding {
	if len(findings) < 2 {
		return findings
	}

	out := make([]Finding, len(findings))
	copy(out, findings)

	rank := map[Severity]int{
		SeverityError:   0,
		SeverityWarning: 1,
		SeverityInfo:    2,
		SeverityOK:      3,
	}

	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}

		if out[i].Target.Account != out[j].Target.Account {
			return out[i].Target.Account < out[j].Target.Account
		}

		return out[i].Target.Device < out[j].Target.Device
	})

	return out
}
