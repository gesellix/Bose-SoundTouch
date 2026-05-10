package setup

import (
	"errors"
	"fmt"
	"strings"
)

// telnetURLConfigCommands returns the canonical sequence of telnet commands
// that point a SoundTouch device at the given local-service base URL.
//
// Order matters: `sys configuration …` writes the runtime URL, while
// `envswitch boseurls set …` writes a parallel persistence layer that
// otherwise wins on the next reboot. See docs/analysis/TELNET-MIGRATION-METHOD.md
// §2.1 for the discussion this is derived from.
func telnetURLConfigCommands(targetURL string) []string {
	return []string{
		"sys configuration bmxRegistryUrl " + targetURL + "/bmx/registry/v1/services",
		"sys configuration statsServerUrl " + targetURL,
		"sys configuration margeServerUrl " + targetURL,
		"sys configuration swUpdateUrl " + targetURL + "/updates/soundtouch",
		"envswitch boseurls set " + targetURL + " " + targetURL + "/updates/soundtouch",
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
func (m *Manager) migrateViaTelnet(deviceIP, targetURL string) (string, error) {
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

	for _, cmd := range telnetURLConfigCommands(targetURL) {
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
