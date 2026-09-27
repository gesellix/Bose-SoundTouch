package handlers

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/landing.html
var landingHTML []byte

// landingHTMLPlayerOff is the chooser while the embedded player is disabled
// (issue 762): the Player entry stays, muted and without a link, and says
// where to turn it back on. Derived once from landingHTML so the page keeps a
// single source.
var landingHTMLPlayerOff = mutePlayerEntry(landingHTML)

const (
	landingPlayerLink    = `<a class="dest dest--player" href="/app">`
	landingPlayerLinkOff = `<a class="dest dest--player dest--off" aria-disabled="true">`
	landingPlayerDesc    = `<span class="desc">Playback, volume, presets, zones, and TuneIn / RadioBrowser.</span>`
	landingPlayerDescOff = `<span class="desc">Turned off in the admin console's Settings.</span>`
)

func mutePlayerEntry(page []byte) []byte {
	out := bytes.Replace(page, []byte(landingPlayerLink), []byte(landingPlayerLinkOff), 1)

	return bytes.Replace(out, []byte(landingPlayerDesc), []byte(landingPlayerDescOff), 1)
}

//go:embed web/css/* web/js/* web/img/favicon-braille* web/img/favicon*
var webFS embed.FS

// indexHTMLVersioned is the HTML the root handler serves: identical to
// indexHTML except the script.js and style.css references carry a
// ?v=<hash> query string so the browser cache invalidates whenever
// the asset content changes. Computed once at package init and reused
// per-request. webAssetHash is the truncated SHA-256 over the asset
// bodies; it's exposed for /setup/settings consumers that want to
// build versioned URLs against /web/* from their own DOM constructors.
var (
	indexHTMLVersioned []byte
	webAssetHash       string
)

func init() {
	webAssetHash = computeWebAssetHash()
	indexHTMLVersioned = applyAssetVersionToHTML(indexHTML, webAssetHash)
}

// computeWebAssetHash hashes the embedded script.js and style.css
// bodies into a short stable identifier. SHA-256 truncated to 12
// hex chars is more than enough to detect content changes across
// release builds without bloating the URL.
func computeWebAssetHash() string {
	h := sha256.New()

	for _, path := range []string{"web/js/script.js", "web/css/style.css"} {
		data, err := webFS.ReadFile(path)
		if err != nil {
			continue
		}

		_, _ = h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil))[:12]
}

// applyAssetVersionToHTML rewrites the script and stylesheet src/href
// attributes in the embedded HTML to carry a ?v=<hash> query string.
// Operates on the byte slice once at startup; HandleRoot then serves
// the cached output verbatim per request.
func applyAssetVersionToHTML(html []byte, hash string) []byte {
	if hash == "" {
		return html
	}

	out := string(html)
	out = strings.Replace(out, `href="/web/css/style.css"`, `href="/web/css/style.css?v=`+hash+`"`, 1)
	out = strings.Replace(out, `src="/web/js/script.js"`, `src="/web/js/script.js?v=`+hash+`"`, 1)

	return []byte(out)
}

//go:embed static/media/*
var mediaFS embed.FS

//go:embed static/ced
var cedFS embed.FS

//go:embed static/bmx_services.json
var bmxServicesJSON []byte

//go:embed static/bmx_services_availability.json
var bmxServicesAvailabilityJSON []byte

// Upstream source available at https://worldwide.bose.com/updates/soundtouch?serialnumber=_serial_
// which results in a redirect to https://downloads.bose.com/ced/soundtouch/mr4_22097fe2/index.xml
//
//go:embed static/swupdate.xml
var swUpdateXML []byte

// HandleRoot returns the root endpoint response.
func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "text/html") && (strings.Contains(accept, "application/json") || accept == "*/*" || accept == "") {
		// Mirror the version + VCS metadata exposed by /health so any
		// caller hitting "/" gets identical release context. Keys that
		// would carry empty strings are omitted by buildVersionInfo.
		payload := map[string]string{
			"Bose":    "AfterTouch",
			"service": "Go/Chi",
			"docs":    "https://gesellix.github.io/Bose-SoundTouch/",
		}

		for k, v := range buildVersionInfo() {
			payload[k] = v
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(payload); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	// HTML branch: a browser hitting "/". Honour the configured default
	// landing surface, otherwise serve the neutral chooser. A "?chooser"
	// query forces the chooser even when a default redirect is set, so the
	// hub (and through it the admin console) stays reachable: the "home"
	// links on the player and admin point here. The admin console itself
	// lives at /admin (served by HandleAdmin).
	if !r.URL.Query().Has("chooser") {
		switch s.defaultLanding() {
		case "app":
			// Falls through to the chooser when the player is disabled
			// (issue 762) instead of redirecting into a route that would
			// just 404 with the disabled message.
			if s.PlayerEnabled() {
				http.Redirect(w, r, "/app", http.StatusFound)
				return
			}
		case "admin":
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}
	}

	page := landingHTML
	if !s.PlayerEnabled() {
		page = landingHTMLPlayerOff
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(page)
}

// HandleAdmin serves the admin / setup console. It used to live at "/";
// the chooser landing page took that spot, so the console moved here.
// Its assets (/web/*) and APIs (/setup, /mgmt) are absolute, so the page
// works unchanged at the new path.
func (s *Server) HandleAdmin(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(indexHTMLVersioned)
}

// defaultLanding returns the configured root-path behaviour for browsers
// ("chooser", "app", or "admin"), defaulting to "chooser" when unset or
// unreadable.
func (s *Server) defaultLanding() string {
	persisted, err := s.ds.GetSettings()
	if err != nil || persisted.DefaultLanding == "" {
		return "chooser"
	}

	return persisted.DefaultLanding
}

// HandleWeb returns a handler for serving web resources.
func (s *Server) HandleWeb() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fs := http.FileServer(http.FS(webFS))
		fs.ServeHTTP(w, r)
	}
}

// HandleMedia returns a handler for serving media files.
func (s *Server) HandleMedia() http.HandlerFunc {
	subFS, _ := fs.Sub(mediaFS, "static/media")

	return func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/media", http.FileServer(http.FS(subFS))).ServeHTTP(w, r)
	}
}

// HandleBmxIcons returns a handler for serving BMX icon assets (media.bose.io /bmx-icons/*).
func (s *Server) HandleBmxIcons() http.HandlerFunc {
	subFS, _ := fs.Sub(mediaFS, "static/media")

	return func(w http.ResponseWriter, r *http.Request) {
		http.FileServer(http.FS(subFS)).ServeHTTP(w, r)
	}
}

// HandleCedStatic returns a handler for serving downloads.bose.com CED static files.
func (s *Server) HandleCedStatic() http.HandlerFunc {
	subFS, _ := fs.Sub(cedFS, "static/ced")

	return func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/ced", http.FileServer(http.FS(subFS))).ServeHTTP(w, r)
	}
}
