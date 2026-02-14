package handlers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
)

// RecordMiddleware returns a middleware that records "self" requests and responses.
func (s *Server) RecordMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.recorder == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Buffer the request body if it exists
		var reqBody []byte
		if r.Body != nil {
			var err error
			reqBody, err = io.ReadAll(r.Body)
			if err == nil {
				r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}
		}

		// wrap ResponseWriter to capture the response
		rw := &responseWriter{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
		}

		next.ServeHTTP(rw, r)

		// Create a response object for the recorder
		res := rw.getRecordedResponse(r)

		// Put back the original request body for recording
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
