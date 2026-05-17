package handlers

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/css/* web/js/* web/img/favicon-braille* web/img/favicon*
var webFS embed.FS

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

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(indexHTML)
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
