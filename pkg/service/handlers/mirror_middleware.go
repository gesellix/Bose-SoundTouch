package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MirrorMiddleware returns a middleware that mirrors specific requests to the Bose upstream.
func (s *Server) MirrorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		enabled := s.mirrorEnabled
		endpoints := s.mirrorEndpoints
		s.mu.RUnlock()

		if !enabled || len(endpoints) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		shouldMirror := false

		for _, pattern := range endpoints {
			if matchPattern(pattern, r.URL.Path) {
				shouldMirror = true
				break
			}
		}

		if !shouldMirror {
			next.ServeHTTP(w, r)
			return
		}

		// Buffer request body for both local and mirror
		var bodyBytes []byte
		if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}

		// Prepare local request
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Wrap response writer to capture local response for parity check
		localRecorder := &mirrorResponseRecorder{
			headers: make(http.Header),
			body:    &bytes.Buffer{},
		}

		// Use a multi-writer if RecordMiddleware isn't already doing this,
		// but let's just wrap it.

		wrappedWriter := &parityResponseWriter{
			ResponseWriter: w,
			recorder:       localRecorder,
		}

		if r.Method == http.MethodGet {
			// GET: Local is primary, Mirror is asynchronous
			log.Printf("[MIRROR] Mirroring GET %s asynchronously", r.URL.Path)

			// We need a clone for the async call, detached from original request context
			// We use context.Background() because the original request's context
			// will be canceled as soon as the local handler finishes and returns
			// the response to the speaker.
			//nolint:contextcheck
			rMirror := r.Clone(context.Background())
			rMirror.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// For GET, we run mirror in background and don't wait for parity in real-time
			// or we can wait for local to finish then trigger parity asynchronously.

			next.ServeHTTP(wrappedWriter, r)

			go func() {
				mirrorRes := s.performMirror(rMirror)
				s.checkParity(r, localRecorder, mirrorRes)
			}()
		} else {
			// POST/PUT/DELETE: Local is primary for speaker response, but we sync synchronously
			log.Printf("[MIRROR] Mirroring %s %s synchronously", r.Method, r.URL.Path)

			// We need a clone for the background sync call
			//nolint:contextcheck
			rMirror := r.Clone(context.Background())
			rMirror.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			next.ServeHTTP(wrappedWriter, r)

			go func() {
				mirrorRes := s.performMirror(rMirror)
				s.checkParity(r, localRecorder, mirrorRes)
			}()
		}
	})
}

type parityResponseWriter struct {
	http.ResponseWriter
	recorder *mirrorResponseRecorder
}

func (p *parityResponseWriter) Header() http.Header {
	return p.ResponseWriter.Header()
}

func (p *parityResponseWriter) Write(b []byte) (int, error) {
	p.recorder.body.Write(b)
	return p.ResponseWriter.Write(b)
}

func (p *parityResponseWriter) WriteHeader(statusCode int) {
	p.recorder.status = statusCode
	p.ResponseWriter.WriteHeader(statusCode)
}

func (s *Server) performMirror(r *http.Request) *mirrorResponseRecorder {
	host := r.Host
	if host == "" || host == "localhost" || strings.HasPrefix(host, "127.0.0.1") {
		host = "streaming.bose.com"
	}

	scheme := "https"
	targetURL := scheme + "://" + host

	target, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("[MIRROR_ERR] Failed to parse target URL %s: %v", targetURL, err)
		return nil
	}

	// Create a proxy that doesn't write to the original ResponseWriter
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Record the mirrored request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.Header.Set("X-Mirror-Request", "true")
	}

	// Capture response for parity check and recording
	recorder := &mirrorResponseRecorder{
		headers: make(http.Header),
		body:    &bytes.Buffer{},
	}

	proxy.ModifyResponse = func(res *http.Response) error {
		res.Header.Set("X-Proxy-Origin", "upstream-mirror")

		// Record mirrored interaction
		if s.recorder != nil && s.recordEnabled {
			_ = s.recorder.Record("mirror", r, res)
		}

		return nil
	}

	// We use a dummy ResponseWriter to capture the results
	proxy.ServeHTTP(recorder, r)

	log.Printf("[MIRROR] Mirror completed for %s with status %d", r.URL.Path, recorder.status)

	return recorder
}

func (s *Server) checkParity(req *http.Request, local, upstream *mirrorResponseRecorder) {
	if local.status == 0 {
		local.status = 200
	}

	if upstream.status == 0 {
		upstream.status = 200
	}

	mismatch := false
	reasons := []string{}

	if local.status != upstream.status {
		mismatch = true

		reasons = append(reasons, fmt.Sprintf("Status mismatch: local %d, upstream %d", local.status, upstream.status))
	}

	// Compare Content-Type
	localCT := local.headers.Get("Content-Type")

	upstreamCT := upstream.headers.Get("Content-Type")
	if localCT != upstreamCT {
		mismatch = true

		reasons = append(reasons, fmt.Sprintf("Content-Type mismatch: local %s, upstream %s", localCT, upstreamCT))
	}

	// Basic body comparison (could be improved with XML semantic diff)
	if !bytes.Equal(local.body.Bytes(), upstream.body.Bytes()) {
		mismatch = true

		reasons = append(reasons, "Body content mismatch")
	}

	if mismatch {
		log.Printf("[PARITY] Mismatch detected for %s %s: %v", req.Method, req.URL.Path, reasons)
		s.saveParityMismatch(req, local, upstream, reasons)
	}
}

func (s *Server) saveParityMismatch(req *http.Request, local, upstream *mirrorResponseRecorder, reasons []string) {
	record := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"method":    req.Method,
		"path":      req.URL.Path,
		"reasons":   reasons,
		"local": map[string]interface{}{
			"status": local.status,
			"body":   local.body.String(),
		},
		"upstream": map[string]interface{}{
			"status": upstream.status,
			"body":   upstream.body.String(),
		},
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		log.Printf("[PARITY_ERR] Failed to marshal parity record: %v", err)
		return
	}

	dir := filepath.Join(s.ds.DataDir, "parity_mismatches")
	_ = os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("%d_%s.json", time.Now().Unix(), strings.ReplaceAll(req.URL.Path, "/", "_"))
	_ = os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

type mirrorResponseRecorder struct {
	status  int
	headers http.Header
	body    *bytes.Buffer
}

func (m *mirrorResponseRecorder) Header() http.Header {
	return m.headers
}

func (m *mirrorResponseRecorder) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mirrorResponseRecorder) WriteHeader(statusCode int) {
	m.status = statusCode
}

// matchPattern checks if a path matches a pattern with wildcards (*)
func matchPattern(pattern, name string) bool {
	matched, _ := path.Match(pattern, name)
	if matched {
		return true
	}
	// Also try prefix match if pattern ends with /*
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// HandleListParityMismatches returns a list of parity mismatches.
func (s *Server) HandleListParityMismatches(w http.ResponseWriter, _ *http.Request) {
	dir := filepath.Join(s.ds.DataDir, "parity_mismatches")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))

		return
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var mismatches []interface{}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			data, err := os.ReadFile(filepath.Join(dir, file.Name()))
			if err == nil {
				var record interface{}
				if json.Unmarshal(data, &record) == nil {
					// Add filename as ID for downloading/deletion if needed
					if m, ok := record.(map[string]interface{}); ok {
						m["id"] = file.Name()
						mismatches = append(mismatches, m)
					} else {
						mismatches = append(mismatches, record)
					}
				}
			}
		}
	}

	// Sort by timestamp descending if possible
	sort.Slice(mismatches, func(i, j int) bool {
		mi, oki := mismatches[i].(map[string]interface{})

		mj, okj := mismatches[j].(map[string]interface{})
		if oki && okj {
			ti, _ := mi["timestamp"].(string)
			tj, _ := mj["timestamp"].(string)

			return ti > tj
		}

		return false
	})

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(mismatches); err != nil {
		log.Printf("[PARITY_ERR] Failed to encode mismatches: %v", err)
	}
}

// HandleClearParityMismatches deletes all parity mismatch records.
func (s *Server) HandleClearParityMismatches(w http.ResponseWriter, _ *http.Request) {
	dir := filepath.Join(s.ds.DataDir, "parity_mismatches")
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0755)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte("{\"ok\": true}"))
}
