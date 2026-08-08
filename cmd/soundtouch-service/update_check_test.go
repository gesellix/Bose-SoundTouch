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

func TestStartUpdateCheck_DisabledIsANoOp(t *testing.T) {
	// Must return immediately without spawning anything that could touch a
	// nil-repo Checker or block — disabled is the default, so this path
	// runs on every install that hasn't opted in.
	startUpdateCheck(nil, false, time.Hour)
}
