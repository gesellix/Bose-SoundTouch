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

	msg, err := r.RunFix("c1", "f1", Target{Account: "A", Device: "D"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg != "applied" {
		t.Errorf("unexpected message: %q", msg)
	}

	if captured.Account != "A" || captured.Device != "D" {
		t.Errorf("target not propagated to fix: %+v", captured)
	}
}

func TestRegistry_RunFix_NotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.RunFix("nope", "also-nope", Target{})
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
