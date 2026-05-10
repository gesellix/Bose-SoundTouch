package setup

import (
	"fmt"
	"strings"
)

// crossCheckPreflights compares the URL fields visible via SSH (from the
// parsed SoundTouchSdkPrivateCfg.xml) with the same fields visible via
// telnet (from `getpdo CurrentSystemConfiguration`). Any field that is
// reported by both transports but with different values is recorded as
// a non-fatal warning.
//
// In practice the two sources can diverge briefly: `sys configuration …`
// writes the runtime fields, while `envswitch boseurls set …` writes a
// parallel persistence layer that wins on next boot — and the XML file
// is only re-rendered after a reboot. A warning here is therefore not an
// error per se; it usually means "reboot the device to make the two
// layers agree."
func (m *Manager) crossCheckPreflights(summary *MigrationSummary) {
	if summary.ParsedCurrentConfig == nil || summary.TelnetVerifiedConfig == "" {
		return
	}

	telnet := parseGetpdoConfig(summary.TelnetVerifiedConfig)
	xml := summary.ParsedCurrentConfig

	pairs := []struct {
		name     string
		xmlValue string
	}{
		{"margeServerUrl", xml.MargeServerUrl},
		{"statsServerUrl", xml.StatsServerUrl},
		{"swUpdateUrl", xml.SwUpdateUrl},
		{"bmxRegistryUrl", xml.BmxRegistryUrl},
	}

	for _, p := range pairs {
		telnetValue, hasTelnet := telnet[p.name]
		if !hasTelnet || p.xmlValue == "" {
			continue
		}

		if telnetValue == p.xmlValue {
			continue
		}

		summary.Warnings = append(summary.Warnings, fmt.Sprintf(
			"%s differs between transports: SSH-XML=%q telnet-getpdo=%q (a reboot usually re-syncs the runtime layer with the persisted XML)",
			p.name, p.xmlValue, telnetValue,
		))
	}
}

// parseGetpdoConfig extracts key=value pairs from `getpdo CurrentSystemConfiguration`
// output. The format observed in the wild is one pair per line; any line
// that does not match key=value is silently skipped, so the parser is
// tolerant to banner text or trailing prompt characters.
func parseGetpdoConfig(text string) map[string]string {
	out := map[string]string{}

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])

		if key != "" {
			out[key] = val
		}
	}

	return out
}
