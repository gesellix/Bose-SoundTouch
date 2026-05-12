package setup

import (
	"errors"
	"fmt"
	"strings"
)

// FactoryReset issues `sys factorydefault` over the device's port-17000
// diagnostic shell. The device wipes its persistent state (account
// pairing, Wi-Fi credentials, presets, source configuration) and reboots
// into setup mode — broadcasting its own `Bose SoundTouch XXXX` access
// point on 192.0.2.1.
//
// After this call the device is unreachable on the home network until
// the caller pushes new Wi-Fi credentials via PushWiFiCredentials (see
// wifi_provision.go).
func (m *Manager) FactoryReset(deviceIP string) (string, error) {
	if m.NewTelnet == nil {
		return "", errors.New("FactoryReset: Manager.NewTelnet is nil")
	}

	var logs strings.Builder

	t := m.NewTelnet(deviceIP)
	if err := t.Dial(); err != nil {
		return logs.String(), fmt.Errorf("telnet dial %s:17000: %w", deviceIP, err)
	}

	defer func() { _ = t.Close() }()

	banner, _ := t.Probe()
	if banner != "" {
		fmt.Fprintf(&logs, "Telnet banner: %q\n", strings.TrimSpace(banner))
	}

	resp, err := t.SendCommand("sys factorydefault")
	if err != nil {
		// A graceful close right after the command is normal — the device
		// reboots immediately. We treat "connection closed" responses as
		// success rather than failure.
		if isExpectedDisconnect(err) {
			fmt.Fprintf(&logs, "→ sys factorydefault\n(device disconnected — reset accepted)\n")
			return logs.String(), nil
		}

		return logs.String(), fmt.Errorf("sys factorydefault: %w", err)
	}

	fmt.Fprintf(&logs, "→ sys factorydefault\n%s\n", strings.TrimRight(resp, "\r\n"))

	if isCommandNotFound(resp) {
		return logs.String(), fmt.Errorf("device rejected `sys factorydefault` (firmware does not expose this command)")
	}

	return logs.String(), nil
}

// isExpectedDisconnect reports whether an error from SendCommand is the
// normal "device closed the socket while rebooting" pattern, which we
// see during factory-reset.
func isExpectedDisconnect(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "broken pipe")
}
