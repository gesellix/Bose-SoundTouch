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

// fwScript is the speaker's persistent iptables script; appending here makes a
// rule survive reboot (it is re-applied on boot).
const fwScript = "/etc/init.d/Firewalls/update_iptables"

// block17000Marker guards the appended rule so Close17000 is idempotent.
const block17000Marker = "# Block 17000 (added by AfterTouch)"

// Close17000 blocks the port-17000 diagnostic shell from the LAN over SSH:
// it persists an iptables rule in the firewall script (idempotent, keyed on
// block17000Marker) and applies the rule immediately, keeping loopback access.
// Opt-in — the caller decides whether to harden. Needs SSH already enabled.
func (m *Manager) Close17000(deviceIP string) (string, error) {
	if m.NewSSH == nil {
		return "", errors.New("ssh not configured: Manager.NewSSH is nil")
	}

	persist := "grep -q '" + block17000Marker + "' " + fwScript + " 2>/dev/null || cat >> " + fwScript + " <<'AFTEREOF'\n\n" +
		block17000Marker + "\n" +
		"iptables -I INPUT -p tcp --dport 17000 -j DROP\n" +
		"iptables -I INPUT -p tcp --dport 17000 -i lo -j ACCEPT\n" +
		"AFTEREOF"

	steps := []struct{ desc, cmd string }{
		{"remount / read-write", "mount / -o rw,remount"},
		{"persist firewall rule", persist},
		{"apply firewall rule now", "iptables -I INPUT -p tcp --dport 17000 -j DROP; iptables -I INPUT -p tcp --dport 17000 -i lo -j ACCEPT"},
	}

	return m.runSSHSteps(deviceIP, steps)
}

// InstallAuthorizedKey installs an SSH public key for root so access no longer
// relies on the empty-password login. Opt-in. Needs SSH already enabled.
func (m *Manager) InstallAuthorizedKey(deviceIP, publicKey string) (string, error) {
	if m.NewSSH == nil {
		return "", errors.New("ssh not configured: Manager.NewSSH is nil")
	}

	key := strings.TrimSpace(publicKey)
	if key == "" {
		return "", errors.New("public key is empty")
	}

	c := m.NewSSH(deviceIP)

	var logs strings.Builder

	if out, err := c.Run("mount / -o rw,remount && mkdir -p -m 700 /home/root/.ssh"); err != nil {
		fmt.Fprintf(&logs, "→ prepare /home/root/.ssh\n%s\n", strings.TrimSpace(out))
		return logs.String(), fmt.Errorf("prepare /home/root/.ssh: %w", err)
	}

	if err := c.UploadContent([]byte(key+"\n"), "/home/root/.ssh/authorized_keys"); err != nil {
		return logs.String(), fmt.Errorf("upload authorized_keys: %w", err)
	}

	if out, err := c.Run("chmod 600 /home/root/.ssh/authorized_keys"); err != nil {
		fmt.Fprintf(&logs, "→ chmod authorized_keys\n%s\n", strings.TrimSpace(out))
		return logs.String(), fmt.Errorf("chmod authorized_keys: %w", err)
	}

	logs.WriteString("Installed authorized_keys for root.\n")

	return logs.String(), nil
}

// runSSHSteps runs an ordered list of shell commands over a single-shot SSH
// client, aborting on the first failure. Commands MUST be service-controlled
// literals, never built from untrusted HTTP input.
func (m *Manager) runSSHSteps(deviceIP string, steps []struct{ desc, cmd string }) (string, error) {
	c := m.NewSSH(deviceIP)

	var logs strings.Builder

	for _, s := range steps {
		out, err := c.Run(s.cmd)

		fmt.Fprintf(&logs, "→ %s\n%s\n", s.desc, strings.TrimSpace(out))

		if err != nil {
			return logs.String(), fmt.Errorf("%s: %w", s.desc, err)
		}
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
