package health

import (
	"errors"
	"testing"
)

func TestRegistry_RunAll_NoFindingsReturnsOK(t *testing.T) {
	r := NewRegistry()
	r.Register(Check{
		ID:    "passes",
		Title: "Always passes",
		Run:   func() []Finding { return nil },
	})

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Severity != SeverityOK {
		t.Errorf("expected SeverityOK for empty findings, got %q", results[0].Severity)
	}

	if len(results[0].Findings) != 0 {
		t.Errorf("expected zero findings, got %d", len(results[0].Findings))
	}
}

func TestRegistry_RunAll_SeverityRollup(t *testing.T) {
	r := NewRegistry()
	r.Register(Check{
		ID: "mixed",
		Run: func() []Finding {
			return []Finding{
				{Severity: SeverityInfo, Message: "info"},
				{Severity: SeverityWarning, Message: "warn"},
				{Severity: SeverityError, Message: "err"},
			}
		},
	})

	results := r.RunAll()
	if results[0].Severity != SeverityError {
		t.Errorf("expected SeverityError rollup, got %q", results[0].Severity)
	}

	if results[0].Findings[0].Severity != SeverityError {
		t.Errorf("expected error to sort first, got %q", results[0].Findings[0].Severity)
	}
}

func TestRegistry_RunFix_Dispatch(t *testing.T) {
	r := NewRegistry()
	var captured Target
	r.RegisterFix("c1", "f1", func(t Target) (string, error) {
		captured = t
		return "applied", nil
	})

	msg, refresh, err := r.RunFix("c1", "f1", Target{Account: "A", Device: "D"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg != "applied" {
		t.Errorf("unexpected message: %q", msg)
	}

	if !refresh {
		t.Errorf("expected refresh=true for a fix registered via RegisterFix")
	}

	if captured.Account != "A" || captured.Device != "D" {
		t.Errorf("target not propagated to fix: %+v", captured)
	}
}

func TestRegistry_RunFix_NotFound(t *testing.T) {
	r := NewRegistry()

	_, _, err := r.RunFix("nope", "also-nope", Target{})
	if !errors.Is(err, ErrFixNotFound) {
		t.Errorf("expected ErrFixNotFound, got %v", err)
	}
}

func TestRegistry_Register_ReplacesByID(t *testing.T) {
	r := NewRegistry()
	r.Register(Check{ID: "x", Title: "first", Run: func() []Finding { return nil }})
	r.Register(Check{ID: "x", Title: "second", Run: func() []Finding { return nil }})

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 check after replace, got %d", len(results))
	}

	if results[0].Title != "second" {
		t.Errorf("expected title to be replaced, got %q", results[0].Title)
	}
}

func TestEnrichTargets_FillsNameAndIPForDeviceFindings(t *testing.T) {
	results := []CheckResult{{
		ID: "c1",
		Findings: []Finding{
			{Target: Target{Account: "3230304", Device: "08DF1F0BA325"}}, // per-device: enriched
			{Target: Target{Account: "3230304"}},                         // account-only: untouched
			{Target: Target{}},                                           // service-wide: untouched
			{Target: Target{Device: "UNKNOWNDEV"}},                       // not in resolver: left as-is
		},
	}}

	resolve := func(deviceID string) (string, string) {
		if deviceID == "08DF1F0BA325" {
			return "Cantina", "192.0.2.9"
		}
		return "", ""
	}

	EnrichTargets(results, resolve)

	got := results[0].Findings
	if got[0].Target.Name != "Cantina" || got[0].Target.IP != "192.0.2.9" {
		t.Errorf("per-device finding not enriched: %+v", got[0].Target)
	}
	if got[1].Target.Name != "" || got[1].Target.IP != "" {
		t.Errorf("account-only finding should be untouched: %+v", got[1].Target)
	}
	if got[2].Target.Name != "" || got[2].Target.IP != "" {
		t.Errorf("service-wide finding should be untouched: %+v", got[2].Target)
	}
	if got[3].Target.Name != "" || got[3].Target.IP != "" {
		t.Errorf("unknown device should be left as-is: %+v", got[3].Target)
	}
}

func TestEnrichTargets_NilResolveIsNoOp(t *testing.T) {
	results := []CheckResult{{Findings: []Finding{{Target: Target{Device: "D1"}}}}}
	EnrichTargets(results, nil) // must not panic
	if results[0].Findings[0].Target.Name != "" {
		t.Error("nil resolve should be a no-op")
	}
}
