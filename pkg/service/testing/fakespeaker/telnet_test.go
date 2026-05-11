package fakespeaker

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTelnetServerBannerAndGetpdo(t *testing.T) {
	s, err := Start(Config{TelnetListen: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	if s.TelnetAddr() == "" {
		t.Fatalf("telnet listener not started")
	}

	conn, err := net.DialTimeout("tcp", s.TelnetAddr(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	reader := bufio.NewReader(conn)

	banner, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read banner: %v", err)
	}

	if !strings.Contains(banner, "Bose SoundTouch") {
		t.Errorf("banner = %q, want substring %q", banner, "Bose SoundTouch")
	}

	if _, err := conn.Write([]byte("getpdo CurrentSystemConfiguration\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 1024)

	var got strings.Builder

	for {
		n, err := conn.Read(buf)
		if n > 0 {
			got.Write(buf[:n])
		}

		if err != nil {
			break
		}

		if strings.Contains(got.String(), "->OK") {
			break
		}
	}

	out := got.String()

	for _, want := range []string{"margeServerUrl", "streaming.bose.com", "swUpdateUrl"} {
		if !strings.Contains(out, want) {
			t.Errorf("response missing %q\nfull response:\n%s", want, out)
		}
	}
}
