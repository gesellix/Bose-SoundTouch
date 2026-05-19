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

// Target identifies what a Finding is about. Both fields are
// optional: a service-wide finding leaves both empty, a
// per-account finding fills Account only, and the common
// per-device case fills both. The UI displays the populated fields
// as a small label next to the finding.
type Target struct {
	Account string `json:"account,omitempty"`
	Device  string `json:"device,omitempty"`
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

// Finding is the unit of output from a check. Severity should be
// SeverityWarning or SeverityError for findings that need
// attention; SeverityInfo is for things the operator might want to
// notice but doesn't need to act on. Target locates the finding
// (per-device / per-account / service-wide). QuickFixes is
// optional.
type Finding struct {
	Severity   Severity   `json:"severity"`
	Target     Target     `json:"target"`
	Message    string     `json:"message"`
	Details    string     `json:"details,omitempty"`
	QuickFixes []QuickFix `json:"quickFixes,omitempty"`
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

// Registry owns the set of checks and fixes for one Server
// instance. The default zero value is not usable; construct via
// NewRegistry.
type Registry struct {
	mu     sync.RWMutex
	checks []Check
	fixes  map[string]FixFunc // key: "<checkID>/<fixID>"
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{fixes: map[string]FixFunc{}}
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
// emitted by the matching check.
func (r *Registry) RegisterFix(checkID, fixID string, fn FixFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fixes[fixKey(checkID, fixID)] = fn
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

// RunFix dispatches to the FixFunc registered for (checkID,
// fixID). The returned string is forwarded as the user-facing
// success message. ErrFixNotFound is returned when no fix is
// registered.
func (r *Registry) RunFix(checkID, fixID string, target Target) (string, error) {
	r.mu.RLock()
	fn, ok := r.fixes[fixKey(checkID, fixID)]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %s/%s", ErrFixNotFound, checkID, fixID)
	}

	return fn(target)
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
