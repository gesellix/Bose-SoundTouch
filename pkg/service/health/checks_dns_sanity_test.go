package health

import (
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSSanity_NotRunning(t *testing.T) {
	got := runDNSSanityCheck(
		func() (bool, string) { return false, "" },
		func() string { return "192.0.2.10" },
	)

	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding when DNS not running, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "not running") {
		t.Errorf("expected 'not running' in message, got %q", got[0].Message)
	}
}

func TestDNSSanity_NoExpectedIP(t *testing.T) {
	got := runDNSSanityCheck(
		func() (bool, string) { return true, "127.0.0.1:53000" },
		func() string { return "" },
	)

	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning when expectedIP is empty, got %+v", got)
	}
}

func TestDNSSanity_HappyPath(t *testing.T) {
	expectedIP := "192.0.2.10"

	srv := startStubDNSServer(t, func(hostname string) string {
		return expectedIP
	})

	got := runDNSSanityCheck(
		func() (bool, string) { return true, srv },
		func() string { return expectedIP },
	)

	if len(got) != 0 {
		t.Errorf("expected no findings on happy path, got %+v", got)
	}
}

func TestDNSSanity_MismatchedAnswer(t *testing.T) {
	srv := startStubDNSServer(t, func(_ string) string {
		return "203.0.113.99" // wrong IP, doesn't match expected
	})

	got := runDNSSanityCheck(
		func() (bool, string) { return true, srv },
		func() string { return "192.0.2.10" },
	)

	var foundMismatch bool
	for _, f := range got {
		if strings.Contains(f.Message, "unexpected IP") && f.Severity == SeverityWarning {
			foundMismatch = true
			if !strings.Contains(f.Details, "203.0.113.99") {
				t.Errorf("expected actual IP in details, got %q", f.Details)
			}
		}
	}

	if !foundMismatch {
		t.Errorf("expected a mismatch warning, got %+v", got)
	}
}

func TestDNSSanity_UnansweredHostnames(t *testing.T) {
	// Stub returns "" for some hostnames → server emits NXDOMAIN.
	srv := startStubDNSServer(t, func(hostname string) string {
		if strings.Contains(hostname, "streaming") {
			return "" // refuse
		}

		return "192.0.2.10"
	})

	got := runDNSSanityCheck(
		func() (bool, string) { return true, srv },
		func() string { return "192.0.2.10" },
	)

	var foundUnanswered bool
	for _, f := range got {
		if strings.Contains(f.Message, "didn't get an answer") {
			foundUnanswered = true
			if !strings.Contains(f.Message, "streaming") {
				t.Errorf("expected 'streaming' in unanswered list, got %q", f.Message)
			}
		}
	}

	if !foundUnanswered {
		t.Errorf("expected unanswered warning, got %+v", got)
	}
}

func TestExtractHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"127.0.0.1:53", "127.0.0.1"},
		{"0.0.0.0:53", "0.0.0.0"},
		{"[::]:53", "[::]"},
		{"no-port", "no-port"},
	}

	for _, c := range cases {
		if got := extractHost(c.in); got != c.want {
			t.Errorf("extractHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// startStubDNSServer binds a UDP DNS responder on 127.0.0.1 (random
// port). For each A query, answerForHost is called with the queried
// hostname (no trailing dot); a non-empty return is the answer, an
// empty return triggers NXDOMAIN.
func startStubDNSServer(t *testing.T, answerForHost func(string) string) string {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)

		for _, q := range req.Question {
			if q.Qtype != dns.TypeA {
				continue
			}

			name := strings.TrimSuffix(q.Name, ".")
			ip := answerForHost(name)

			if ip == "" {
				resp.SetRcode(req, dns.RcodeNameError)
				continue
			}

			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}

			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   parsed.To4(),
			})
		}

		_ = w.WriteMsg(resp)
	})

	srv := &dns.Server{PacketConn: pc, Handler: mux}
	go func() { _ = srv.ActivateAndServe() }()

	t.Cleanup(func() {
		_ = srv.Shutdown()
		_ = pc.Close()
	})

	return pc.LocalAddr().String()
}
