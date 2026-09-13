package handlers

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
)

// tuneInStub stands in for opml.radiotime.com and api.radiotime.com so the
// TuneIn handler tests never reach the live service. A CI run once failed
// because the real search API dropped one connection.
type tuneInStub struct {
	URL string

	mu    sync.Mutex
	paths []string
}

// served reports whether the stub answered a request for path.
func (s *tuneInStub) served(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.paths {
		if p == path {
			return true
		}
	}

	return false
}

// stubTuneIn redirects every TuneIn upstream to a local server for the rest of
// the test and restores the real endpoints afterwards.
func stubTuneIn(t *testing.T) *tuneInStub {
	t.Helper()

	stub := &tuneInStub{}

	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.mu.Lock()
		stub.paths = append(stub.paths, r.URL.Path)
		stub.mu.Unlock()

		switch r.URL.Path {
		case "/profiles":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Items":[{"Title":"Stations","Children":[` +
				`{"GuideId":"s100001","Title":"Mock Station","Subtitle":"Test","Image":"http://example.com/logo.png","Type":"Station"}]}]}`))
		case "/Tune.ashx":
			_, _ = w.Write([]byte("https://stream.example.com/mock.mp3\n"))
		case "/describe.ashx":
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte(`<opml><body><outline text="Mock Station" image="http://example.com/logo.png"/></body></opml>`))
		default:
			// Navigate requests, root and sub-pages alike.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"head":{"title":"Browse"},"body":[{"text":"Music","children":[]}]}`))
		}
	}))
	server.Start()

	t.Cleanup(bmx.SaveTuneInEndpoints())
	bmx.SetTuneInEndpoints(server.URL, server.URL)

	stub.URL = server.URL

	return stub
}
