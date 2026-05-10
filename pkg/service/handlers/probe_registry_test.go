package handlers

import (
	"testing"
	"time"
)

func TestProbeRegistry_RegisterSignalForget(t *testing.T) {
	r := newProbeRegistry()

	ch := r.Register("abc123")
	if ch == nil {
		t.Fatal("Register returned nil channel")
	}

	if !r.Signal("abc123") {
		t.Error("Signal returned false for registered token")
	}

	select {
	case <-ch:
		// channel closed as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Signal did not close the channel")
	}

	// Signal again on the same token must be idempotent (no panic on
	// double close).
	if !r.Signal("abc123") {
		t.Error("second Signal returned false")
	}

	r.Forget("abc123")

	// After Forget, Signal returns false.
	if r.Signal("abc123") {
		t.Error("Signal returned true after Forget")
	}
}

func TestProbeRegistry_UnknownToken(t *testing.T) {
	r := newProbeRegistry()
	if r.Signal("never-registered") {
		t.Error("Signal returned true for unregistered token")
	}
}
