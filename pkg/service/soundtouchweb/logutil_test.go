package soundtouchweb

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestIsErrorSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"INVALID_SOURCE", true},
		{"UNKNOWN_SOURCE_ERROR", true},
		{"INTERNET_RADIO_ERROR", true},
		{"STANDBY", false},
		{"TUNEIN", false},
		{"AUX", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isErrorSource(tt.source); got != tt.want {
			t.Errorf("isErrorSource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}

// captureLog redirects the standard logger for the duration of f and returns
// what was written.
func captureLog(t *testing.T, f func()) string {
	t.Helper()

	var buf bytes.Buffer

	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)

	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	f()

	return buf.String()
}

func TestLogPlaybackRequest(t *testing.T) {
	out := captureLog(t, func() {
		logPlaybackRequest("source-select", "DEVICEID01", "AUX", "AUX1", "", "")
	})

	for _, want := range []string{`[play]`, `source-select`, `device="DEVICEID01"`, `source="AUX"`, `sourceAccount="AUX1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output %q missing %q", out, want)
		}
	}
}

func TestLogPlaybackRequest_SanitizesNewlines(t *testing.T) {
	out := captureLog(t, func() {
		logPlaybackRequest("device-play", "DEVICEID01", "TUNEIN", "", "http://evil/\nINJECTED line", "Station")
	})

	if strings.Contains(out, "\nINJECTED") {
		t.Errorf("log output not sanitized against newline injection: %q", out)
	}
	if !strings.Contains(out, `\nINJECTED`) {
		t.Errorf("expected escaped newline in output, got %q", out)
	}
}

func TestLogNowPlayingError(t *testing.T) {
	out := captureLog(t, func() {
		logNowPlayingError("DEVICEID01", "INVALID_SOURCE", "")
	})

	for _, want := range []string{`now_playing entered error`, `source="INVALID_SOURCE"`, `device="DEVICEID01"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output %q missing %q", out, want)
		}
	}
}

func TestLogZoneVolumePartial(t *testing.T) {
	out := captureLog(t, func() {
		logZoneVolumePartial(zoneVolumeResult{
			MasterDeviceID: "MASTER",
			Requested:      42,
			Partial:        true,
			Members: []zoneVolumeMemberResult{
				{ControlID: "192.0.2.10", Error: "readback volume: timeout"},
				{ControlID: "192.0.2.20", Error: "read volume: failed\nINJECTED"},
			},
		})
	})

	for _, want := range []string{
		`[zone-volume] partial`,
		`master="MASTER"`,
		`requested=42`,
		`192.0.2.10=readback volume: timeout`,
		`192.0.2.20=read volume: failed\\nINJECTED`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output %q missing %q", out, want)
		}
	}
	if strings.Contains(out, "failed\nINJECTED") {
		t.Errorf("log output not sanitized against newline injection: %q", out)
	}
}

func TestLogZoneVolumeSuccessIsQuiet(t *testing.T) {
	out := captureLog(t, func() {
		logZoneVolumePartial(zoneVolumeResult{MasterDeviceID: "MASTER", Requested: 42})
	})

	if out != "" {
		t.Fatalf("successful group-volume update logged: %q", out)
	}
}
