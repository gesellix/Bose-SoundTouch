package setup

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// remoteServicesInjection is appended to the marge URL in the envswitch
// command. When the speaker next reads its boseurls (within ~60s), the device
// runs these shell commands: it touches the remote_services marker and starts
// sshd. This is the #471 bootstrap — it enables SSH on firmware with no prior
// SSH access and without a USB recovery stick. The whole marge value is
// double-quoted in the telnet command because it now contains spaces and
// semicolons.
const remoteServicesInjection = ";touch /tmp/remote_services;/etc/init.d/sshd start"

// EnableSSHViaTelnet bootstraps SSH on a speaker over its port-17000 shell by
// setting boseurls to an injected value (see remoteServicesInjection). It needs
// no existing SSH and no USB recovery. The injected commands run on the
// speaker's next boseurls check (up to ~60s), so callers should WaitForSSHPort
// afterwards, then ResetBoseURLs (to restore a usable marge URL) and
// EnsureRemoteServices (to persist SSH across reboots).
//
// serviceURL is the AfterTouch service base the speaker should point at
// (e.g. https://192.0.2.10:8443). It must not contain a double quote.
func (m *Manager) EnableSSHViaTelnet(deviceIP, serviceURL string) (string, error) {
	return m.setBoseURLsViaTelnet(deviceIP, serviceURL+remoteServicesInjection, serviceURL+"/update")
}

// ResetBoseURLs restores clean boseurls (no injected commands) after SSH has
// been enabled, so the speaker's marge URL is usable again.
func (m *Manager) ResetBoseURLs(deviceIP, serviceURL string) (string, error) {
	return m.setBoseURLsViaTelnet(deviceIP, serviceURL, serviceURL+"/update")
}

// setBoseURLsViaTelnet runs `envswitch boseurls set "<marge>" "<swUpdate>"`
// over the port-17000 shell. Both arguments are double-quoted so values
// containing spaces or semicolons (the SSH-enable injection) survive the
// device's command parser.
func (m *Manager) setBoseURLsViaTelnet(deviceIP, marge, swUpdate string) (string, error) {
	if m.NewTelnet == nil {
		return "", errors.New("telnet not configured: Manager.NewTelnet is nil")
	}

	if strings.Contains(marge, `"`) || strings.Contains(swUpdate, `"`) {
		return "", errors.New("boseurls values must not contain a double quote")
	}

	var logs strings.Builder

	t := m.NewTelnet(deviceIP)
	if err := t.Dial(); err != nil {
		return logs.String(), fmt.Errorf("telnet dial %s:17000: %w", deviceIP, err)
	}

	defer func() { _ = t.Close() }()

	if banner, _ := t.Probe(); banner != "" {
		fmt.Fprintf(&logs, "Telnet banner: %q\n", strings.TrimSpace(banner))
	}

	cmd := `envswitch boseurls set "` + marge + `" "` + swUpdate + `"`

	resp, err := t.SendCommand(cmd)
	if err != nil {
		return logs.String(), fmt.Errorf("telnet command %q failed: %w", cmd, err)
	}

	fmt.Fprintf(&logs, "→ %s\n%s\n", cmd, strings.TrimRight(resp, "\r\n"))

	if isCommandNotFound(resp) {
		return logs.String(), fmt.Errorf("device rejected %q (firmware does not expose envswitch)", cmd)
	}

	return logs.String(), nil
}

// WaitForSSHPort polls TCP :22 on the speaker until it accepts a connection or
// timeout elapses. Used after EnableSSHViaTelnet, since sshd starts only when
// the speaker next reads its boseurls (up to ~60s later).
func WaitForSSHPort(deviceIP string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(deviceIP, "22")

	for {
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("ssh (:22) on %s not reachable within %s: %w", deviceIP, timeout, err)
		}

		time.Sleep(3 * time.Second)
	}
}
