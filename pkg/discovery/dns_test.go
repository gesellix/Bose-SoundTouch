package discovery

import (
	"net"
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
