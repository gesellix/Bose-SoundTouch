package setup

import (
	"strings"
	"testing"
)

func TestEnableSSHViaTelnet_BuildsInjectedCommand(t *testing.T) {
	const svc = "https://192.0.2.10:8443"

	want := `envswitch boseurls set "https://192.0.2.10:8443;touch /tmp/remote_services;/etc/init.d/sshd start" "https://192.0.2.10:8443/update"`

	f := &fakeTelnet{responses: map[string]string{want: "OK\n"}}
	m := newFakeTelnetManager(f)

	if _, err := m.EnableSSHViaTelnet("192.0.2.10", svc); err != nil {
		t.Fatalf("EnableSSHViaTelnet: %v", err)
	}

	if len(f.commands) != 1 || f.commands[0] != want {
		t.Errorf("sent %q\n want %q", f.commands, want)
	}
}

func TestEnableSSHViaTelnetFullConfig_BuildsInjectedSequence(t *testing.T) {
	const svc = "https://192.0.2.10:8443"

	const injected = `https://192.0.2.10:8443;touch /tmp/remote_services;/etc/init.d/sshd start`

	want := []string{
		`sys configuration bmxRegistryUrl "https://192.0.2.10:8443/bmx/registry/v1/services"`,
		`sys configuration statsServerUrl "https://192.0.2.10:8443"`,
		`sys configuration margeServerUrl "` + injected + `"`,
		`sys configuration swUpdateUrl "https://192.0.2.10:8443/updates/soundtouch"`,
		`envswitch boseurls set "` + injected + `" "https://192.0.2.10:8443/updates/soundtouch"`,
		`getpdo CurrentSystemConfiguration`,
	}

	resp := map[string]string{}
	for _, c := range want {
		resp[c] = "OK\n"
	}

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	if _, err := m.EnableSSHViaTelnetFullConfig("192.0.2.10", svc); err != nil {
		t.Fatalf("EnableSSHViaTelnetFullConfig: %v", err)
	}

	if len(f.commands) != len(want) {
		t.Fatalf("sent %d commands %q\n want %d %q", len(f.commands), f.commands, len(want), want)
	}

	for i, c := range want {
		if f.commands[i] != c {
			t.Errorf("command %d = %q\n want %q", i, f.commands[i], c)
		}
	}
}

func TestResetBoseURLs_BuildsCleanCommand(t *testing.T) {
	const svc = "https://192.0.2.10:8443"

	want := `envswitch boseurls set "https://192.0.2.10:8443" "https://192.0.2.10:8443/update"`

	f := &fakeTelnet{responses: map[string]string{want: "OK\n"}}
	m := newFakeTelnetManager(f)

	if _, err := m.ResetBoseURLs("192.0.2.10", svc); err != nil {
		t.Fatalf("ResetBoseURLs: %v", err)
	}

	if len(f.commands) != 1 || f.commands[0] != want {
		t.Errorf("sent %q\n want %q", f.commands, want)
	}
}

func TestSetBoseURLs_RejectsDoubleQuote(t *testing.T) {
	m := newFakeTelnetManager(&fakeTelnet{})

	if _, err := m.EnableSSHViaTelnet("192.0.2.10", `https://x"evil`); err == nil {
		t.Fatal("expected an error when the service URL contains a double quote")
	}
}

func TestClose17000_RunsFirewallSteps(t *testing.T) {
	var ran []string

	m := &Manager{NewSSH: func(string) SSHClient {
		return &mockSSH{runFunc: func(cmd string) (string, error) {
			ran = append(ran, cmd)
			return "", nil
		}}
	}}

	if _, err := m.Close17000("192.0.2.10"); err != nil {
		t.Fatalf("Close17000: %v", err)
	}

	joined := strings.Join(ran, "\n")
	for _, want := range []string{
		"mount / -o rw,remount",
		block17000Marker,
		"iptables -I INPUT -p tcp --dport 17000 -j DROP",
		"--dport 17000 -i lo -j ACCEPT",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("Close17000 commands missing %q\nran:\n%s", want, joined)
		}
	}
}

func TestInstallAuthorizedKey_UploadsKey(t *testing.T) {
	m := &Manager{NewSSH: func(string) SSHClient {
		return &mockSSH{runFunc: func(string) (string, error) { return "", nil }}
	}}

	if _, err := m.InstallAuthorizedKey("192.0.2.10", "  ssh-ed25519 AAAATEST comment  "); err != nil {
		t.Fatalf("InstallAuthorizedKey: %v", err)
	}
	// A fresh mockSSH is created per NewSSH call, so re-run against a captured
	// one to assert the upload.
	var captured *mockSSH

	m.NewSSH = func(string) SSHClient {
		captured = &mockSSH{runFunc: func(string) (string, error) { return "", nil }}
		return captured
	}

	if _, err := m.InstallAuthorizedKey("192.0.2.10", "ssh-ed25519 AAAATEST comment"); err != nil {
		t.Fatalf("InstallAuthorizedKey: %v", err)
	}

	got, ok := captured.uploaded["/home/root/.ssh/authorized_keys"]
	if !ok {
		t.Fatal("authorized_keys was not uploaded")
	}

	if strings.TrimSpace(string(got)) != "ssh-ed25519 AAAATEST comment" {
		t.Errorf("uploaded key = %q", string(got))
	}
}
