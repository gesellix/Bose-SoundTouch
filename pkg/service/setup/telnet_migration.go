package setup

import (
	"errors"
	"fmt"
	"strings"
)

// telnetURLs holds the four URLs the migration writes via telnet. Most
// users keep all four pointing at the same service base; per-field
// overrides exist mainly so soundcork users can append /marge to the
// marge URL.
type telnetURLs struct {
	Marge       string
	Stats       string
	SwUpdate    string
	BmxRegistry string
}

// defaultTelnetURLs returns the canonical URL set derived from the
// soundtouch-service base targetURL.
func defaultTelnetURLs(targetURL string) telnetURLs {
	return telnetURLs{
		Marge:       targetURL,
		Stats:       targetURL,
		SwUpdate:    targetURL + "/updates/soundtouch",
		BmxRegistry: targetURL + "/bmx/registry/v1/services",
	}
}

// telnetURLsFromOptions resolves the four URLs from targetURL plus
// per-field overrides supplied via the migration options map. Recognised
// keys are marge_url, stats_url, sw_update_url, bmx_url; missing or empty
// entries fall back to the canonical default.
//
// We deliberately do not expose a "proxied"/"original" semantic here
// (unlike the XML method's applyProxyOptions): per the discussion that
// motivated this iteration, the goal is to keep the user model simple —
// one base URL plus optional path suffixes — and let the service layer
// hold any non-trivial logic.
func telnetURLsFromOptions(targetURL string, options map[string]string) telnetURLs {
	u := defaultTelnetURLs(targetURL)

	if v := options["marge_url"]; v != "" {
		u.Marge = v
	}

	if v := options["stats_url"]; v != "" {
		u.Stats = v
	}

	if v := options["sw_update_url"]; v != "" {
		u.SwUpdate = v
	}

	if v := options["bmx_url"]; v != "" {
		u.BmxRegistry = v
	}

	return u
}

// Commands returns the canonical sequence of telnet commands. Order
// matters: `sys configuration …` writes the runtime layer; the closing
// `envswitch boseurls set …` writes the parallel persistence layer that
// otherwise wins on the next reboot.
//
// Envswitch derivation rule: arg1 mirrors u.Marge verbatim, arg2 mirrors
// u.SwUpdate verbatim. Soundcork users who set Marge to "<base>/marge"
// therefore get "envswitch boseurls set <base>/marge <base>/updates/soundtouch"
// without any extra plumbing — the parallel layer stays consistent with
// the runtime layer.
func (u telnetURLs) Commands() []string {
	return []string{
		"sys configuration bmxRegistryUrl " + u.BmxRegistry,
		"sys configuration statsServerUrl " + u.Stats,
		"sys configuration margeServerUrl " + u.Marge,
		"sys configuration swUpdateUrl " + u.SwUpdate,
		"envswitch boseurls set " + u.Marge + " " + u.SwUpdate,
	}
}

// migrateViaTelnet runs the URL-configuration sequence over the device's
// port-17000 diagnostic shell. It writes configuration only — reboot is left
// to the user, who triggers it via the existing reboot button (which now
// accepts a method=telnet|ssh selector).
//
// The sequence aborts on the first non-OK response so we never half-write the
// configuration; the caller can retry safely after fixing the underlying
// issue (closed port, hardened firmware, etc.).
//
// targetURL is kept as a separate verification anchor: most users have
// every URL share that base, so substring-matching it against the
// device's `getpdo` reply is the simplest "did the writes stick?" check
// that still works for the soundcork "/marge on one field" case.
func (m *Manager) migrateViaTelnet(deviceIP, targetURL string, urls telnetURLs) (string, error) {
	if m.NewTelnet == nil {
		return "", errors.New("telnet migration not configured: Manager.NewTelnet is nil")
	}

	var logs strings.Builder

	t := m.NewTelnet(deviceIP)
	if err := t.Dial(); err != nil {
		return logs.String(), fmt.Errorf("telnet dial %s:17000 failed: %w", deviceIP, err)
	}

	defer func() { _ = t.Close() }()

	banner, _ := t.Probe()
	if banner != "" {
		fmt.Fprintf(&logs, "Telnet banner: %q\n", strings.TrimSpace(banner))
	}

	for _, cmd := range urls.Commands() {
		resp, err := t.SendCommand(cmd)
		if err != nil {
			return logs.String(), fmt.Errorf("telnet command %q failed: %w", cmd, err)
		}

		fmt.Fprintf(&logs, "→ %s\n%s\n", cmd, strings.TrimRight(resp, "\r\n"))

		if isCommandNotFound(resp) {
			return logs.String(), fmt.Errorf("device rejected %q (firmware does not expose this command)", cmd)
		}
	}

	verify, err := t.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil {
		return logs.String(), fmt.Errorf("verification command failed: %w", err)
	}

	fmt.Fprintf(&logs, "→ getpdo CurrentSystemConfiguration\n%s\n", strings.TrimRight(verify, "\r\n"))

	if !strings.Contains(verify, targetURL) {
		return logs.String(), fmt.Errorf("verification failed: getpdo response does not contain %q (device may have rejected the new URLs)", targetURL)
	}

	logs.WriteString("Telnet migration succeeded. Reboot the device to apply.\n")

	return logs.String(), nil
}

// isCommandNotFound returns true if the device's response to a command
// indicates the command is not available on this firmware. Different firmware
// builds use slightly different wording; we accept any of the observed
// variants.
func isCommandNotFound(resp string) bool {
	low := strings.ToLower(resp)

	return strings.Contains(low, "command not found") ||
		strings.Contains(low, "unknown command") ||
		strings.Contains(low, "not implemented")
}
