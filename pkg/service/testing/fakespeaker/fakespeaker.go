// Package fakespeaker runs a minimal HTTP server that impersonates the
// SoundTouch device's :8090 API surface with sanitized, embedded fixture
// data. It exists so docs/screenshot tooling and integration setups can
// register a "speaker" without depending on real hardware or leaking
// personal data into committed artifacts.
//
// The fixture set is deliberately narrow: enough for the soundtouch-service
// to accept device registration and render initial UI views. Extend the
// route set as additional pre-flight or migration flows need coverage.
package fakespeaker

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

//go:embed testdata/info.xml testdata/presets.xml testdata/recents.xml
var fixtures embed.FS

// Config configures a fake speaker. The zero value is valid and binds the
// HTTP API to a random port on 127.0.0.1 with no telnet listener.
type Config struct {
	// HTTPListen is the bind address for the device's :8090 HTTP API
	// (e.g. "127.0.0.1:8090" or ":8090"). Empty means "127.0.0.1:0" —
	// let the OS pick a port.
	HTTPListen string

	// TelnetListen is the bind address for the device's :17000
	// diagnostic shell. Empty disables the telnet listener entirely.
	// Use "127.0.0.1:17000" to match the real port the wizard probes.
	TelnetListen string
}

// Server is a running fake speaker. It bundles whichever sub-servers
// were enabled in the Config; consult HTTPAddr / TelnetAddr to discover
// where they actually bound.
type Server struct {
	srv      *http.Server
	httpAddr string
	telnet   *telnetServer
}

// Start binds the configured listeners and serves them in background
// goroutines. It returns once they are ready (so callers can immediately
// use the resolved addresses) or with an error if any bind failed.
func Start(cfg Config) (*Server, error) {
	httpListen := cfg.HTTPListen
	if httpListen == "" {
		httpListen = "127.0.0.1:0"
	}

	ln, err := net.Listen("tcp", httpListen)
	if err != nil {
		return nil, fmt.Errorf("fakespeaker: listen %s: %w", httpListen, err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux)

	s := &Server{
		srv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		httpAddr: ln.Addr().String(),
	}

	go func() {
		_ = s.srv.Serve(ln)
	}()

	if cfg.TelnetListen != "" {
		ts, terr := startTelnetServer(cfg.TelnetListen)
		if terr != nil {
			_ = s.srv.Close()
			return nil, terr
		}

		s.telnet = ts
	}

	return s, nil
}

// HTTPAddr returns the resolved HTTP listen address as "host:port".
func (s *Server) HTTPAddr() string {
	return s.httpAddr
}

// TelnetAddr returns the resolved telnet listen address as "host:port",
// or "" if the telnet listener is disabled.
func (s *Server) TelnetAddr() string {
	if s.telnet == nil {
		return ""
	}

	return s.telnet.Addr()
}

// Stop shuts all sub-servers down, blocking until in-flight requests
// finish or ctx is cancelled.
func (s *Server) Stop(ctx context.Context) error {
	if s.telnet != nil {
		s.telnet.Stop()
	}

	if err := s.srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/info", serveFixture("testdata/info.xml"))
	mux.HandleFunc("/presets", serveFixture("testdata/presets.xml"))
	mux.HandleFunc("/recents", serveFixture("testdata/recents.xml"))
}

func serveFixture(path string) http.HandlerFunc {
	body, err := fixtures.ReadFile(path)
	if err != nil {
		// Embed failure is a build-time programmer error; surface it
		// loudly the first time the route is hit.
		return func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fakespeaker: missing fixture "+path+": "+err.Error(), http.StatusInternalServerError)
		}
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write(body)
	}
}
