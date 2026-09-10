//go:build linux && arm

package netcompat

import (
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"testing"
)

// These tests run on any linux/arm machine, including one whose kernel has
// accept4(): AFTERTOUCH_ACCEPT_FALLBACK=1 takes the raw accept() path
// regardless, which is the whole point of the override. Hardware old enough to
// need the fallback for real cannot be put in CI, so this is as close as an
// automated test gets.

func TestForcedFallbackAcceptsConnections(t *testing.T) {
	t.Setenv(FallbackEnv, "1")

	ln, err := Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	defer func() { _ = ln.Close() }()

	fallback, ok := ln.(*fallbackListener)
	if !ok {
		t.Fatalf("Listen returned %T, want *fallbackListener", ln)
	}

	if !fallback.raw.Load() {
		t.Fatal("forced fallback listener is not on the raw accept path")
	}

	const payload = "netcompat"

	accepted := make(chan net.Conn, 1)
	errs := make(chan error, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errs <- err

			return
		}

		accepted <- conn
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = client.Close() }()

	select {
	case err := <-errs:
		t.Fatalf("Accept: %v", err)
	case conn := <-accepted:
		defer func() { _ = conn.Close() }()

		assertKeepAliveEnabled(t, conn)

		if _, err := io.WriteString(conn, payload); err != nil {
			t.Fatalf("write: %v", err)
		}

		_ = conn.Close()
	}

	got, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != payload {
		t.Fatalf("read %q, want %q", got, payload)
	}
}

// TestForcedFallbackAcceptsSequentially covers the part of the raw path that a
// single connection never reaches: draining the queue, parking on EAGAIN when
// it is empty, and waking again for the next client.
func TestForcedFallbackAcceptsSequentially(t *testing.T) {
	t.Setenv(FallbackEnv, "1")

	ln, err := Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	defer func() { _ = ln.Close() }()

	const connections = 5

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < connections; i++ {
			conn, err := ln.Accept()
			if err != nil {
				t.Errorf("Accept %d: %v", i, err)

				return
			}

			_, _ = io.WriteString(conn, fmt.Sprint(i))
			_ = conn.Close()
		}
	}()

	for i := 0; i < connections; i++ {
		client, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("Dial %d: %v", i, err)
		}

		got, err := io.ReadAll(client)
		_ = client.Close()

		if err != nil {
			t.Fatalf("ReadAll %d: %v", i, err)
		}

		if want := fmt.Sprint(i); string(got) != want {
			t.Fatalf("connection %d read %q, want %q", i, got, want)
		}
	}

	wg.Wait()
}

// TestFallbackOffKeepsTheStandardListener guards the promise that the override
// can also switch the fallback out of the way.
func TestFallbackOffKeepsTheStandardListener(t *testing.T) {
	t.Setenv(FallbackEnv, "0")

	ln, err := Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	defer func() { _ = ln.Close() }()

	if _, ok := ln.(*fallbackListener); ok {
		t.Fatal("Listen wrapped the listener even though the fallback was switched off")
	}
}

// assertKeepAliveEnabled checks the socket option itself, because net.FileConn
// does not carry the listener's keep-alive settings over and the service leans
// on them for its long-lived speaker websockets.
func assertKeepAliveEnabled(t *testing.T, conn net.Conn) {
	t.Helper()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Fatalf("accepted connection is %T, want *net.TCPConn", conn)
	}

	rc, err := tcpConn.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn: %v", err)
	}

	var (
		value   int
		optErr  error
		ctrlErr = rc.Control(func(fd uintptr) {
			value, optErr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE)
		})
	)

	if ctrlErr != nil {
		t.Fatalf("Control: %v", ctrlErr)
	}

	if optErr != nil {
		t.Fatalf("GetsockoptInt: %v", optErr)
	}

	if value == 0 {
		t.Fatal("accepted connection has keep-alives disabled")
	}
}
