package netcompat

import (
	"io"
	"net"
	"testing"
)

func TestRequestedFallbackReadsTheEnvironment(t *testing.T) {
	tests := []struct {
		value string
		want  fallbackMode
	}{
		{value: "", want: fallbackAuto},
		{value: "auto", want: fallbackAuto},
		{value: "AUTO", want: fallbackAuto},
		{value: "  auto  ", want: fallbackAuto},
		{value: "1", want: fallbackForceOn},
		{value: "true", want: fallbackForceOn},
		{value: "On", want: fallbackForceOn},
		{value: "yes", want: fallbackForceOn},
		{value: "0", want: fallbackForceOff},
		{value: "false", want: fallbackForceOff},
		{value: "OFF", want: fallbackForceOff},
		{value: "no", want: fallbackForceOff},
		// A typo must not keep the service from starting.
		{value: "maybe", want: fallbackAuto},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv(FallbackEnv, tt.value)

			if got := requestedFallback(); got != tt.want {
				t.Fatalf("requestedFallback() with %s=%q = %v, want %v", FallbackEnv, tt.value, got, tt.want)
			}
		})
	}
}

// TestListenAcceptsConnections is the contract every build has to keep,
// fallback or not: Listen hands back a listener that accepts and carries bytes.
func TestListenAcceptsConnections(t *testing.T) {
	ln, err := Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	defer func() { _ = ln.Close() }()

	const payload = "netcompat"

	done := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err

			return
		}

		defer func() { _ = conn.Close() }()

		_, err = io.WriteString(conn, payload)
		done <- err
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = conn.Close() }()

	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != payload {
		t.Fatalf("read %q, want %q", got, payload)
	}

	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
}

// TestListenRejectsABadAddress checks that Listen does not swallow or reshape
// the error net.Listen produces.
func TestListenRejectsABadAddress(t *testing.T) {
	ln, err := Listen("tcp", "127.0.0.1:not-a-port")
	if err == nil {
		_ = ln.Close()

		t.Fatal("Listen succeeded on an unparseable address")
	}
}
