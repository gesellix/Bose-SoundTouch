package main

import (
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/updatecheck"
)

func TestShouldCheckImmediately(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	interval := 24 * time.Hour

	cases := []struct {
		name          string
		lastCheckedAt time.Time
		want          bool
	}{
		{"never checked", time.Time{}, true},
		{"stale (older than interval)", now.Add(-25 * time.Hour), true},
		{"exactly one interval ago", now.Add(-interval), true},
		{"recent (within interval)", now.Add(-1 * time.Hour), false},
	}

	for _, tc := range cases {
		if got := shouldCheckImmediately(tc.lastCheckedAt, interval, now); got != tc.want {
			t.Errorf("%s: shouldCheckImmediately() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestShouldSkipDueToBackoff(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		lastErrorAt time.Time
		want        bool
	}{
		{"no recent failure", time.Time{}, false},
		{"failed 30 minutes ago", now.Add(-30 * time.Minute), true},
		{"failed exactly 1 hour ago", now.Add(-time.Hour), false},
		{"failed 2 hours ago", now.Add(-2 * time.Hour), false},
	}

	for _, tc := range cases {
		if got := shouldSkipDueToBackoff(tc.lastErrorAt, now); got != tc.want {
			t.Errorf("%s: shouldSkipDueToBackoff() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLogUpdateIfNewlyAvailable(t *testing.T) {
	cases := []struct {
		name              string
		result            updatecheck.Result
		lastLoggedVersion string
		want              string
	}{
		{
			name:              "nothing available",
			result:            updatecheck.Result{Available: false},
			lastLoggedVersion: "",
			want:              "",
		},
		{
			name:              "newly available",
			result:            updatecheck.Result{Available: true, LatestVersion: "v1.1.0"},
			lastLoggedVersion: "",
			want:              "v1.1.0",
		},
		{
			name:              "already logged this version",
			result:            updatecheck.Result{Available: true, LatestVersion: "v1.1.0"},
			lastLoggedVersion: "v1.1.0",
			want:              "v1.1.0",
		},
		{
			name:              "a newer version than what was logged",
			result:            updatecheck.Result{Available: true, LatestVersion: "v1.2.0"},
			lastLoggedVersion: "v1.1.0",
			want:              "v1.2.0",
		},
	}

	for _, tc := range cases {
		if got := logUpdateIfNewlyAvailable(tc.result, tc.lastLoggedVersion); got != tc.want {
			t.Errorf("%s: logUpdateIfNewlyAvailable() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRandomJitter(t *testing.T) {
	if got := randomJitter(0); got != 0 {
		t.Errorf("randomJitter(0) = %v, want 0", got)
	}

	upperBound := 5 * time.Minute
	for i := 0; i < 20; i++ {
		got := randomJitter(upperBound)
		if got < 0 || got >= upperBound {
			t.Fatalf("randomJitter(%v) = %v, want in [0, %v)", upperBound, got, upperBound)
		}
	}
}

// There is deliberately no test for startUpdateCheck itself, matching
// startDeviceDiscovery (its equally untested sibling): both are thin,
// forever-looping goroutine wrappers whose only decisions live in pure
// helpers, which is what the tests above and below cover. The former
// TestStartUpdateCheck_DisabledIsANoOp asserted a contract that no longer
// exists — the goroutine now always starts, precisely so that enabling the
// check from the Settings page takes effect without a restart, and an
// early return for "disabled" would defeat that.
func TestShouldRunUpdateCheckNow(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	interval := 24 * time.Hour
	stale := now.Add(-25 * time.Hour)
	fresh := now.Add(-1 * time.Hour)

	cases := []struct {
		name          string
		enabled       bool
		lastCheckedAt time.Time
		interval      time.Duration
		lastErrorAt   time.Time
		want          bool
	}{
		{"disabled, never checked", false, time.Time{}, interval, time.Time{}, false},
		{"disabled, due", false, stale, interval, time.Time{}, false},
		{"enabled, never checked", true, time.Time{}, interval, time.Time{}, true},
		{"enabled, due", true, stale, interval, time.Time{}, true},
		{"enabled, not due yet", true, fresh, interval, time.Time{}, false},
		{"enabled and due, but in error backoff", true, stale, interval, now.Add(-30 * time.Minute), false},
		{"enabled and due, backoff expired", true, stale, interval, now.Add(-2 * time.Hour), true},
		// A zero interval must not turn every poll tick into a GitHub request.
		{"enabled with a zero interval", true, stale, 0, time.Time{}, false},
	}

	for _, tc := range cases {
		got := shouldRunUpdateCheckNow(tc.enabled, tc.lastCheckedAt, tc.interval, tc.lastErrorAt, now)
		if got != tc.want {
			t.Errorf("%s: shouldRunUpdateCheckNow() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestUpdateCheckPollTickIsShorterThanTheDefaultInterval guards the property
// that makes the Settings-page toggle feel live: the goroutine must re-read
// the settings far more often than the check interval itself, otherwise
// switching the check on would appear to do nothing for up to a day.
func TestUpdateCheckPollTickIsShorterThanTheDefaultInterval(t *testing.T) {
	if updateCheckPollTick >= 24*time.Hour {
		t.Errorf("updateCheckPollTick = %v, want well below the 24h default interval", updateCheckPollTick)
	}
}
