package setup

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakePeerObserver is a deterministic PeerObserverHandle for unit
// tests. It exposes the channel returned from Register so the test can
// signal it manually to simulate a device inbound landing.
type fakePeerObserver struct {
	mu        sync.Mutex
	channels  map[string]chan PeerHit
	forgotten []string
}

func newFakePeerObserver() *fakePeerObserver {
	return &fakePeerObserver{channels: map[string]chan PeerHit{}}
}

func (o *fakePeerObserver) Register(ip string) <-chan PeerHit {
	o.mu.Lock()
	defer o.mu.Unlock()
	ch := make(chan PeerHit, 1)
	o.channels[ip] = ch
	return ch
}

func (o *fakePeerObserver) Forget(ip string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.channels, ip)
	o.forgotten = append(o.forgotten, ip)
}

func (o *fakePeerObserver) signal(ip string, hit PeerHit) {
	o.mu.Lock()
	defer o.mu.Unlock()
	ch, ok := o.channels[ip]
	if !ok {
		return
	}
	select {
	case ch <- hit:
	default:
	}
}

func peerProbeManager(onTrigger func()) *Manager {
	return &Manager{
		HTTPGet: func(url string) (*http.Response, error) {
			if onTrigger != nil {
				onTrigger()
			}
			rr := httptest.NewRecorder()
			rr.WriteHeader(200)
			return rr.Result(), nil
		},
	}
}

func TestRunPeerReachabilityProbe_HappyPath(t *testing.T) {
	obs := newFakePeerObserver()

	// On nudge, simulate the device fanning out to /updates/soundtouch
	// which the middleware would signal as a hit on this IP.
	m := peerProbeManager(func() {
		obs.signal("192.0.2.42", PeerHit{Path: "/updates/soundtouch", At: time.Now()})
	})

	result, err := m.RunPeerReachabilityProbe("192.0.2.42", obs, 2*time.Second)
	if err != nil {
		t.Fatalf("RunPeerReachabilityProbe error: %v", err)
	}
	if !result.Reached {
		t.Error("Reached = false, want true")
	}
	if result.ObservedPath != "/updates/soundtouch" {
		t.Errorf("ObservedPath = %q, want %q", result.ObservedPath, "/updates/soundtouch")
	}
	if len(obs.forgotten) != 1 || obs.forgotten[0] != "192.0.2.42" {
		t.Errorf("Forget not called for IP: forgotten = %v", obs.forgotten)
	}
}

func TestRunPeerReachabilityProbe_Timeout(t *testing.T) {
	obs := newFakePeerObserver()
	m := peerProbeManager(nil) // nudge fires but device never responds

	start := time.Now()
	result, err := m.RunPeerReachabilityProbe("192.0.2.42", obs, 200*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunPeerReachabilityProbe error: %v", err)
	}
	if result.Reached {
		t.Error("Reached = true, want false (no hit)")
	}
	if elapsed < 200*time.Millisecond {
		t.Errorf("returned early after %v; expected >= 200ms timeout", elapsed)
	}
	if len(obs.forgotten) != 1 {
		t.Errorf("Forget not called after timeout: forgotten = %v", obs.forgotten)
	}
}

func TestRunPeerReachabilityProbe_NilObserver(t *testing.T) {
	m := peerProbeManager(nil)
	_, err := m.RunPeerReachabilityProbe("192.0.2.42", nil, time.Second)
	if err == nil {
		t.Error("expected error for nil observer, got nil")
	}
}

func TestRunPeerReachabilityProbe_EmptyIP(t *testing.T) {
	m := peerProbeManager(nil)
	obs := newFakePeerObserver()
	_, err := m.RunPeerReachabilityProbe("", obs, time.Second)
	if err == nil {
		t.Error("expected error for empty deviceIP, got nil")
	}
}

func TestRunPeerReachabilityProbe_NilHTTPGetTimesOut(t *testing.T) {
	// With nil HTTPGet the nudge is skipped entirely; the probe just
	// waits for the device to dial in on its own. Useful in tests and
	// in environments where the trigger isn't safe to fire.
	m := &Manager{} // HTTPGet nil
	obs := newFakePeerObserver()

	result, err := m.RunPeerReachabilityProbe("192.0.2.42", obs, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result.Reached {
		t.Error("Reached = true with no nudge and no signal")
	}
}
