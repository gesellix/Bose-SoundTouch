package handlers

import (
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

func TestPeerObserver_RegisterSignalForget(t *testing.T) {
	o := newPeerObserver()

	ch := o.Register("192.168.1.42")
	if ch == nil {
		t.Fatal("Register returned nil channel")
	}

	first := setup.PeerHit{Path: "/updates/soundtouch", At: time.Now()}
	if !o.Signal("192.168.1.42", first) {
		t.Error("Signal returned false for registered IP")
	}

	// Second signal while the buffer is still full (no reader yet) drops
	// silently and returns false — only the first hit per window matters.
	if o.Signal("192.168.1.42", setup.PeerHit{Path: "/streaming/x"}) {
		t.Error("second Signal returned true; expected false (buffer full, undrained)")
	}

	select {
	case got := <-ch:
		if got.Path != first.Path {
			t.Errorf("hit.Path = %q, want %q", got.Path, first.Path)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Signal did not deliver hit to channel")
	}

	o.Forget("192.168.1.42")

	// After Forget, Signal returns false.
	if o.Signal("192.168.1.42", first) {
		t.Error("Signal returned true after Forget")
	}
}

func TestPeerObserver_UnknownIP(t *testing.T) {
	o := newPeerObserver()
	if o.Signal("10.0.0.1", setup.PeerHit{Path: "/anything"}) {
		t.Error("Signal returned true for unregistered IP")
	}
}

func TestPeerObserver_SignalIsNonBlocking(t *testing.T) {
	o := newPeerObserver()
	o.Register("192.168.1.42") // never drain

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			o.Signal("192.168.1.42", setup.PeerHit{Path: "/x"})
		}
		close(done)
	}()

	select {
	case <-done:
		// Signal never blocked even with no reader and a full buffer.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Signal blocked when buffer was full — must drop silently")
	}
}
