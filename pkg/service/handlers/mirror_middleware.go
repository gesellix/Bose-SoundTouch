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
		enabled, endpoints, preferredSource := s.getMirrorSettings()

		if !enabled || len(endpoints) == 0 || !s.shouldMirror(r.URL.Path, endpoints) {
			next.ServeHTTP(w, r)
			return
		}

		// Try to fetch snapshot from context
		var snapshot *RequestSnapshot
		if snap, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot); ok {
			snapshot = snap
		}

		// Buffer request body if snapshot is missing (compatibility mode)
		var bodyBytes []byte
		if snapshot != nil {
			bodyBytes = snapshot.Body
		} else if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Use request context but detach it for background operations to prevent cancellation when the primary request finishes
		detachedCtx := context.WithoutCancel(r.Context())
		if snapshot != nil {
			detachedCtx = context.WithValue(detachedCtx, SnapshotKey, snapshot)
		}

		if preferredSource == "upstream" {
			s.mirrorUpstreamPreferred(detachedCtx, w, r, next, bodyBytes)
			return
		}

		s.mirrorLocalPreferred(detachedCtx, w, r, next, bodyBytes)
	})
}

func (s *Server) getMirrorSettings() (bool, []string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.mirrorEnabled, s.mirrorEndpoints, s.preferredSource
}

func (s *Server) shouldMirror(path string, endpoints []string) bool {
	for _, pattern := range endpoints {
		if matchPattern(pattern, path) {
			return true
		}
	}

	return false
}

func (s *Server) mirrorUpstreamPreferred(detachedCtx context.Context, w http.ResponseWriter, r *http.Request, next http.Handler, bodyBytes []byte) {
	log.Printf("[MIRROR] Upstream is preferred source for %s %s", r.Method, r.URL.Path)

	// Clone request for local execution
	rLocal := r.Clone(detachedCtx)
	rLocal.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	localRecorder := &mirrorResponseRecorder{
		headers: make(http.Header),
		body:    &bytes.Buffer{},
	}

	// Run local handler in background
	localDone := make(chan struct{})

	go func() {
		next.ServeHTTP(localRecorder, rLocal)
		close(localDone)
	}()

	// Clone request for mirror execution
	rMirror := r.Clone(detachedCtx)
	rMirror.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Execute mirror synchronously
	mirrorRes := s.performMirror(rMirror)

	// Send mirror response to client
	if mirrorRes != nil && mirrorRes.status != 0 && mirrorRes.status < 500 {
		for k, vv := range mirrorRes.headers {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}

		w.WriteHeader(mirrorRes.status)
		_, _ = w.Write(mirrorRes.body.Bytes())
	} else {
		// Fallback to local if mirror failed
		log.Printf("[MIRROR_ERR] Mirror failed, falling back to local for %s", r.URL.Path)
		<-localDone

		for k, vv := range localRecorder.headers {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}

		if localRecorder.status == 0 {
			localRecorder.status = http.StatusOK
		}

		w.WriteHeader(localRecorder.status)
		_, _ = w.Write(localRecorder.body.Bytes())
	}

	// Perform parity check once local is done
	go func() {
		<-localDone

		if mirrorRes != nil {
			s.checkParity(r, localRecorder, mirrorRes)
		}
	}()
}

func (s *Server) mirrorLocalPreferred(detachedCtx context.Context, w http.ResponseWriter, r *http.Request, next http.Handler, bodyBytes []byte) {
	// Default: local is preferred source of truth
	// Prepare local request
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Wrap response writer to capture local response for parity check
	localRecorder := &mirrorResponseRecorder{
		headers: make(http.Header),
		body:    &bytes.Buffer{},
	}

	wrappedWriter := &parityResponseWriter{
		ResponseWriter: w,
		recorder:       localRecorder,
	}

	log.Printf("[MIRROR] Mirroring %s %s %s", r.Method, r.URL.Path, map[bool]string{true: "asynchronously", false: "synchronously"}[r.Method == http.MethodGet])

	rMirror := r.Clone(detachedCtx)
	rMirror.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	next.ServeHTTP(wrappedWriter, r)

	go func() {
		mirrorRes := s.performMirror(rMirror)
		s.checkParity(r, localRecorder, mirrorRes)
	}()
}

type parityResponseWriter struct {
	http.ResponseWriter
	recorder *mirrorResponseRecorder
}

func (p *parityResponseWriter) Header() http.Header {
	return p.recorder.Header()
}

func (p *parityResponseWriter) Write(b []byte) (int, error) {
	if p.recorder.status == 0 {
		p.WriteHeader(http.StatusOK)
	}

	p.recorder.body.Write(b)

	return p.ResponseWriter.Write(b)
}

func (p *parityResponseWriter) WriteHeader(statusCode int) {
	p.recorder.status = statusCode
	// Copy headers to the real response writer before writing the header
	for k, vv := range p.recorder.headers {
		for _, v := range vv {
			p.ResponseWriter.Header().Add(k, v)
		}
	}

	p.ResponseWriter.WriteHeader(statusCode)
}

func (s *Server) performMirror(r *http.Request) *mirrorResponseRecorder {
	// Try to fetch snapshot from context
	var snapshot *RequestSnapshot
	if snap, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot); ok {
		snapshot = snap
	}

	// Preserve request body for recording before it gets consumed by the proxy
	var requestForRecording *http.Request
	if s.recorder != nil && s.recordEnabled {
		requestForRecording = r.Clone(r.Context())
		if snapshot != nil {
			// Use snapshot for both proxy and recording
			r.Body = io.NopCloser(bytes.NewReader(snapshot.Body))
			requestForRecording.Body = io.NopCloser(bytes.NewReader(snapshot.Body))
		} else if r.Body != nil {
			// Compatibility fallback
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("[MIRROR_ERR] Failed to read request body for recording: %v", err)
			} else {
				// Restore body for proxy
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				// Set body for recording
				requestForRecording.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		// Ensure Content-Length is set for the recording clone
		if requestForRecording.Body != nil {
			if snapshot != nil {
				requestForRecording.ContentLength = int64(len(snapshot.Body))
			}
		}
	}

	host := r.Host
	if host == "" || host == "localhost" {
		host = "streaming.bose.com"
	}

	scheme := "https"
	if strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "localhost") {
		scheme = "http"
	}

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

		// Record mirrored interaction with preserved request body
		if s.recorder != nil && s.recordEnabled && requestForRecording != nil {
			_ = s.recorder.Record("mirror", requestForRecording, res)
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
			"status":  local.status,
			"headers": local.headers,
			"body":    local.body.String(),
		},
		"upstream": map[string]interface{}{
			"status":  upstream.status,
			"headers": upstream.headers,
			"body":    upstream.body.String(),
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
