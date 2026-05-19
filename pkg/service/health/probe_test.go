package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	r := ProbeGet(context.Background(), srv.URL, 2*time.Second)
	if !r.Reachable {
		t.Fatalf("expected Reachable=true, got %+v", r)
	}

	if r.Status != 200 {
		t.Errorf("status: want 200, got %d", r.Status)
	}

	if string(r.Body) != "hello" {
		t.Errorf("body: want 'hello', got %q", r.Body)
	}

	if !strings.Contains(r.CurlCommand, srv.URL) {
		t.Errorf("CurlCommand should include URL, got %q", r.CurlCommand)
	}
}

func TestProbeGet_UnreachableHostPopulatesFallback(t *testing.T) {
	// 127.0.0.1:1 — reliably refused locally; quick to time out.
	r := ProbeGet(context.Background(), "http://127.0.0.1:1/info", 200*time.Millisecond)
	if r.Reachable {
		t.Fatalf("expected Reachable=false for refused port, got %+v", r)
	}

	if r.Err == "" {
		t.Errorf("expected Err to be populated")
	}

	if r.CurlCommand == "" {
		t.Errorf("expected CurlCommand to remain populated on failure")
	}

	if !strings.Contains(r.CurlCommand, "127.0.0.1:1") {
		t.Errorf("curl should still reference the requested URL, got %q", r.CurlCommand)
	}
}

func TestProbeGet_RespectsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	start := time.Now()
	r := ProbeGet(context.Background(), srv.URL, 200*time.Millisecond)
	elapsed := time.Since(start)

	if r.Reachable {
		t.Errorf("expected probe to fail under timeout, got Reachable=true")
	}

	if elapsed > time.Second {
		t.Errorf("timeout not honoured; took %v", elapsed)
	}
}

func TestProbeGet_InvalidURL(t *testing.T) {
	r := ProbeGet(context.Background(), "://broken", 200*time.Millisecond)
	if r.Reachable {
		t.Errorf("expected Reachable=false for invalid URL")
	}

	if r.Err == "" {
		t.Errorf("expected Err to be populated")
	}
}

func TestCurlForGet_QuotesURL(t *testing.T) {
	got := curlForGet("http://192.0.2.10:8090/sources?x=1&y=2")
	if !strings.Contains(got, "'http://192.0.2.10:8090/sources?x=1&y=2'") {
		t.Errorf("expected single-quoted URL in curl, got %q", got)
	}
}
