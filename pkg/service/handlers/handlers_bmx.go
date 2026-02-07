package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/go-chi/chi/v5"
)

func (s *Server) HandleBMXRegistry(w http.ResponseWriter, r *http.Request) {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}

	content := string(bmxServicesJSON)
	content = strings.ReplaceAll(content, "{BMX_SERVER}", baseURL)
	content = strings.ReplaceAll(content, "{MEDIA_SERVER}", baseURL+"/media")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(content))
}

func (s *Server) HandleTuneInPlayback(w http.ResponseWriter, r *http.Request) {
	stationID := chi.URLParam(r, "stationID")
	resp, err := bmx.TuneInPlayback(stationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) HandleTuneInPodcastInfo(w http.ResponseWriter, r *http.Request) {
	podcastID := chi.URLParam(r, "podcastID")
	encodedName := r.URL.Query().Get("encoded_name")
	resp, err := bmx.TuneInPodcastInfo(podcastID, encodedName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) HandleTuneInPlaybackPodcast(w http.ResponseWriter, r *http.Request) {
	podcastID := chi.URLParam(r, "podcastID")
	resp, err := bmx.TuneInPlaybackPodcast(podcastID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) HandleOrionPlayback(w http.ResponseWriter, r *http.Request) {
	data := chi.URLParam(r, "data")
	resp, err := bmx.PlayCustomStream(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
