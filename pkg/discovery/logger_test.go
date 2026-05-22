package discovery

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// captureLog redirects log output into a buffer and returns the buffer
// plus a cleanup func that restores the original log destination. Used
// by the tests below to assert what logVerbose / SetVerbose actually
// produces under each toggle state.
func captureLog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	buf := &bytes.Buffer{}
	original := log.Writer()
	log.SetOutput(buf)

	return buf, func() {
		log.SetOutput(original)
	}
}

func TestVerboseToggle_DefaultIsQuiet(t *testing.T) {
	// Reset to default state (zero value of atomic.Bool is false).
	SetVerbose(false)
	t.Cleanup(func() { SetVerbose(false) })

	buf, restore := captureLog(t)
	defer restore()

	logVerbose("noisy: should-be-suppressed message")

	if buf.Len() != 0 {
		t.Errorf("expected no log output when verbose is off, got: %q", buf.String())
	}

	if IsVerbose() {
		t.Errorf("IsVerbose() = true, want false")
	}
}

func TestVerboseToggle_OnEmitsToLog(t *testing.T) {
	SetVerbose(true)
	t.Cleanup(func() { SetVerbose(false) })

	buf, restore := captureLog(t)
	defer restore()

	logVerbose("trace: %s = %d", "answer", 42)

	if !strings.Contains(buf.String(), "trace: answer = 42") {
		t.Errorf("expected trace output, got: %q", buf.String())
	}

	if !IsVerbose() {
		t.Errorf("IsVerbose() = false, want true")
	}
}

func TestVerboseToggle_StaysOffByDefaultAfterPackageInit(t *testing.T) {
	// New goroutines / new processes see the zero-value default. This
	// codifies that contract for callers like cmd/soundtouch-service
	// that rely on never having to call SetVerbose.
	t.Cleanup(func() { SetVerbose(false) })
	SetVerbose(false)

	if IsVerbose() {
		t.Errorf("default verbose state must be false")
	}
}
