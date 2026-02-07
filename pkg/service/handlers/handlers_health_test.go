package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

type healthResp struct {
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
	Version     string `json:"version"`
	VcsRevision string `json:"vcs_revision"`
	VcsTime     string `json:"vcs_time"`
	VcsModified string `json:"vcs_modified"`
}

func TestHealthEndpoint(t *testing.T) {
	r := chi.NewRouter()
	srv := &Server{}
	r.Get("/health", srv.HandleHealth)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %v", res.Status)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json content type, got %s", ct)
	}

	var hr healthResp
	if err := json.NewDecoder(res.Body).Decode(&hr); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if hr.Status != "up" {
		t.Fatalf("expected status 'up', got %q", hr.Status)
	}
	if hr.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
	if hr.Version == "" {
		t.Error("expected non-empty version")
	}
}
