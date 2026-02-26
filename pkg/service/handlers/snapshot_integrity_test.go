package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
)

func TestSnapshotIntegrity_SelfAndMirror(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "recording-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	recorder := proxy.NewRecorder(tempDir)
	s := NewServer(ds, nil, "http://localhost:8000", false, false, true, false, false)
	s.SetRecorder(recorder)
	s.SetMirrorSettings(true, []string{"/mirror/*"}, "local")

	// Upstream mock
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Request-Body-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	}))
	defer upstream.Close()

	// Configure mirror to point to our mock upstream
	s.SetMirrorSettings(true, []string{"/mirror/*"}, "local")
	// We need to override the host in performMirror but for tests we can just mock it via env if needed or rely on the fact that performMirror uses r.Host

	handler := s.SnapshotMiddleware(s.MirrorMiddleware(s.RecordMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("local response: " + string(body)))
	}))))

	bodyText := `{"test":"integrity"}`
	req := httptest.NewRequest("POST", "http://localhost:8000/mirror/test", strings.NewReader(bodyText))
	req.Header.Set("Content-Type", "application/json")
	// Override r.Host to point to our mock upstream (performMirror will use it)
	req.Host = strings.TrimPrefix(upstream.URL, "http://")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Wait for async operations
	time.Sleep(200 * time.Millisecond)

	var selfFile, mirrorFile string
	_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".http") {
			if strings.Contains(path, "/self/") {
				selfFile = path
			} else if strings.Contains(path, "/mirror/") {
				mirrorFile = path
			}
		}
		return nil
	})

	// Retry a few times for async operations
	for i := 0; i < 10 && (selfFile == "" || mirrorFile == ""); i++ {
		time.Sleep(100 * time.Millisecond)
		_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".http") {
				if strings.Contains(path, "/self/") {
					selfFile = path
				} else if strings.Contains(path, "/mirror/") {
					mirrorFile = path
				}
			}
			return nil
		})
	}

	if selfFile == "" {
		// Try one more scan
		filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() && strings.HasSuffix(path, ".http") {
				if strings.Contains(path, "/self/") {
					selfFile = path
				} else if strings.Contains(path, "/mirror/") {
					mirrorFile = path
				}
			}
			return nil
		})
	}

	if selfFile == "" {
		t.Fatal("Self recording file not found")
	}
	if mirrorFile == "" {
		t.Fatal("Mirror recording file not found")
	}

	selfContent, _ := os.ReadFile(selfFile)
	mirrorContent, _ := os.ReadFile(mirrorFile)

	if !bytes.Contains(selfContent, []byte(bodyText)) {
		t.Errorf("Self recording missing body. Content:\n%s", string(selfContent))
	}
	if !bytes.Contains(mirrorContent, []byte(bodyText)) {
		t.Errorf("Mirror recording missing body. Content:\n%s", string(mirrorContent))
	}
}
