package setup

import (
	"errors"
	"strings"
	"testing"
)

func TestReboot_DefaultIsSSH(t *testing.T) {
	var ranCmds []string

	m := &Manager{
		NewSSH: func(host string) SSHClient {
			return &mockSSH{runFunc: func(cmd string) (string, error) {
				ranCmds = append(ranCmds, cmd)
				return "ok\n", nil
			}}
		},
	}

	if _, err := m.Reboot("192.0.2.1", ""); err != nil {
		t.Fatalf("Reboot: %v", err)
	}

	found := false
	for _, c := range ranCmds {
		if strings.Contains(c, "reboot") {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected SSH `reboot` command, got %v", ranCmds)
	}
}

func TestReboot_TelnetSendsSysReboot(t *testing.T) {
	f := &fakeTelnet{
		responses: map[string]string{"sys reboot": "OK\n"},
	}

	m := &Manager{
		NewTelnet: func(host string) TelnetClient { return f },
	}

	if _, err := m.Reboot("192.0.2.1", RebootMethodTelnet); err != nil {
		t.Fatalf("Reboot: %v", err)
	}

	if len(f.commands) != 1 || f.commands[0] != "sys reboot" {
		t.Errorf("commands = %v, want [sys reboot]", f.commands)
	}
}

func TestReboot_TelnetTreatsCloseAsSuccess(t *testing.T) {
	// The device closes the socket as part of rebooting. SendCommand surfaces
	// that as an EOF/closed error; the reboot path must absorb it.
	f := &fakeTelnet{
		fail: map[string]error{"sys reboot": errors.New("EOF")},
	}

	m := &Manager{
		NewTelnet: func(host string) TelnetClient { return f },
	}

	out, err := m.Reboot("192.0.2.1", RebootMethodTelnet)
	if err != nil {
		t.Fatalf("Reboot should swallow socket-close after sys reboot, got %v", err)
	}

	if !strings.Contains(out, "connection closed by reboot") {
		t.Errorf("output should annotate the close, got %q", out)
	}
}

func TestReboot_TelnetSurfacesDialError(t *testing.T) {
	f := &fakeTelnet{dialErr: errors.New("connection refused")}
	m := &Manager{
		NewTelnet: func(host string) TelnetClient { return f },
	}

	if _, err := m.Reboot("192.0.2.1", RebootMethodTelnet); err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

func TestReboot_UnknownMethodErrors(t *testing.T) {
	m := &Manager{}

	if _, err := m.Reboot("192.0.2.1", RebootMethod("ftp")); err == nil {
		t.Fatal("expected error for unsupported reboot method")
	}
}
