package handlers

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed index.html
var indexHTML []byte

//go:embed soundcork/media/*
var mediaFS embed.FS

//go:embed soundcork/bmx_services.json
var bmxServicesJSON []byte

//go:embed soundcork/swupdate.xml
var swUpdateXML []byte

func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "text/html") && (strings.Contains(accept, "application/json") || accept == "*/*" || accept == "") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Bose": "Can't Brick Us", "service": "Go/Chi"}`)

		return
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(indexHTML)
}

func (s *Server) HandleMedia() http.HandlerFunc {
	subFS, _ := fs.Sub(mediaFS, "soundcork/media")

	return func(w http.ResponseWriter, r *http.Request) {
		fs := http.StripPrefix("/media/", http.FileServer(http.FS(subFS)))
		fs.ServeHTTP(w, r)
	}
}
