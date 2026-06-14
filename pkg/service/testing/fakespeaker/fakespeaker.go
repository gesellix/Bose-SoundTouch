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
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

//go:embed testdata/info.xml testdata/presets.xml testdata/recents.xml testdata/networkinfo.xml testdata/sources.xml testdata/supportedurls.xml testdata/now_playing.xml
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

	// FixtureOverrides replaces the response body for the given fixture
	// route (e.g. "/info", "/presets", "/sources") with the supplied
	// bytes. Routes not present in the map fall through to the embedded
	// defaults shipped under testdata/. A nil or empty map keeps the
	// default behaviour the screenshot pipeline relies on.
	//
	// Stateful handlers (/getGroup, /addGroup, /updateGroup,
	// /removeGroup) are not affected — overrides only apply to the
	// GET fixture routes. Use this to wire issue-specific payloads
	// into per-issue regression tests; see
	// pkg/service/setup/issue218_regression_test.go for the pattern.
	FixtureOverrides map[string][]byte
}

// Server is a running fake speaker. It bundles whichever sub-servers
// were enabled in the Config; consult HTTPAddr / TelnetAddr to discover
// where they actually bound.
type Server struct {
	srv      *http.Server
	httpAddr string
	telnet   *telnetServer

	mu            sync.Mutex
	notifications []NotificationCall
}

// NotificationCall records a single POST /notification request the
// fake received. Tests use it to assert that AfterTouch (or any
// other component under test) fired the expected speaker-side
// notification.
type NotificationCall struct {
	// Body is the request body verbatim.
	Body []byte
	// ContentType is the value of the Content-Type header.
	ContentType string
}

// Notifications returns a snapshot of every POST /notification call
// the fake has received, in arrival order. The slice is independent
// of the server's internal state — callers can keep it for assertions
// without holding a lock.
func (s *Server) Notifications() []NotificationCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]NotificationCall, len(s.notifications))
	copy(out, s.notifications)

	return out
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

	s := &Server{
		httpAddr: ln.Addr().String(),
	}

	mux := http.NewServeMux()
	registerRoutes(mux, cfg.FixtureOverrides, s)

	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
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

func registerRoutes(mux *http.ServeMux, overrides map[string][]byte, s *Server) {
	fixture := func(route, embedPath string) {
		mux.HandleFunc(route, serveFixtureOr(embedPath, overrides[route]))
	}

	fixture("/info", "testdata/info.xml")
	fixture("/presets", "testdata/presets.xml")
	fixture("/recents", "testdata/recents.xml")
	fixture("/networkInfo", "testdata/networkinfo.xml")
	fixture("/sources", "testdata/sources.xml")
	fixture("/supportedURLs", "testdata/supportedurls.xml")
	fixture("/now_playing", "testdata/now_playing.xml")

	mux.HandleFunc("/getGroup", serveEmptyGroup)
	mux.HandleFunc("/addGroup", handleAddGroup)
	mux.HandleFunc("/updateGroup", handleUpdateGroup)
	mux.HandleFunc("/removeGroup", handleRemoveGroup)
	mux.HandleFunc("/notification", s.handleNotification)
}

// handleNotification records a POST /notification call so tests can
// assert that AfterTouch fired the expected speaker-side nudge (e.g.
// the <sourcesUpdated/> notification that recovers the source list
// after a factory reset, per issue #234). GET returns 405 — real
// speakers expose /notification as POST-only.
func (s *Server) handleNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))

	s.mu.Lock()
	s.notifications = append(s.notifications, NotificationCall{
		Body:        body,
		ContentType: r.Header.Get("Content-Type"),
	})
	s.mu.Unlock()

	// Real speakers respond with <status>/notification</status>; the
	// pkg/client.Client.NotifySourcesUpdated path validates that
	// shape, so the fake has to match it too.
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?>` + "\n<status>/notification</status>\n"))
}

// serveFixtureOr returns a handler that writes override (when non-nil)
// or the embedded fixture at embedPath (when override is nil). The
// override is snapshotted at construction so later mutations of the
// caller's slice don't change the served body.
func serveFixtureOr(embedPath string, override []byte) http.HandlerFunc {
	if override != nil {
		snapshot := append([]byte(nil), override...)

		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			_, _ = w.Write(snapshot)
		}
	}

	return serveFixture(embedPath)
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

// serveEmptyGroup mirrors a real device's /getGroup response when it is
// not part of a stereo pair: an empty <group/> element. Tests that want
// to assert "no group" round-trip semantics can rely on this shape.
func serveEmptyGroup(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<group/>\n"))
}

// handleAddGroup echoes the posted <group> XML back with
// <status>GROUP_OK</status> appended, matching the success path
// documented for the stereo-pair flow in issue #252 (see also
// soundtouch-cli/cmd_group.go and pkg/service/handlers/handlers_marge.go).
// On GET, returns the same empty-group shape as /getGroup so curl
// smoke-tests don't 405. Anything other than GET/POST gets a 405.
func handleAddGroup(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serveEmptyGroup(w, r)
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	resp := buildAddGroupResponse(body)
	_, _ = w.Write(resp)
}

// handleUpdateGroup mirrors handleAddGroup's contract: POST a <group>
// payload, get the same payload back with <status>GROUP_OK</status>
// appended. Real speakers use this for renames (POST /updateGroup with
// the changed <name>) and other in-place edits to an existing pair.
// GET returns the same empty-group shape /getGroup uses; non-GET/POST
// gets a 405.
func handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		serveEmptyGroup(w, r)
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	resp := buildAddGroupResponse(body)
	_, _ = w.Write(resp)
}

// handleRemoveGroup matches the documented wiki behaviour: GET on the
// master speaker, no body, returns the now-empty group shape. The real
// device dissolves the pair on receipt; the fake is stateless so it
// just always responds as "no group right now".
func handleRemoveGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	serveEmptyGroup(w, r)
}

// buildAddGroupResponse echoes the posted group payload back with a
// <status>GROUP_OK</status> appended, mimicking a real speaker's success
// response. The body is parsed into typed fields and re-marshalled (rather
// than splicing raw bytes) so every echoed value is XML-escaped. If the body
// is empty or unparseable, it falls back to a minimal canned success response
// so callers still see a 200 + parseable XML.
func buildAddGroupResponse(posted []byte) []byte {
	cannedOK := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<group>
    <status>GROUP_OK</status>
</group>
`)

	if len(posted) == 0 {
		return cannedOK
	}

	// Parse into the canonical request type and re-marshal it (rather than
	// splicing the raw bytes) so every echoed value is XML-escaped and the
	// fake stays in lock-step with whatever fields the client actually sends.
	var in models.Group
	if err := xml.Unmarshal(posted, &in); err != nil {
		return cannedOK
	}

	in.Status = "GROUP_OK"

	data, err := xml.Marshal(in)
	if err != nil {
		return cannedOK
	}

	return append([]byte(`<?xml version="1.0" encoding="UTF-8"?>`+"\n"), data...)
}
