package handlers

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/css/* web/js/*
var webFS embed.FS

//go:embed static/media/*
var mediaFS embed.FS

//go:embed static/bmx_services.json
var bmxServicesJSON []byte

//go:embed static/swupdate.xml
var swUpdateXML []byte

// HandleRoot returns the root endpoint response.
func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "text/html") && (strings.Contains(accept, "application/json") || accept == "*/*" || accept == "") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Bose": "AfterTouch", "service": "Go/Chi", "docs": "https://gesellix.github.io/Bose-SoundTouch/"}`)

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
		fs := http.StripPrefix("/media/", http.FileServer(http.FS(subFS)))
		fs.ServeHTTP(w, r)
	}
}
