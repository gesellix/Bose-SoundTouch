package handlers

import (
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

	lp := proxy.NewLoggingProxy(target.String(), s.proxyRedact)
	lp.LogBody = s.proxyLogBody
	lp.RecordEnabled = s.recordEnabled
	lp.SetRecorder(s.recorder)

	proxy := httputil.NewSingleHostReverseProxy(target)
	// Update director to set the correct host and path
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.URL.Path = target.Path
		req.URL.RawQuery = r.URL.RawQuery
		lp.LogRequest(req)
	}

	proxy.ModifyResponse = func(res *http.Response) error {
		// Generic Header Preservation
		if etags, ok := res.Header["Etag"]; ok {
			delete(res.Header, "Etag")
			res.Header["ETag"] = etags
		}

		lp.LogResponse(res)

		return nil
	}

	proxy.ServeHTTP(w, r)
}
