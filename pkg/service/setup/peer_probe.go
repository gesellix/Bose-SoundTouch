package setup

import (
	"errors"
	"fmt"
	"time"
)

// PeerHit is the payload the observer middleware delivers to a probe
// waiter when a request from a registered peer IP lands on the service.
type PeerHit struct {
	Path string
	At   time.Time
}

// PeerObserverHandle is the abstract view of the peer-observer registry
// the probe needs: register interest in an IP, eventually forget it.
// The handlers package's peerObserver satisfies this implicitly.
type PeerObserverHandle interface {
	Register(ip string) <-chan PeerHit
	Forget(ip string)
}

// PeerProbeResult is the JSON-serializable outcome of a passive
// reachability probe. Reached is the canonical success bit the UI keys
// off; ObservedPath and ElapsedMs are diagnostic.
type PeerProbeResult struct {
	Reached      bool   `json:"reached"`
	ObservedPath string `json:"observed_path,omitempty"`
	ElapsedMs    int64  `json:"elapsed_ms"`
}

// RunPeerReachabilityProbe is the post-migration reachability check
// that replaces the active swUpdateUrl round-trip. The sequence:
//
//  1. Register the device IP with the observer.
//  2. Nudge :8090/swUpdateCheck on the device to make the swUpdate
//     daemon fan out *something* sooner than its own ~5min timer.
//  3. Wait up to timeout for any inbound from that IP.
//
// Any inbound counts as proof of reachability — on a migrated speaker,
// DNS interception means the daemon's outbounds (update fan-out, marge
// polls, BMX registry calls) all funnel through this service regardless
// of which URL the daemon resolved internally. We don't need a specific
// URL to land; we just need *the device* to dial us.
//
// The nudge is fire-and-forget. If :8090 is unreachable, the request
// returns quickly and we still wait for the daemon's own next fan-out
// (or time out). No state on the device is mutated; the probe is safe
// to re-run.
func (m *Manager) RunPeerReachabilityProbe(deviceIP string, observer PeerObserverHandle, timeout time.Duration) (*PeerProbeResult, error) {
	if observer == nil {
		return nil, errors.New("peer probe not configured: observer is nil")
	}

	if deviceIP == "" {
		return nil, errors.New("peer probe: deviceIP is required")
	}

	hitCh := observer.Register(deviceIP)
	defer observer.Forget(deviceIP)

	// Nudge the device. Fire-and-forget — we don't gate on the response
	// because the swUpdateCheck endpoint returns immediately after
	// enqueuing, and the daemon's fan-out is what we actually want to
	// observe. HTTPGet can be nil in test contexts.
	if m.HTTPGet != nil {
		swCheckURL := fmt.Sprintf("http://%s:8090/swUpdateCheck", deviceIP)

		go func() {
			resp, err := m.HTTPGet(swCheckURL)
			if err != nil {
				return
			}

			_ = resp.Body.Close()
		}()
	}

	start := time.Now()
	result := &PeerProbeResult{}

	select {
	case hit := <-hitCh:
		result.Reached = true
		result.ObservedPath = hit.Path
	case <-time.After(timeout):
		result.Reached = false
	}

	result.ElapsedMs = time.Since(start).Milliseconds()

	return result, nil
}
