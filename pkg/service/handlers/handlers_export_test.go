package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestParseSpeakerLogTime(t *testing.T) {
	currentYear := 2025

	cases := []struct {
		line    string
		wantOK  bool
		wantSub string // substring that should appear in formatted result
	}{
		{"Wed Jun  4 12:34:56 2025 daemon.info app: hello", true, "2025"},
		{"Mon Jan 02 15:04:05 2025 kern.info kernel: boot", true, "2025"},
		{"Wed Jun  4 12:34:56 daemon.info app: no year", true, "2025"}, // year injected
		{"not a log line at all", false, ""},
		{"", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			got, ok := parseSpeakerLogTime(tc.line, currentYear)
			if ok != tc.wantOK {
				t.Errorf("parseSpeakerLogTime(%q) ok=%v, want %v", tc.line, ok, tc.wantOK)
			}

			if tc.wantOK && tc.wantSub != "" && !strings.Contains(got.Format(time.RFC3339), tc.wantSub) {
				t.Errorf("parseSpeakerLogTime(%q) = %v, expected to contain %q", tc.line, got.Format(time.RFC3339), tc.wantSub)
			}
		})
	}
}

func TestFilterSpeakerLog(t *testing.T) {
	now := time.Now().UTC()
	format := "Mon Jan _2 15:04:05 2006"

	recent := now.Add(-5 * time.Minute).Format(format)
	old := now.Add(-30 * time.Minute).Format(format)

	rawLog := strings.Join([]string{
		old + " kern.info kernel: old message",
		recent + " daemon.info app: recent message",
		"unparseable line — keep it",
		"",
	}, "\n")

	filtered := filterSpeakerLog(rawLog, 20*time.Minute)

	if strings.Contains(filtered, "old message") {
		t.Error("filtered log should not contain old message")
	}

	if !strings.Contains(filtered, "recent message") {
		t.Error("filtered log should contain recent message")
	}

	if !strings.Contains(filtered, "unparseable line") {
		t.Error("filtered log should keep unparseable lines (fail-open)")
	}
}
