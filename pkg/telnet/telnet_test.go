package telnet

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedServer is a minimal mock of the device's port-17000 shell. It
// returns the supplied banner on connect, then for each line read it emits
// the corresponding entry from responses (or "Command not found" if the line
// is not in the map).
type scriptedServer struct {
	t         *testing.T
	listener  net.Listener
	banner    string
	responses map[string]string
	// hangAfter, if non-empty, names a command after which the server stops
	// responding (to exercise the read-timeout path).
	hangAfter string
	// closeAfter, if non-empty, names a command after which the server closes
	// the connection mid-stream.
	closeAfter string

	stop chan struct{}
	wg   sync.WaitGroup
}

func newScriptedServer(t *testing.T, banner string, responses map[string]string) *scriptedServer {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &scriptedServer{
		t:         t,
		listener:  l,
		banner:    banner,
		responses: responses,
		stop:      make(chan struct{}),
	}

	s.wg.Add(1)

	go s.serve()

	return s
}

func (s *scriptedServer) addr() string {
	return s.listener.Addr().String()
}

func (s *scriptedServer) hostPort() (string, int) {
	host, portStr, err := net.SplitHostPort(s.addr())
	if err != nil {
		s.t.Fatalf("split host/port: %v", err)
	}

	port := 0

	if _, err := parseInt(portStr, &port); err != nil {
		s.t.Fatalf("parse port %q: %v", portStr, err)
	}

	return host, port
}

func (s *scriptedServer) close() {
	close(s.stop)
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *scriptedServer) serve() {
	defer s.wg.Done()

	conn, err := s.listener.Accept()
	if err != nil {
		return
	}

	defer func() { _ = conn.Close() }()

	if s.banner != "" {
		_, _ = conn.Write([]byte(s.banner))
	}

	r := bufio.NewReader(conn)

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}

		cmd := strings.TrimRight(line, "\r\n")
		if cmd == "" {
			continue
		}

		if cmd == s.closeAfter {
			return
		}

		resp, ok := s.responses[cmd]
		if !ok {
			resp = "Command not found\n"
		}

		_, _ = conn.Write([]byte(resp))

		if cmd == s.hangAfter {
			// Block until the server is closed; the client's read deadline
			// must fire before then.
			<-s.stop
			return
		}
	}
}

// parseInt is a tiny strconv.Atoi wrapper so we don't drag strconv into this file.
func parseInt(s string, out *int) (int, error) {
	n := 0

	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errors.New("not a number")
		}

		n = n*10 + int(ch-'0')
	}

	*out = n

	return n, nil
}

func newClientFor(t *testing.T, s *scriptedServer) *Client {
	t.Helper()

	host, port := s.hostPort()

	c := NewClient(host)
	c.Port = port
	// Tighten the timeouts so tests fail fast if the implementation regresses.
	c.DialTimeout = 500 * time.Millisecond
	c.ReadTimeout = 1500 * time.Millisecond
	c.WriteTimeout = 500 * time.Millisecond

	return c
}

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("192.168.1.10")
	if c.Host != "192.168.1.10" {
		t.Errorf("Host = %q, want 192.168.1.10", c.Host)
	}

	if c.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", c.Port, DefaultPort)
	}

	if c.DialTimeout != DefaultDialTimeout {
		t.Errorf("DialTimeout = %v, want %v", c.DialTimeout, DefaultDialTimeout)
	}
}

func TestDial_Failure(t *testing.T) {
	// A reserved-for-test address that nothing should be listening on.
	c := NewClient("127.0.0.1")
	c.Port = 1 // privileged port, will not connect from a test
	c.DialTimeout = 200 * time.Millisecond

	if err := c.Dial(); err == nil {
		t.Error("expected dial failure, got nil")
	}
}

func TestProbe_ReturnsBanner(t *testing.T) {
	s := newScriptedServer(t, "BoseShell v1\n-> ", nil)
	defer s.close()

	c := newClientFor(t, s)

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	got, err := c.Probe()
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if !strings.Contains(got, "BoseShell v1") {
		t.Errorf("Probe = %q, want to contain banner", got)
	}
}

func TestProbe_NoBannerIsOK(t *testing.T) {
	s := newScriptedServer(t, "", nil)
	defer s.close()

	c := newClientFor(t, s)

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	got, err := c.Probe()
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if got != "" {
		t.Errorf("Probe = %q, want empty when no banner is sent", got)
	}
}

func TestSendCommand_HappyPath(t *testing.T) {
	s := newScriptedServer(t, "", map[string]string{
		"sys configuration bmxRegistryUrl http://example:8000/bmx/registry/v1/services": "OK\n",
		"sys configuration margeServerUrl http://example:8000":                          "OK\n",
		"getpdo CurrentSystemConfiguration":                                             "margeServerUrl=http://example:8000\nbmxRegistryUrl=http://example:8000/bmx/registry/v1/services\n",
	})
	defer s.close()

	c := newClientFor(t, s)

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	resp, err := c.SendCommand("sys configuration margeServerUrl http://example:8000")
	if err != nil {
		t.Fatalf("SendCommand: %v", err)
	}

	if !strings.Contains(resp, "OK") {
		t.Errorf("response = %q, want to contain OK", resp)
	}

	resp, err = c.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil {
		t.Fatalf("SendCommand getpdo: %v", err)
	}

	if !strings.Contains(resp, "margeServerUrl=http://example:8000") {
		t.Errorf("getpdo response = %q, want to echo configured url", resp)
	}
}

func TestSendCommand_CommandNotFound(t *testing.T) {
	s := newScriptedServer(t, "", map[string]string{})
	defer s.close()

	c := newClientFor(t, s)

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	resp, err := c.SendCommand("definitely not a real command")
	if err != nil {
		t.Fatalf("SendCommand: %v", err)
	}

	if !strings.Contains(resp, "Command not found") {
		t.Errorf("response = %q, want to contain 'Command not found'", resp)
	}
}

func TestSendCommand_DeadlineFiresWhenDeviceHangs(t *testing.T) {
	s := newScriptedServer(t, "", map[string]string{
		"first":  "OK\n",
		"second": "",
	})
	s.hangAfter = "second"

	defer s.close()

	c := newClientFor(t, s)
	c.ReadTimeout = 600 * time.Millisecond

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	if _, err := c.SendCommand("first"); err != nil {
		t.Fatalf("first SendCommand: %v", err)
	}

	start := time.Now()

	_, err := c.SendCommand("second")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want timed-out wording", err)
	}

	// The error must arrive within roughly the ReadTimeout, not after several
	// times that — guards against an accidental infinite read loop.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("SendCommand returned after %v, want under 2s", elapsed)
	}
}

func TestSendCommand_ConnectionClosedMidStream(t *testing.T) {
	s := newScriptedServer(t, "", map[string]string{})
	s.closeAfter = "trigger close"

	defer s.close()

	c := newClientFor(t, s)

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer func() { _ = c.Close() }()

	_, err := c.SendCommand("trigger close")
	if err == nil {
		t.Fatal("expected error after server closes mid-stream, got nil")
	}
}

func TestSendCommand_FailsWithoutDial(t *testing.T) {
	c := NewClient("127.0.0.1")
	if _, err := c.SendCommand("anything"); err == nil {
		t.Error("SendCommand without Dial should fail, got nil")
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	s := newScriptedServer(t, "", nil)
	defer s.close()

	c := newClientFor(t, s)

	if err := c.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
