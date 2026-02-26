package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSnapshotMiddleware(t *testing.T) {
	s := &Server{}

	t.Run("CapturesBodyAndMetadata", func(t *testing.T) {
		bodyText := "hello world"
		req := httptest.NewRequest("POST", "http://example.com/foo?bar=baz", bytes.NewBufferString(bodyText))
		req.Header.Set("Content-Type", "text/plain")
		req.Host = "example.com"

		recorded := false
		handler := s.SnapshotMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorded = true

			// Verify snapshot in context
			snapshot, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot)
			if !ok {
				t.Fatal("Snapshot not found in context")
			}

			if snapshot.Method != "POST" {
				t.Errorf("Expected method POST, got %s", snapshot.Method)
			}
			if snapshot.URL.Path != "/foo" {
				t.Errorf("Expected path /foo, got %s", snapshot.URL.Path)
			}
			if snapshot.Headers.Get("Content-Type") != "text/plain" {
				t.Errorf("Expected header text/plain, got %s", snapshot.Headers.Get("Content-Type"))
			}
			if string(snapshot.Body) != bodyText {
				t.Errorf("Expected body %s, got %s", bodyText, string(snapshot.Body))
			}
			if snapshot.Host != "example.com" {
				t.Errorf("Expected host example.com, got %s", snapshot.Host)
			}

			// Verify r.Body is still readable
			body, _ := io.ReadAll(r.Body)
			if string(body) != bodyText {
				t.Errorf("Expected r.Body to be %s, got %s", bodyText, string(body))
			}
		}))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !recorded {
			t.Error("Handler was not called")
		}
	})

	t.Run("HandlesEmptyBody", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/foo", nil)

		handler := s.SnapshotMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			snapshot, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot)
			if !ok {
				t.Fatal("Snapshot not found in context")
			}
			if len(snapshot.Body) != 0 {
				t.Errorf("Expected empty body, got %d bytes", len(snapshot.Body))
			}
		}))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	})

	t.Run("RespectsSizeLimit", func(t *testing.T) {
		largeBody := make([]byte, 3*1024*1024) // 3MB
		for i := range largeBody {
			largeBody[i] = 'A'
		}

		req := httptest.NewRequest("POST", "http://example.com/foo", bytes.NewReader(largeBody))

		handler := s.SnapshotMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			snapshot, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot)
			if !ok {
				t.Fatal("Snapshot not found in context")
			}

			const maxBodySize = 2 * 1024 * 1024
			if len(snapshot.Body) != maxBodySize {
				t.Errorf("Expected body size %d, got %d", maxBodySize, len(snapshot.Body))
			}
		}))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	})
}
