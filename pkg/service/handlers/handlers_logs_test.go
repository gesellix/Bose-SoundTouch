package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/logbuf"
	"github.com/go-chi/chi/v5"
)

func newLogsTestServer(t *testing.T, buf *logbuf.Buffer) *httptest.Server {
	t.Helper()

	_, server := setupRouter("http://localhost:8001", nil)
	server.SetLogBuffer(buf)

	r := chi.NewRouter()
	r.Get("/setup/logs", server.HandleGetLogs)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts
}

func TestHandleGetLogs_FullSnapshot(t *testing.T) {
	buf := logbuf.New(16)
	for _, line := range []string{"first\n", "second\n", "third\n"} {
		_, _ = buf.Write([]byte(line))
	}

	ts := newLogsTestServer(t, buf)

	res, err := http.Get(ts.URL + "/setup/logs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var resp logsResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(resp.Entries))
	}

	if resp.Entries[0].Message != "first" || resp.Entries[2].Message != "third" {
		t.Errorf("unexpected ordering: %+v", resp.Entries)
	}

	if resp.NextSince != 3 {
		t.Errorf("nextSince: want 3, got %d", resp.NextSince)
	}

	if resp.Capacity != 16 {
		t.Errorf("capacity: want 16, got %d", resp.Capacity)
	}
}

func TestHandleGetLogs_SinceRoundTrip(t *testing.T) {
	buf := logbuf.New(16)
	for i := 0; i < 5; i++ {
		_, _ = buf.Write([]byte("line\n"))
	}

	ts := newLogsTestServer(t, buf)

	res, err := http.Get(ts.URL + "/setup/logs?since=2")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	var resp logsResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Entries) != 3 {
		t.Errorf("expected 3 entries with since=2, got %d", len(resp.Entries))
	}

	if resp.NextSince != 5 {
		t.Errorf("nextSince: want 5, got %d", resp.NextSince)
	}
}

func TestHandleGetLogs_Limit(t *testing.T) {
	buf := logbuf.New(16)
	for i := 0; i < 8; i++ {
		_, _ = buf.Write([]byte("line\n"))
	}

	ts := newLogsTestServer(t, buf)

	res, err := http.Get(ts.URL + "/setup/logs?limit=3")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	var resp logsResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Entries) != 3 {
		t.Errorf("limit=3 should cap result, got %d entries", len(resp.Entries))
	}
}

func TestHandleGetLogs_MalformedSinceReturns400(t *testing.T) {
	ts := newLogsTestServer(t, logbuf.New(4))

	res, err := http.Get(ts.URL + "/setup/logs?since=notanumber")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", res.StatusCode)
	}
}

func TestHandleGetLogs_NoBufferAttached(t *testing.T) {
	ts := newLogsTestServer(t, nil)

	res, err := http.Get(ts.URL + "/setup/logs")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 even without buffer, got %d", res.StatusCode)
	}

	var resp logsResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Entries) != 0 {
		t.Errorf("expected empty entries when no buffer, got %d", len(resp.Entries))
	}
}

func TestHandleGetLogs_DroppedReportedWhenLagging(t *testing.T) {
	buf := logbuf.New(3)
	for i := 0; i < 10; i++ {
		_, _ = buf.Write([]byte("line\n"))
	}

	ts := newLogsTestServer(t, buf)

	res, err := http.Get(ts.URL + "/setup/logs?since=2")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	var resp logsResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Dropped == 0 {
		t.Errorf("expected dropped > 0 when buffer evicted past `since`, got 0")
	}
}
