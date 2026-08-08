package handlers

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/updatecheck"
)

// TestUpdateCheckResult_NilCheckerIsSafe verifies the default (opt-in
// checker never registered) returns a safe zero value rather than
// panicking — the common case, since UPDATE_CHECK_ENABLED defaults to
// false.
func TestUpdateCheckResult_NilCheckerIsSafe(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)

	result := s.UpdateCheckResult()
	if result.Available {
		t.Error("Expected a nil checker to report Available=false")
	}
}

// TestUpdateCheckResult_ReflectsRegisteredChecker verifies SetUpdateChecker
// wires the checker in and UpdateCheckResult reads through to it.
func TestUpdateCheckResult_ReflectsRegisteredChecker(t *testing.T) {
	s := NewServer(nil, nil, "http://localhost", false, false, false)

	checker := updatecheck.NewChecker(nil, "owner/repo", "v1.0.0")
	s.SetUpdateChecker(checker)

	result := s.UpdateCheckResult()
	if result.CurrentVersion != "v1.0.0" {
		t.Errorf("Expected UpdateCheckResult to read through to the registered checker, got %+v", result)
	}
}
