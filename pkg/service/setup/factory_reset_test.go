package setup

import (
	"errors"
	"strings"
	"testing"
)

func TestFactoryReset_HappyPath(t *testing.T) {
	f := &fakeTelnet{
		banner:    "BoseDebug>",
		responses: map[string]string{"sys factorydefault": "Rebooting...\n"},
	}
	m := newFakeTelnetManager(f)

	logs, err := m.FactoryReset("192.0.2.10")
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}

	if len(f.commands) != 1 || f.commands[0] != "sys factorydefault" {
		t.Errorf("commands = %v, want [sys factorydefault]", f.commands)
	}

	if !strings.Contains(logs, "Rebooting") {
		t.Errorf("logs missing reboot output: %s", logs)
	}
}

func TestFactoryReset_DisconnectIsAcceptedAsSuccess(t *testing.T) {
	// Some firmwares drop the socket as soon as the reset starts, before
	// they finish writing a response. That's not a failure.
	f := &fakeTelnet{
		fail: map[string]error{"sys factorydefault": errors.New("read EOF")},
	}
	m := newFakeTelnetManager(f)

	logs, err := m.FactoryReset("192.0.2.10")
	if err != nil {
		t.Fatalf("disconnect during reset should be treated as success, got: %v", err)
	}

	if !strings.Contains(logs, "device disconnected") {
		t.Errorf("logs should mention the expected disconnect, got: %s", logs)
	}
}

func TestFactoryReset_RejectsFirmwareWithoutCommand(t *testing.T) {
	// Default fakeTelnet response is "Command not found\n" for unmapped commands.
	f := &fakeTelnet{}
	m := newFakeTelnetManager(f)

	_, err := m.FactoryReset("192.0.2.10")
	if err == nil || !strings.Contains(err.Error(), "firmware does not expose") {
		t.Errorf("err = %v, want firmware-rejection error", err)
	}
}

func TestFactoryReset_NoTelnetClient(t *testing.T) {
	m := &Manager{} // NewTelnet nil

	_, err := m.FactoryReset("192.0.2.10")
	if err == nil || !strings.Contains(err.Error(), "NewTelnet") {
		t.Errorf("err = %v, want NewTelnet-nil error", err)
	}
}

func TestFactoryReset_DialFailurePropagates(t *testing.T) {
	f := &fakeTelnet{dialErr: errors.New("connection refused")}
	m := newFakeTelnetManager(f)

	_, err := m.FactoryReset("192.0.2.10")
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("err = %v, want dial error", err)
	}
}
