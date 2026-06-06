package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerURLReachableCheck_PassesWhenVersionEndpointReturns200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/setup/version" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"test"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	got := runServerURLReachableCheck(srv.URL)
	if len(got) != 0 {
		t.Errorf("expected no findings when /api/setup/version returns 200, got %+v", got)
	}
}

func TestServerURLReachableCheck_WarnsWhenVersionEndpointReturns404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	got := runServerURLReachableCheck(srv.URL)
	if len(got) != 1 {
		t.Fatalf("expected one finding when /api/setup/version returns 404, got %+v", got)
	}

	if got[0].Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %v", got[0].Severity)
	}

	if !strings.Contains(got[0].Message, srv.URL) {
		t.Errorf("expected server URL in message, got %q", got[0].Message)
	}
}

func TestServerURLReachableCheck_WarnsWhenServerUnreachable(t *testing.T) {
	// Use a port that is almost certainly not listening.
	got := runServerURLReachableCheck("http://127.0.0.1:19999")
	if len(got) != 1 {
		t.Fatalf("expected one finding for unreachable server, got %+v", got)
	}

	if got[0].Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %v", got[0].Severity)
	}
}

func TestServerURLReachableCheck_EmptyURLIsNoOp(t *testing.T) {
	for _, url := range []string{"", "  "} {
		got := runServerURLReachableCheck(url)
		if len(got) != 0 {
			t.Errorf("expected no findings for empty URL %q, got %+v", url, got)
		}
	}
}
