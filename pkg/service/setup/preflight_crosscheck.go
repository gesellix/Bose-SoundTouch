package setup

import (
	"fmt"
	"strings"
)

// ParseGetpdoConfig is an exported wrapper around the internal `getpdo
// CurrentSystemConfiguration` reply parser, for callers outside this package
// (e.g. the diagnostic export, which reads the live URL set over telnet when
// SSH is unavailable). Returns a map keyed by config field name
// (margeServerUrl, statsServerUrl, swUpdateUrl, bmxRegistryUrl, …).
func ParseGetpdoConfig(text string) map[string]string {
	return parseGetpdoConfig(text)
}

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

// parseGetpdoConfig extracts field values from a `getpdo
// CurrentSystemConfiguration` reply. Two formats are accepted:
//
//  1. Protobuf-text-like nested blocks (the format observed on FW
//     27.0.6 ST 10/20/300 in the wild):
//
//     margeServerUrl {
//     text: "https://streaming.bose.com"
//     }
//
//  2. Flat key=value lines (kept as a tolerance path for firmware
//     variants that report differently or for hand-crafted test
//     fixtures).
//
// Any line that doesn't match either shape is silently ignored, so the
// parser tolerates banner text, prompt characters (`->`, `->OK`),
// blank lines, and unrelated fields.
func parseGetpdoConfig(text string) map[string]string {
	out := map[string]string{}

	var currentKey string

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		// Block open: "<key> {".
		if strings.HasSuffix(line, "{") {
			head := strings.TrimSpace(strings.TrimSuffix(line, "{"))
			if head != "" && isIdentifier(head) {
				currentKey = head
			}

			continue
		}

		// Block close.
		if line == "}" {
			currentKey = ""
			continue
		}

		// "text: ..." inside a block is the field value.
		if currentKey != "" && strings.HasPrefix(line, "text:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "text:"))
			val = strings.Trim(val, `"`)
			out[currentKey] = val

			continue
		}

		// Flat key=value, only if the key is a bare identifier (so we
		// don't misread protobuf "text: value" as a key=value pair via
		// some other separator).
		if i := strings.IndexByte(line, '='); i > 0 {
			key := strings.TrimSpace(line[:i])
			if key != "" && isIdentifier(key) {
				out[key] = strings.TrimSpace(line[i+1:])
			}
		}
	}

	return out
}

// isIdentifier reports whether s looks like a configuration field name —
// alphanumeric or underscore only. Used to keep parseGetpdoConfig from
// promoting random "x: y" or "x = y" lines (with spaces, punctuation,
// arrows) into the result map.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}

	return true
}
