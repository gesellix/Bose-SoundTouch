package handlers

import (
	"encoding/json"
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

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected text/html content type, got %s", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "AfterTouch") {
		t.Errorf("Expected body to contain 'AfterTouch', got %s", string(body))
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

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected application/json content type, got %s", contentType)
	}

	var got map[string]string
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode root response: %v", err)
	}

	for _, want := range []struct{ key, value string }{
		{"Bose", "AfterTouch"},
		{"service", "Go/Chi"},
		{"docs", "https://gesellix.github.io/Bose-SoundTouch/"},
	} {
		if got[want.key] != want.value {
			t.Errorf("payload[%q] = %q, want %q", want.key, got[want.key], want.value)
		}
	}

	// Version mirrors /health — falls back to "0.0.1" under `go test`
	// (debug.ReadBuildInfo reports Main.Version="(devel)") but must
	// always be present so monitoring tools can pin a release.
	if got["version"] == "" {
		t.Error("expected version field to be present")
	}
}

func TestStaticMedia(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Use a known file from static/media in a subdirectory
	res, err := http.Get(ts.URL + "/media/bmx-icons/siriusxm-everest/SiriusXM_Logo_Color.svg")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "image/svg+xml") {
		t.Errorf("Expected image/svg+xml content type, got %s", contentType)
	}
}

func TestStaticWeb(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// 1. Test CSS
	res, err := http.Get(ts.URL + "/web/css/style.css")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("CSS: Expected status OK, got %v", res.Status)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "text/css") {
		t.Errorf("CSS: Expected text/css content type, got %s", res.Header.Get("Content-Type"))
	}

	// 2. Test JS
	res, err = http.Get(ts.URL + "/web/js/script.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("JS: Expected status OK, got %v", res.Status)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "application/javascript") &&
		!strings.Contains(res.Header.Get("Content-Type"), "text/javascript") {
		t.Errorf("JS: Expected javascript content type, got %s", res.Header.Get("Content-Type"))
	}

	// 3. Test diff.min.js
	res, err = http.Get(ts.URL + "/web/js/diff.min.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Diff JS: Expected status OK, got %v", res.Status)
	}

	// 4. Test Favicon
	res, err = http.Get(ts.URL + "/web/img/favicon-braille.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Favicon: Expected status OK, got %v", res.Status)
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "image/svg+xml") {
		t.Errorf("Favicon: Expected image/svg+xml content type, got %s", res.Header.Get("Content-Type"))
	}

	// 5. Test old Favicon path (should be 404)
	res, err = http.Get(ts.URL + "/media/favicon-braille.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("Old Favicon: Expected status NotFound, got %v", res.Status)
	}

	// 6. Test old Favicon path in web/ (should be 404)
	res, err = http.Get(ts.URL + "/web/favicon-braille.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("Web Root Favicon: Expected status NotFound, got %v", res.Status)
	}
}
