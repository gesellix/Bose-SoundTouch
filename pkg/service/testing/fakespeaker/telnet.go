package fakespeaker

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// telnetBanner mimics what a real SoundTouch device emits on connect to
// :17000. The exact wording is not load-bearing for the migration UI —
// only TelnetReachable is — but a non-empty banner matches the production
// shape and gets surfaced in the wizard for diagnostic value.
const telnetBanner = "Welcome to the Bose SoundTouch diagnostic shell\r\n"

// telnetGetpdoResponse simulates the protobuf-text-like reply to
// `getpdo CurrentSystemConfiguration` for an *unmigrated* speaker — every
// URL still points at the Bose cloud. This is the happy path for a
// documentation screenshot: the wizard renders as "Not Migrated", lists
// the original URLs, and offers the migration plan.
//
// The shape matches what preflight_crosscheck.parseGetpdoConfig expects:
// "<key> {\n  text: \"<value>\"\n}".
const telnetGetpdoResponse = `margeServerUrl {
  text: "https://streaming.bose.com"
}
statsServerUrl {
  text: "https://stats.bose.com"
}
swUpdateUrl {
  text: "https://worldwide.bose.com/updates/soundtouch"
}
bmxRegistryUrl {
  text: "https://bmxservice.bose.com/bmx/registry/v1/services"
}
->OK
`

// telnetServer is a minimal TCP server that satisfies the read-only pre-flight
// probe in pkg/service/setup/telnet_preflight.go. It handles only the commands
// the wizard actually issues and answers every other line with a stub.
type telnetServer struct {
	ln   net.Listener
	addr string
	wg   sync.WaitGroup
	once sync.Once
	done chan struct{}
}

func startTelnetServer(listen string) (*telnetServer, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, fmt.Errorf("fakespeaker telnet: listen %s: %w", listen, err)
	}

	s := &telnetServer{
		ln:   ln,
		addr: ln.Addr().String(),
		done: make(chan struct{}),
	}

	s.wg.Add(1)

	go s.accept()

	return s, nil
}

func (s *telnetServer) Addr() string {
	return s.addr
}

func (s *telnetServer) Stop() {
	s.once.Do(func() {
		close(s.done)
		_ = s.ln.Close()
	})
	s.wg.Wait()
}

func (s *telnetServer) accept() {
	defer s.wg.Done()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			// Listener closed → graceful shutdown; any other error means
			// the OS gave up on us and we should also stop.
			if errors.Is(err, net.ErrClosed) {
				return
			}

			select {
			case <-s.done:
				return
			default:
			}

			continue
		}

		s.wg.Add(1)

		go s.handle(conn)
	}
}

func (s *telnetServer) handle(conn net.Conn) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	// Banner on connect — clients read it via Probe() before any command.
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

	if _, err := conn.Write([]byte(telnetBanner)); err != nil {
		return
	}

	reader := bufio.NewReader(conn)

	for {
		// No idle deadline — let the client drive the cadence. The client
		// closes the socket after it has its answer (~600 ms idle window),
		// which surfaces here as io.EOF and ends the loop.
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		resp := respondTo(strings.TrimRight(line, "\r\n"))
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))

		if _, werr := conn.Write([]byte(resp)); werr != nil {
			return
		}
	}
}

func respondTo(cmd string) string {
	cmd = strings.TrimSpace(cmd)

	switch cmd {
	case "getpdo CurrentSystemConfiguration":
		return telnetGetpdoResponse
	case "":
		return "->OK\r\n"
	default:
		// Unrecognized commands get a benign acknowledgement so the
		// probe loop never hangs waiting for a response.
		return "->OK\r\n"
	}
}
