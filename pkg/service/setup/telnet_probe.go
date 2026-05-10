package setup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ProbeRegistrar is the rendezvous between the round-trip probe
// orchestrator (which registers a token and waits) and an HTTP layer
// (which signals the channel when the device's outbound lands on the
// matching /probe/{token}/* path). The handlers package wires its
// probeRegistry into this interface.
type ProbeRegistrar interface {
	Register(token string) <-chan struct{}
	Forget(token string)
}

// TelnetProbeResult records what RunTelnetRoundTripProbe observed.
// Reached reports whether the device's outbound landed on our service
// within the configured timeout; Restored reports whether the
// temporary swUpdateUrl override was reverted to the captured
// original. The orchestrator always attempts the restore even on the
// failure path, so a Reached=false + Restored=true is the common
// "couldn't reach us, device is back to its old configuration" state.
type TelnetProbeResult struct {
	Reached     bool   `json:"reached"`
	Restored    bool   `json:"restored"`
	OriginalURL string `json:"original_url,omitempty"`
	ProbeURL    string `json:"probe_url,omitempty"`
	ElapsedMs   int64  `json:"elapsed_ms"`
	Logs        string `json:"logs,omitempty"`
}

// generateProbeToken returns a random hex token suitable for use in a
// URL path. 12 bytes → 24 hex chars; collision probability is
// negligible for the dozens-of-probes-per-session scope.
func generateProbeToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

// RunTelnetRoundTripProbe is the SSH-less reachability check that
// fills the gap the curl-from-device HTTPS test leaves on USB-
// unlock-refusing speakers. The sequence:
//
//  1. Telnet `getpdo CurrentSystemConfiguration` to capture the
//     speaker's current swUpdateUrl.
//  2. Generate a token, register a one-shot signal channel under it.
//  3. Telnet `sys configuration swUpdateUrl <targetURL>/probe/<token>`
//     to point the runtime layer at our service. Deliberately NOT
//     `envswitch boseurls set …` — the persistence layer keeps the
//     original, so a reboot heals the device naturally if our
//     restore step fails.
//  4. HTTP GET `<deviceIP>:8090/swUpdateCheck` to make the speaker
//     fan out a request to the new swUpdateUrl.
//  5. Wait on the registered channel up to timeout.
//  6. Telnet `sys configuration swUpdateUrl <originalURL>` to revert.
//
// Returns Reached=true only if the inbound landed before the timeout
// fired. Restore runs in a deferred call so it executes even when
// earlier steps fail.
func (m *Manager) RunTelnetRoundTripProbe(deviceIP, targetURL string, registrar ProbeRegistrar, timeout time.Duration) (*TelnetProbeResult, error) {
	if m.NewTelnet == nil {
		return nil, errors.New("telnet probe not configured: Manager.NewTelnet is nil")
	}

	if registrar == nil {
		return nil, errors.New("telnet probe not configured: registrar is nil")
	}

	parsedTarget, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsedTarget.Host == "" {
		return nil, fmt.Errorf("invalid target URL %q: hostname required", targetURL)
	}

	result := &TelnetProbeResult{}

	var logs strings.Builder

	t := m.NewTelnet(deviceIP)
	if dialErr := t.Dial(); dialErr != nil {
		return nil, fmt.Errorf("telnet dial %s:17000 failed: %w", deviceIP, dialErr)
	}

	defer func() { _ = t.Close() }()

	// 1. Capture the current swUpdateUrl from getpdo. If the device
	// refuses getpdo we cannot safely flip the URL — abort.
	verify, err := t.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil {
		return nil, fmt.Errorf("getpdo CurrentSystemConfiguration failed: %w", err)
	}

	if isCommandNotFound(verify) {
		return nil, errors.New("device rejected getpdo CurrentSystemConfiguration — cannot capture original URL")
	}

	parsed := parseGetpdoConfig(verify)

	originalURL := parsed["swUpdateUrl"]
	if originalURL == "" {
		return nil, errors.New("could not parse original swUpdateUrl from getpdo response")
	}

	result.OriginalURL = originalURL
	fmt.Fprintf(&logs, "Original swUpdateUrl: %s\n", originalURL)

	// 2. Token + registration.
	token, err := generateProbeToken()
	if err != nil {
		return nil, fmt.Errorf("generate probe token: %w", err)
	}

	probeCh := registrar.Register(token)
	defer registrar.Forget(token)

	probeURL := fmt.Sprintf("%s://%s/probe/%s", parsedTarget.Scheme, parsedTarget.Host, token)
	result.ProbeURL = probeURL

	fmt.Fprintf(&logs, "Probe URL: %s\n", probeURL)

	// 3. Set swUpdateUrl to the probe URL via telnet. Deferred restore
	// runs regardless of subsequent failures.
	setCmd := "sys configuration swUpdateUrl " + probeURL

	resp, err := t.SendCommand(setCmd)
	if err != nil {
		return result, fmt.Errorf("telnet set swUpdateUrl failed: %w", err)
	}

	if isCommandNotFound(resp) {
		return result, fmt.Errorf("device rejected %q (firmware does not expose this command)", setCmd)
	}

	fmt.Fprintf(&logs, "→ %s\n%s\n", setCmd, strings.TrimRight(resp, "\r\n"))

	defer func() {
		restoreCmd := "sys configuration swUpdateUrl " + originalURL
		if rresp, rerr := t.SendCommand(restoreCmd); rerr == nil && !isCommandNotFound(rresp) {
			result.Restored = true

			fmt.Fprintf(&logs, "→ %s (restored)\n%s\n", restoreCmd, strings.TrimRight(rresp, "\r\n"))
		} else if rerr != nil {
			fmt.Fprintf(&logs, "Restore failed: %v (envswitch persistence will heal on next reboot)\n", rerr)
		}

		result.Logs = logs.String()
	}()

	// 4. Trigger the device's outbound via :8090/swUpdateCheck. The
	// HTTP call is fire-and-forget — we don't need its response, only
	// that the device fans out to the probe URL we just set.
	swCheckURL := fmt.Sprintf("http://%s:8090/swUpdateCheck", deviceIP)

	go func() {
		if m.HTTPGet == nil {
			return
		}

		resp, err := m.HTTPGet(swCheckURL)
		if err != nil {
			return
		}

		_ = resp.Body.Close()
	}()

	// 5. Wait for the inbound.
	start := time.Now()

	select {
	case <-probeCh:
		result.Reached = true

		fmt.Fprintf(&logs, "Probe inbound observed after %v\n", time.Since(start))
	case <-time.After(timeout):
		result.Reached = false

		fmt.Fprintf(&logs, "Probe timed out after %v\n", timeout)
	}

	result.ElapsedMs = time.Since(start).Milliseconds()

	return result, nil
}
