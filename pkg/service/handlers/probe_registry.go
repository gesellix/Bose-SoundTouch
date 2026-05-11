package handlers

import "sync"

// DEPRECATED: probeRegistry is scheduled for removal. The active
// round-trip probe it backs cannot work without a reboot (the swUpdate
// daemon caches its URL at boot). See
// docs/analysis/TELNET-MIGRATION-METHOD.md §9.8. Replaced by
// peerObserver (peer_observer.go) which keys on device IP and supports
// passive post-migration reachability checks.

// probeRegistry is the rendezvous between the telnet round-trip probe
// orchestrator (which registers a one-shot token and waits for an
// inbound) and the /probe/{token}/* HTTP handler (which closes the
// matching channel when the speaker's swUpdateCheck fan-out lands).
type probeRegistry struct {
	mu      sync.Mutex
	pending map[string]chan struct{}
}

func newProbeRegistry() *probeRegistry {
	return &probeRegistry{pending: make(map[string]chan struct{})}
}

// Register creates a one-shot channel keyed by token. The caller waits
// on the returned channel for the matching inbound; the channel is
// closed by Signal. Must be paired with Forget to release the entry.
func (r *probeRegistry) Register(token string) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan struct{})
	r.pending[token] = ch

	return ch
}

// Signal closes the channel for token (idempotent — repeated hits on
// the same probe path are tolerated, the device sometimes retries).
// Returns true when a matching registration existed.
func (r *probeRegistry) Signal(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, ok := r.pending[token]
	if !ok {
		return false
	}

	select {
	case <-ch:
		// already closed; nothing to do
	default:
		close(ch)
	}

	return true
}

// Forget removes the entry. Safe to call after Register's channel has
// been closed (or never signalled); does not affect already-returned
// channels.
func (r *probeRegistry) Forget(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.pending, token)
}
