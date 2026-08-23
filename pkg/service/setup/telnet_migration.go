package setup

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// ErrInvalidTelnetURL identifies URL input rejected before any device
// connection or command is attempted.
var ErrInvalidTelnetURL = errors.New("invalid telnet URL")

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

type telnetURLField struct {
	configName string
	value      string
}

func (u telnetURLs) fields() []telnetURLField {
	return []telnetURLField{
		{configName: "margeServerUrl", value: u.Marge},
		{configName: "statsServerUrl", value: u.Stats},
		{configName: "swUpdateUrl", value: u.SwUpdate},
		{configName: "bmxRegistryUrl", value: u.BmxRegistry},
	}
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

// canonicalBoseTelnetURLs returns the public Bose endpoints restored by a
// telnet revert.
func canonicalBoseTelnetURLs() telnetURLs {
	return telnetURLs{
		Marge:       "https://streaming.bose.com",
		Stats:       "https://events.api.bosecm.com",
		SwUpdate:    "https://worldwide.bose.com/updates/soundtouch",
		BmxRegistry: "https://content.api.bose.io/bmx/registry/v1/services",
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
	return defaultTelnetURLs(targetURL).withOptions(options)
}

func (u telnetURLs) withOptions(options map[string]string) telnetURLs {
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

func (u telnetURLs) validate() error {
	for _, field := range u.fields() {
		if err := validateTelnetURL(field.configName, field.value); err != nil {
			return err
		}
	}

	return nil
}

func validateTelnetURL(field, value string) error {
	invalid := func(reason string) error {
		return fmt.Errorf("%w for %s: %s", ErrInvalidTelnetURL, field, reason)
	}

	if value == "" {
		return invalid("value is empty")
	}

	if strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) >= 0 {
		return invalid("whitespace and control characters are not allowed")
	}

	// These characters can change command parsing or the persisted shell
	// expression used by Bose firmware. Clean service URLs do not need them.
	if strings.ContainsAny(value, "\"'`;\\|&$<>(){}") {
		return invalid("shell metacharacters are not allowed")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return invalid("value cannot be parsed")
	}

	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return invalid("scheme must be http or https")
	}

	if parsed.Hostname() == "" {
		return invalid("host is required")
	}

	if parsed.User != nil {
		return invalid("user information is not allowed")
	}

	if parsed.RawQuery != "" || parsed.ForceQuery {
		return invalid("query parameters are not allowed")
	}

	if parsed.Fragment != "" {
		return invalid("fragments are not allowed")
	}

	return nil
}

func telnetRevertAvailable(response string) bool {
	current := parseGetpdoConfig(response)

	for _, field := range canonicalBoseTelnetURLs().fields() {
		if value, ok := current[field.configName]; ok && value != "" && value != field.value {
			return true
		}
	}

	return false
}

// RevertTelnetURLs restores the canonical Bose URL configuration over the
// device's port-17000 shell. Per-field URL options take precedence over the
// defaults.
func (m *Manager) RevertTelnetURLs(deviceIP string, options map[string]string) (string, error) {
	urls := canonicalBoseTelnetURLs().withOptions(options)
	logs := "Restoring Bose URL configuration only; no factory reset, account change, or reboot will be performed. " +
		"Telnet writes are sequential, not transactional; if the operation fails, read back all four URL fields before retrying or rebooting.\n"
	migrationLogs, err := m.migrateViaTelnet(deviceIP, urls)

	return logs + migrationLogs, err
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
// Commands are sent sequentially and cannot be rolled back atomically. Errors
// therefore report whether runtime writes were confirmed or the persistence
// command may already have taken effect.
func (m *Manager) migrateViaTelnet(deviceIP string, urls telnetURLs) (string, error) {
	if err := urls.validate(); err != nil {
		return "", err
	}

	if m.NewTelnet == nil {
		return "", errors.New("telnet migration not configured: Manager.NewTelnet is nil")
	}

	unlock := m.lockTelnetURLMutation(deviceIP)
	defer unlock()

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

	commands := urls.Commands()
	runtimeWrites := 0

	for i, cmd := range commands {
		persistenceCommand := i == len(commands)-1

		resp, err := t.SendCommand(cmd)
		if err != nil {
			return logs.String(), fmt.Errorf("telnet command %q failed: %w; %s", cmd, err,
				telnetWriteFailureContext(runtimeWrites, persistenceCommand))
		}

		fmt.Fprintf(&logs, "→ %s\n%s\n", cmd, strings.TrimRight(resp, "\r\n"))

		if err := validateTelnetMutationResponse(cmd, resp, persistenceCommand, urls); err != nil {
			return logs.String(), fmt.Errorf("%w; %s", err,
				telnetWriteFailureContext(runtimeWrites, persistenceCommand))
		}

		if !persistenceCommand {
			runtimeWrites++
		}
	}

	verify, err := t.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil {
		return logs.String(), fmt.Errorf("verification command failed after envswitch was accepted: %w; "+
			"persistence may already have changed, so read back and reconcile all four URL fields before rebooting", err)
	}

	fmt.Fprintf(&logs, "→ getpdo CurrentSystemConfiguration (runtime layer only — confirms the writes were accepted, not that they'll survive a reboot)\n%s\n", strings.TrimRight(verify, "\r\n"))

	if err := verifyTelnetURLs(verify, urls); err != nil {
		return logs.String(), fmt.Errorf("%w; envswitch was accepted, so persistence may already have changed; "+
			"read back and reconcile all four URL fields before rebooting", err)
	}

	logs.WriteString("Telnet writes accepted (runtime layer). Reboot the device so the envswitch-persisted layer takes over.\n")

	return logs.String(), nil
}

func validateTelnetMutationResponse(cmd, response string, persistenceCommand bool, urls telnetURLs) error {
	if isCommandNotFound(response) {
		return fmt.Errorf("device rejected %q (firmware does not expose this command)", cmd)
	}

	if persistenceCommand {
		expected := "Setting Bose Server URLs to " + urls.Marge + " and " + urls.SwUpdate
		if hasTelnetPersistenceConfirmation(response, expected) {
			return nil
		}
	} else if hasTelnetOKResponse(response) {
		return nil
	}

	return fmt.Errorf("device did not confirm telnet command %q; response was %q", cmd, strings.TrimSpace(response))
}

func hasTelnetOKResponse(response string) bool {
	return hasExactTelnetResponseLine(response, "OK", true)
}

func meaningfulTelnetResponseLines(response string) []string {
	var meaningful []string

	for _, raw := range strings.Split(response, "\n") {
		line := strings.TrimSpace(raw)
		for strings.HasPrefix(line, "->") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "->"))
		}

		if line != "" {
			meaningful = append(meaningful, line)
		}
	}

	return meaningful
}

func hasExactTelnetResponseLine(response, expected string, foldCase bool) bool {
	meaningful := meaningfulTelnetResponseLines(response)

	if len(meaningful) != 1 {
		return false
	}

	if foldCase {
		return strings.EqualFold(meaningful[0], expected)
	}

	return meaningful[0] == expected
}

// hasTelnetPersistenceConfirmation accepts exactly the two response shapes
// observed on SoundTouch firmware: a confirmation ending in a prompt marker,
// or the confirmation followed by a prompt-prefixed OK line.
func hasTelnetPersistenceConfirmation(response, expected string) bool {
	meaningful := meaningfulTelnetResponseLines(response)

	return len(meaningful) == 1 && meaningful[0] == expected+" ->" ||
		len(meaningful) == 2 && meaningful[0] == expected && meaningful[1] == "OK"
}

func telnetWriteFailureContext(runtimeWrites int, persistenceAttempted bool) string {
	if persistenceAttempted {
		return "all four runtime URL writes were confirmed, but the persistence outcome is uncertain; " +
			"read back all four URL fields before retrying or rebooting"
	}

	if runtimeWrites > 0 {
		return fmt.Sprintf("%d runtime URL write(s) were confirmed, so the runtime configuration may be partial; "+
			"read back all four URL fields before retrying or rebooting", runtimeWrites)
	}

	return "no URL write was confirmed, but the attempted command may have reached the speaker; " +
		"read back all four URL fields before retrying or rebooting"
}

func verifyTelnetURLs(response string, want telnetURLs) error {
	got := parseGetpdoConfig(response)

	for _, field := range want.fields() {
		value, ok := got[field.configName]
		if !ok {
			return fmt.Errorf("verification failed: getpdo response is missing %s", field.configName)
		}

		if value != field.value {
			return fmt.Errorf("verification failed: %s is %q, want %q", field.configName, value, field.value)
		}
	}

	return nil
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
