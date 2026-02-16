package handlers

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
)

// HandleProxyRequest handles requests to the logging proxy.
func (s *Server) HandleProxyRequest(w http.ResponseWriter, r *http.Request) {
	targetURLStr := strings.TrimPrefix(r.URL.Path, "/proxy/")
	if targetURLStr == "" {
		http.Error(w, "Target URL is required", http.StatusBadRequest)
		return
	}

	// Reconstruct original URL (it might have lost its double slashes in the path)
	if !strings.HasPrefix(targetURLStr, "http://") && !strings.HasPrefix(targetURLStr, "https://") {
		// Try to fix it if it looks like http:/...
		if strings.HasPrefix(targetURLStr, "http:/") {
			targetURLStr = "http://" + strings.TrimPrefix(targetURLStr, "http:/")
		} else if strings.HasPrefix(targetURLStr, "https:/") {
			targetURLStr = "https://" + strings.TrimPrefix(targetURLStr, "https:/")
		}
	}

	target, err := url.Parse(targetURLStr)
	if err != nil {
		http.Error(w, "Invalid target URL: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.ServeProxy(target)(w, r)
}

// ServeProxy returns a handler that proxies to the given target.
func (s *Server) ServeProxy(target *url.URL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lp := proxy.NewLoggingProxy(target.String(), s.proxyRedact)
		lp.LogBody = s.proxyLogBody
		lp.RecordEnabled = s.recordEnabled
		lp.SetRecorder(s.recorder)

		// Capture request body for recording, as it will be consumed by the proxy
		var reqBody []byte
		if r.Body != nil {
			reqBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		rp := httputil.NewSingleHostReverseProxy(target)
		rp.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		// Update director to set the correct host and path
		originalDirector := rp.Director
		rp.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = target.Host
			// If target has a path, we should probably append or replace.
			// For Bose upstream, it's usually just the domain.
			if target.Path != "" && target.Path != "/" {
				req.URL.Path = target.Path
			}

			lp.LogRequest(req)
		}

		rp.ModifyResponse = func(res *http.Response) error {
			res.Header.Set("X-Proxy-Origin", "upstream")
			// Generic Header Preservation
			if etags, ok := res.Header["Etag"]; ok {
				delete(res.Header, "Etag")
				res.Header["ETag"] = etags
			}

			// Restore captured request body for the recorder
			if reqBody != nil {
				res.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}

			lp.LogResponse(res)

			return nil
		}

		rp.ServeHTTP(w, r)
	}
}

// HandleNotFound handles requests that don't match any route.
func (s *Server) HandleNotFound(w http.ResponseWriter, r *http.Request) {
	if s.enableSoundcorkProxy {
		s.HandleSoundcorkWithFallback(w, r)
		return
	}

	s.HandleBoseProxy(w, r)
}

// HandleSoundcorkWithFallback tries Soundcork first, then Bose if Soundcork returns 404 or fails.
func (s *Server) HandleSoundcorkWithFallback(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(s.soundcorkURL)

	// Buffer request body if any, to allow multiple proxy attempts
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
	}

	// We use a custom response writer to catch 404s
	rw := &fallbackResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		buffer:         &bytes.Buffer{},
	}

	// Create a shallow copy of the request to avoid side effects between attempts
	r2 := r.Clone(r.Context())
	if bodyBytes != nil {
		r2.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	} else {
		r2.Body = nil
	}

	// Remove RequestURI as it's not allowed in client requests
	r2.RequestURI = ""

	s.ServeProxy(target)(rw, r2)

	if rw.statusCode == http.StatusNotFound || rw.statusCode == http.StatusBadGateway || rw.statusCode == http.StatusServiceUnavailable {
		log.Printf("[PROXY] Soundcork returned %d for %s, falling back to Bose", rw.statusCode, r.URL.Path)

		if !rw.wroteHeader {
			// Restore original body if any
			if bodyBytes != nil {
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			s.HandleBoseProxy(w, r)
		}
	}
}

type fallbackResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
	buffer      *bytes.Buffer
}

func (rw *fallbackResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	if code != http.StatusNotFound && code != http.StatusBadGateway && code != http.StatusServiceUnavailable {
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *fallbackResponseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == http.StatusNotFound || rw.statusCode == http.StatusBadGateway || rw.statusCode == http.StatusServiceUnavailable {
		return len(b), nil // Drop the body
	}

	rw.wroteHeader = true

	return rw.ResponseWriter.Write(b)
}

// HandleBoseProxy proxies the request to the Bose upstream.
func (s *Server) HandleBoseProxy(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = "streaming.bose.com"
	}

	// Default to HTTPS for Bose services
	scheme := "https"
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") || strings.HasPrefix(host, "::1") {
		scheme = "http"
	}

	targetURL := scheme + "://" + host

	target, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("[PROXY_ERR] Failed to parse target URL %s: %v", targetURL, err)
		http.Error(w, "Invalid upstream host", http.StatusBadGateway)

		return
	}

	s.ServeProxy(target)(w, r)
}
