package discovery

import (
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNSDiscovery_Interception(t *testing.T) {
	serviceIP := "192.168.1.100"
	upstreamDNS := "8.8.8.8"
	d := NewDNSDiscovery(upstreamDNS, serviceIP)

	// Test intercepting Bose service
	m := new(dns.Msg)
	m.SetQuestion("api.bose.com.", dns.TypeA)

	rw := &mockResponseWriter{}
	d.ServeDNS(rw, m)

	if rw.msg == nil {
		t.Fatal("Expected a response message, got nil")
	}

	if len(rw.msg.Answer) == 0 {
		t.Fatal("Expected an answer in the response")
	}

	if a, ok := rw.msg.Answer[0].(*dns.A); ok {
		if a.A.String() != serviceIP {
			t.Errorf("Expected intercepted IP %s, got %s", serviceIP, a.A.String())
		}
	} else {
		t.Errorf("Expected A record, got %T", rw.msg.Answer[0])
	}

	// Test aftertouch.test
	m2 := new(dns.Msg)
	m2.SetQuestion("aftertouch.test.", dns.TypeA)
	rw2 := &mockResponseWriter{}
	d.ServeDNS(rw2, m2)

	if rw2.msg == nil || len(rw2.msg.Answer) == 0 {
		t.Fatal("Expected response for aftertouch.test")
	}

	if a, ok := rw2.msg.Answer[0].(*dns.A); ok {
		if a.A.String() != serviceIP {
			t.Errorf("Expected intercepted IP %s for aftertouch.test, got %s", serviceIP, a.A.String())
		}
	} else {
		t.Errorf("Expected A record for aftertouch.test, got %T", rw2.msg.Answer[0])
	}
}

func TestDNSDiscovery_Forwarding(t *testing.T) {
	// This test is harder because it needs a real upstream or a mock.
	// For now, let's just test that it calls forward and record.
	serviceIP := "192.168.1.100"
	upstreamDNS := "127.0.0.1:5353" // Use a port that is likely closed or we can mock
	d := NewDNSDiscovery(upstreamDNS, serviceIP)

	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)

	rw := &mockResponseWriter{}

	// Start a mock upstream DNS server
	mux := dns.NewServeMux()
	mux.HandleFunc("google.com.", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		_ = w.WriteMsg(m)
	})
	ts := &dns.Server{Addr: "127.0.0.1:5353", Net: "udp", Handler: mux, ReadTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond}
	go func() {
		_ = ts.ListenAndServe()
	}()
	defer func() { _ = ts.Shutdown() }()

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// We expect forward to succeed
	d.ServeDNS(rw, m)

	d.mu.RLock()
	host, exists := d.discovered["google.com"]
	d.mu.RUnlock()

	if !exists {
		t.Error("Expected google.com to be recorded in discovery")
	}
	if host.IsBoseService {
		t.Error("google.com should not be identified as a Bose service")
	}
}

func TestDNSDiscovery_StartTCP(t *testing.T) {
	serviceIP := "192.168.1.100"
	upstreamDNS := "8.8.8.8"
	d := NewDNSDiscovery(upstreamDNS, serviceIP)

	addr := "127.0.0.1:5354"
	go func() {
		_ = d.Start(addr)
	}()

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	// Test TCP resolution
	m := new(dns.Msg)
	m.SetQuestion("api.bose.com.", dns.TypeA)

	c := new(dns.Client)
	c.Net = "tcp"
	in, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("Failed to exchange via TCP: %v", err)
	}

	if len(in.Answer) == 0 {
		t.Fatal("Expected answer in TCP response")
	}

	if a, ok := in.Answer[0].(*dns.A); ok {
		if a.A.String() != serviceIP {
			t.Errorf("Expected intercepted IP %s via TCP, got %s", serviceIP, a.A.String())
		}
	} else {
		t.Errorf("Expected A record via TCP, got %T", in.Answer[0])
	}

	// Test Shutdown
	err = d.Shutdown()
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Verify it's really shut down by trying to connect
	_, _, err = c.Exchange(m, addr)
	if err == nil {
		t.Error("Expected error after shutdown, but could still exchange")
	}
}

func TestDNSDiscovery_IsRunning(t *testing.T) {
	serviceIP := "192.168.1.100"
	upstreamDNS := "8.8.8.8"
	d := NewDNSDiscovery(upstreamDNS, serviceIP)

	addr := "127.0.0.1:5355"

	if d.IsRunning(addr) {
		t.Error("Expected IsRunning to be false before Start")
	}

	go func() {
		_ = d.Start(addr)
	}()

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	if !d.IsRunning(addr) {
		t.Error("Expected IsRunning to be true after Start")
	}

	if d.IsRunning("127.0.0.1:9999") {
		t.Error("Expected IsRunning to be false for wrong address")
	}

	_ = d.Shutdown()

	if d.IsRunning(addr) {
		t.Error("Expected IsRunning to be false after Shutdown")
	}
}

type mockResponseWriter struct {
	msg *dns.Msg
}

func (m *mockResponseWriter) LocalAddr() net.Addr         { return nil }
func (m *mockResponseWriter) RemoteAddr() net.Addr        { return nil }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error { m.msg = msg; return nil }
func (m *mockResponseWriter) Write([]byte) (int, error)   { return 0, nil }
func (m *mockResponseWriter) Close() error                { return nil }
func (m *mockResponseWriter) TsigStatus() error           { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool)         {}
func (m *mockResponseWriter) Hijack()                     {}

func TestDNSDiscovery_LogThrottling(t *testing.T) {
	d := NewDNSDiscovery("8.8.8.8", "192.168.1.100")

	// Capture log output
	var logBuf strings.Builder
	oldOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(oldOutput)

	msg := "Test log message"
	d.throttledLog(msg)
	d.throttledLog(msg)
	d.throttledLog(msg)

	count := strings.Count(logBuf.String(), msg)
	if count != 1 {
		t.Errorf("Expected log message to appear once due to throttling, but appeared %d times", count)
	}

	// Advance time by 11 seconds to bypass throttling
	d.lastLogMu.Lock()
	d.lastLog[msg] = time.Now().Add(-11 * time.Second)
	d.lastLogMu.Unlock()

	d.throttledLog(msg)
	count = strings.Count(logBuf.String(), msg)
	if count != 2 {
		t.Errorf("Expected log message to appear twice after advancing time, but appeared %d times", count)
	}
}

func TestDNSDiscovery_LoopPrevention(t *testing.T) {
	serviceIP := "192.168.1.100"
	bindAddr := "127.0.0.1:53"
	upstreamDNS := "127.0.0.1:53"
	d := NewDNSDiscovery(upstreamDNS, serviceIP)
	d.bindAddr = bindAddr

	// Capture log output to avoid panic if it's being throttled/logged
	var logBuf strings.Builder
	oldOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(oldOutput)

	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)

	rw := &mockResponseWriter{}
	d.forward(rw, m)

	if rw.msg == nil {
		t.Fatal("Expected a response message")
	}

	if rw.msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure (2), got %d", rw.msg.Rcode)
	}
}

func TestDNSDiscovery_EmptyUpstream(t *testing.T) {
	serviceIP := "192.168.1.100"
	upstreamDNS := "" // Empty upstream
	d := NewDNSDiscovery(upstreamDNS, serviceIP)
	d.bindAddr = ":53"

	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)

	rw := &mockResponseWriter{}
	d.ServeDNS(rw, m)

	if rw.msg == nil {
		t.Fatal("Expected a response message, got nil")
	}

	if rw.msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure (2) for empty upstream, got %d", rw.msg.Rcode)
	}
}

func TestDNSDiscovery_ForwardTimeout(t *testing.T) {
	serviceIP := "192.168.1.100"
	// Use an IP that is unroutable or doesn't exist on the network to ensure timeout
	upstreamDNS := "192.0.2.1:53" // TEST-NET-1, usually non-routable
	d := NewDNSDiscovery(upstreamDNS, serviceIP)

	m := new(dns.Msg)
	m.SetQuestion("google.com.", dns.TypeA)

	rw := &mockResponseWriter{}

	start := time.Now()
	d.forward(rw, m)
	duration := time.Since(start)

	if duration < 2*time.Second {
		t.Errorf("Expected forward to take at least 2 seconds (timeout), but took %v", duration)
	}

	if rw.msg == nil || rw.msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("Expected RcodeServerFailure after timeout")
	}
}
