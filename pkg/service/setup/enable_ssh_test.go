package setup

import "testing"

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
