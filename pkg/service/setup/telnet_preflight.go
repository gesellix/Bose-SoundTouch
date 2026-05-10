package setup

import (
	"fmt"
	"strings"
)

// telnetPreflight performs a read-only check of the device's port-17000
// diagnostic shell and populates the Telnet* fields on summary.
//
// It exists so the migration UI can decide whether to offer the telnet
// method, and so a telnet-only (SSH-less) device can still tell us whether
// it is already pointing at our service. The probe is deliberately scoped
// to safe, non-mutating commands:
//
//  1. TCP dial :17000 (Manager.NewTelnet handles the timeouts).
//  2. Read whatever banner the shell emits.
//  3. `getpdo CurrentSystemConfiguration` — a read-only command; if the
//     device answers with "command not found" we record that too so the UI
//     can disable the telnet option with a reason.
//
// Errors are recorded on summary.TelnetProbeError rather than returned, so
// preflight is best-effort and never breaks the rest of GetMigrationSummary.
func (m *Manager) telnetPreflight(summary *MigrationSummary, deviceIP string) {
	if m.NewTelnet == nil {
		summary.TelnetProbeError = "telnet client not configured"
		return
	}

	t := m.NewTelnet(deviceIP)

	if err := t.Dial(); err != nil {
		summary.TelnetProbeError = fmt.Sprintf("dial %s:17000: %v", deviceIP, err)
		return
	}

	defer func() { _ = t.Close() }()

	summary.TelnetReachable = true

	if banner, _ := t.Probe(); banner != "" {
		summary.TelnetBanner = strings.TrimSpace(banner)
	}

	resp, err := t.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil {
		summary.TelnetProbeError = fmt.Sprintf("getpdo CurrentSystemConfiguration: %v", err)
		return
	}

	if isCommandNotFound(resp) {
		summary.TelnetProbeError = "device rejected getpdo CurrentSystemConfiguration (firmware does not expose it)"
		return
	}

	summary.TelnetVerifiedConfig = strings.TrimRight(resp, "\r\n")
}
