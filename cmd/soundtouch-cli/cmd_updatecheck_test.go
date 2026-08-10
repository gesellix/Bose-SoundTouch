package main

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/updatecheck"
)

// TestUpdateCheckCommand_Registered checks the command is wired up with the
// expected name and an Action, without making any real GitHub API calls.
func TestUpdateCheckCommand_Registered(t *testing.T) {
	cmd := updateCheckCommand()

	if cmd.Name != "update-check" {
		t.Errorf("command name = %q; want %q", cmd.Name, "update-check")
	}

	if cmd.Action == nil {
		t.Error("expected an Action to be set")
	}
}

// TestPrintUpdateCheckResult_DoesNotPanic exercises all three result shapes
// (unparseable current version, update available, up to date) purely for
// the "does not panic" guarantee; updatecheck.Checker's own tests already
// cover the comparison logic itself.
func TestPrintUpdateCheckResult_DoesNotPanic(t *testing.T) {
	cases := []struct {
		name   string
		result updatecheck.Result
	}{
		{"unparseable current version", updatecheck.Result{CurrentVersion: "dev"}},
		{"update available", updatecheck.Result{CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", Available: true, ReleaseURL: "https://example.invalid"}},
		{"up to date", updatecheck.Result{CurrentVersion: "v1.1.0", LatestVersion: "v1.1.0", Available: false}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			printUpdateCheckResult(tc.result)
		})
	}
}
