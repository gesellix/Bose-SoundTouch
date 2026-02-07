package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootEndpoint(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Header.Set("Accept", "text/html")

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected text/html content type, got %s", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "Soundcork Management") {
		t.Errorf("Expected body to contain 'Soundcork Management', got %s", string(body))
	}
}

func TestRootEndpointJSON(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected application/json content type, got %s", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	expected := `{"Bose": "Can't Brick Us", "service": "Go/Chi"}`
	if strings.TrimSpace(string(body)) != expected {
		t.Errorf("Expected body %s, got %s", expected, string(body))
	}
}

func TestStaticMedia(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Use a known file from soundcork/media
	res, err := http.Get(ts.URL + "/media/SiriusXM_Logo_Color.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "image/svg+xml") {
		t.Errorf("Expected image/svg+xml content type, got %s", contentType)
	}
}
