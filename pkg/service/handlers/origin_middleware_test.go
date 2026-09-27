package handlers

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureOriginLog runs one request through OriginMiddleware and returns
// what it wrote to the standard logger.
func captureOriginLog(t *testing.T, method, target string, status int) string {
	t.Helper()

	var buf bytes.Buffer

	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)

	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	s := &Server{}
	h := s.OriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, target, nil))

	return buf.String()
}

func TestOriginMiddlewareLogsRequests(t *testing.T) {
	out := captureOriginLog(t, http.MethodGet, "/api/setup/info/DEVICEID01", http.StatusOK)
	if !strings.Contains(out, "[LOG] GET /api/setup/info/DEVICEID01 | 200 | self") {
		t.Fatalf("expected a [LOG] line, got %q", out)
	}
}

// The admin Logs tab polls the log endpoint every 1.5 s while it is open.
// Logging those polls made the page show an endless stream of its own
// requests (issue 728).
func TestOriginMiddlewareSkipsSuccessfulLogPolls(t *testing.T) {
	for _, target := range []string{"/api/setup/logs", "/setup/logs", "/api/setup/logs?since=42"} {
		if out := captureOriginLog(t, http.MethodGet, target, http.StatusOK); out != "" {
			t.Errorf("GET %s: expected no log line, got %q", target, out)
		}
	}
}

func TestOriginMiddlewareStillLogsFailedLogPolls(t *testing.T) {
	out := captureOriginLog(t, http.MethodGet, "/api/setup/logs", http.StatusInternalServerError)
	if !strings.Contains(out, "[LOG] GET /api/setup/logs | 500 | self") {
		t.Fatalf("expected a failed poll to be logged, got %q", out)
	}
}
