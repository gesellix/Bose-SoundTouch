package handlers

import (
	"sync"
	"time"
)

// PeerHit is the payload delivered to a waiter when a request from a
// registered peer IP lands on the service. Path is the chi URL path of
// the inbound; At is the wall-clock time the middleware saw it.
type PeerHit struct {
	Path string
	At   time.Time
}

// peerObserver is the rendezvous between the passive reachability probe
// (which registers interest in a device IP and waits for any inbound)
// and the chi middleware (which signals on every request whose source
// IP matches a registration).
//
// Unlike probeRegistry, which keys on a unique per-probe token, this
// observer keys on the device's IP — the probe doesn't mutate device
// state, so there's no token to thread through the request path. Any
// inbound from the IP counts as proof of reachability.
type peerObserver struct {
	mu      sync.Mutex
	pending map[string]chan PeerHit
}

func newPeerObserver() *peerObserver {
	return &peerObserver{pending: make(map[string]chan PeerHit)}
}

// Register creates a one-shot buffered channel keyed by IP. The buffer
// of 1 lets the middleware deliver the first hit and silently drop
// subsequent hits during the wait window without blocking. Caller is
// responsible for pairing every Register with Forget.
func (o *peerObserver) Register(ip string) <-chan PeerHit {
	o.mu.Lock()
	defer o.mu.Unlock()

	ch := make(chan PeerHit, 1)
	o.pending[ip] = ch

	return ch
}

// Signal delivers a hit to the channel for ip, non-blocking. Returns
// true when a matching registration existed AND the hit was delivered
// (i.e. the channel had buffer space — first hit during the window).
// Subsequent hits during the same window return false without blocking.
func (o *peerObserver) Signal(ip string, hit PeerHit) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	ch, ok := o.pending[ip]
	if !ok {
		return false
	}

	select {
	case ch <- hit:
		return true
	default:
		return false
	}
}

// Forget removes the entry. Safe to call regardless of whether a hit
// landed — does not affect already-returned channels.
func (o *peerObserver) Forget(ip string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.pending, ip)
}