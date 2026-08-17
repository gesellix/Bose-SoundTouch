package setup

import (
	"strings"
	"testing"
	"time"
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

	if _, err := m.EnableSSHViaTelnetFullConfig("192.0.2.10", svc, 0); err != nil {
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

// fullConfigResponses builds the {command: "OK"} map for
// EnableSSHViaTelnetFullConfig's fixed 6-step sequence (5 commands + the
// getpdo verification) against svc, matching
// TestEnableSSHViaTelnetFullConfig_BuildsInjectedSequence's command list.
func fullConfigResponses(svc string) map[string]string {
	injected := svc + `;touch /tmp/remote_services;/etc/init.d/sshd start`

	cmds := []string{
		`sys configuration bmxRegistryUrl "` + svc + `/bmx/registry/v1/services"`,
		`sys configuration statsServerUrl "` + svc + `"`,
		`sys configuration margeServerUrl "` + injected + `"`,
		`sys configuration swUpdateUrl "` + svc + `/updates/soundtouch"`,
		`envswitch boseurls set "` + injected + `" "` + svc + `/updates/soundtouch"`,
		`getpdo CurrentSystemConfiguration`,
	}

	resp := make(map[string]string, len(cmds))
	for _, c := range cmds {
		resp[c] = "OK\n"
	}

	return resp
}

// TestEnableSSHViaTelnetFullConfig_PausesBetweenCommands is the regression
// test for #515 comment 5228449448: the same commands sent back-to-back
// left sshd down on a real device, but succeeded sent one at a time with
// gaps. Uses a small real duration rather than a fake clock/injectable
// sleeper — simplest thing that actually proves time.Sleep is in the loop,
// and small enough (5 gaps x 5ms) not to slow the suite down.
func TestEnableSSHViaTelnetFullConfig_PausesBetweenCommands(t *testing.T) {
	const svc = "https://192.0.2.10:8443"
	const delay = 5 * time.Millisecond

	f := &fakeTelnet{responses: fullConfigResponses(svc)}
	m := newFakeTelnetManager(f)

	start := time.Now()

	if _, err := m.EnableSSHViaTelnetFullConfig("192.0.2.10", svc, delay); err != nil {
		t.Fatalf("EnableSSHViaTelnetFullConfig: %v", err)
	}

	elapsed := time.Since(start)
	// 5 real commands = 5 gaps (see runTelnetInjection: delay after each
	// command in the loop, including before the getpdo verification).
	wantMin := 5 * delay

	if elapsed < wantMin {
		t.Errorf("elapsed %v, want at least %v (delay not applied between commands)", elapsed, wantMin)
	}
}

// TestEnableSSHViaTelnetFullConfig_ZeroDelayIsInstant verifies 0 keeps the
// old back-to-back behavior — no accidental minimum sleep.
func TestEnableSSHViaTelnetFullConfig_ZeroDelayIsInstant(t *testing.T) {
	const svc = "https://192.0.2.10:8443"

	f := &fakeTelnet{responses: fullConfigResponses(svc)}
	m := newFakeTelnetManager(f)

	start := time.Now()

	if _, err := m.EnableSSHViaTelnetFullConfig("192.0.2.10", svc, 0); err != nil {
		t.Fatalf("EnableSSHViaTelnetFullConfig: %v", err)
	}

	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("elapsed %v with a 0 delay, expected near-instant", elapsed)
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

// TestSetAllBoseURLsViaTelnet_WritesAllFourBeforeEnvswitch is the regression
// test for the stale statsServerUrl/bmxRegistryUrl bug reported in #621: the
// XML migration's telnet resync used to commit `envswitch boseurls set` with
// only marge/swUpdate as arguments, silently freezing whatever stats/bmx
// happened to still be in the runtime layer at that moment. This asserts all
// four `sys configuration` writes land before the single `envswitch` commit,
// matching telnetURLs.Commands()'s known-good sequence.
func TestSetAllBoseURLsViaTelnet_WritesAllFourBeforeEnvswitch(t *testing.T) {
	const targetURL = "http://localhost:8000"

	urls := telnetURLs{
		Marge:       targetURL,
		Stats:       targetURL,
		SwUpdate:    targetURL + "/updates/soundtouch",
		BmxRegistry: targetURL + "/bmx/registry/v1/services",
	}

	want := urls.Commands()

	resp := make(map[string]string, len(want))
	for _, c := range want {
		resp[c] = "OK\n"
	}

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	if _, err := m.setAllBoseURLsViaTelnet("192.0.2.10", urls); err != nil {
		t.Fatalf("setAllBoseURLsViaTelnet: %v", err)
	}

	if len(f.commands) != len(want) {
		t.Fatalf("sent %d commands %q\n want %d %q", len(f.commands), f.commands, len(want), want)
	}

	for i, c := range want {
		if f.commands[i] != c {
			t.Errorf("command %d = %q\n want %q", i, f.commands[i], c)
		}
	}

	envswitchIdx := len(want) - 1
	for i, c := range f.commands[:envswitchIdx] {
		if !strings.HasPrefix(c, "sys configuration ") {
			t.Errorf("command %d = %q, want a `sys configuration ...` runtime write before the envswitch commit", i, c)
		}
	}

	if !strings.HasPrefix(f.commands[envswitchIdx], "envswitch boseurls set ") {
		t.Errorf("last command = %q, want the envswitch commit last", f.commands[envswitchIdx])
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
