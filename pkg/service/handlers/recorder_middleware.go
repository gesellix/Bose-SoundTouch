package handlers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
)

// RecordMiddleware returns a middleware that records "self" requests and responses.
func (s *Server) RecordMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.recorder == nil || !s.recordEnabled {
			next.ServeHTTP(w, r)
			return
		}

		s.mu.RLock()
		internalPaths := s.internalPaths
		s.mu.RUnlock()

		for _, pattern := range internalPaths {
			if matched, _ := path.Match(pattern, r.URL.Path); matched {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Use snapshot if available, otherwise buffer body (compatibility mode)
		var snapshot *RequestSnapshot
		if s, ok := r.Context().Value(SnapshotKey).(*RequestSnapshot); ok {
			snapshot = s
		}

		var reqBody []byte
		if snapshot != nil {
			reqBody = snapshot.Body
		} else if r.Body != nil {
			reqBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		// wrap ResponseWriter to capture the response
		rw := &responseWriter{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
		}

		next.ServeHTTP(rw, r)

		// Create a response object for the recorder
		res := rw.getRecordedResponse(r)
		if res.Body != nil {
			defer func() { _ = res.Body.Close() }()
		}

		// Restore body for recording
		r.Body = io.NopCloser(bytes.NewBuffer(reqBody))

		_ = s.recorder.Record("self", r, res)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (rw *responseWriter) Header() http.Header {
	return rw.ResponseWriter.Header()
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	// lgtm[go/reflected-xss] — this middleware is a transparent passthrough;
	// the data written here originates from XML API handlers (Content-Type:
	// application/vnd.bose.streaming-v1.2+xml), not from HTML-rendered pages.
	// Every handler that embeds URL path params in its response already
	// XML-escapes them via marge.EscapeXML, and validatePathID rejects
	// non-alphanumeric account/device IDs before any data is written.
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) getRecordedResponse(r *http.Request) *http.Response {
	statusCode := rw.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	return &http.Response{
		StatusCode: statusCode,
		Header:     rw.ResponseWriter.Header(),
		Body:       io.NopCloser(bytes.NewBuffer(rw.body.Bytes())),
		Request:    r,
	}
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}

	return nil, nil, fmt.Errorf("ResponseWriter does not support Hijacker")
}
