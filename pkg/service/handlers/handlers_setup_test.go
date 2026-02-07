package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxySettingsAPI(t *testing.T) {
	r, server := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Initial State
	server.proxyRedact = true
	server.proxyLogBody = false

	// 1. Test GET
	res, err := http.Get(ts.URL + "/setup/proxy-settings")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("GET: Expected status OK, got %v", res.Status)
	}

	var settings map[string]bool
	if decodeErr := json.NewDecoder(res.Body).Decode(&settings); decodeErr != nil {
		t.Fatalf("GET: Failed to decode response: %v", decodeErr)
	}

	if settings["redact"] != true || settings["log_body"] != false {
		t.Errorf("GET: Unexpected settings: %+v", settings)
	}

	// 2. Test POST
	update := map[string]bool{
		"redact":   false,
		"log_body": true,
	}

	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Failed to marshal update data: %v", err)
	}

	res, err = http.Post(ts.URL+"/setup/proxy-settings", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("POST: Expected status OK, got %v", res.Status)
	}

	// Verify server state
	if server.proxyRedact != false || server.proxyLogBody != true {
		t.Errorf("POST: Server state did not update: redact=%v, logBody=%v", server.proxyRedact, server.proxyLogBody)
	}

	res, err = http.Get(ts.URL + "/setup/proxy-settings")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if err := json.NewDecoder(res.Body).Decode(&settings); err != nil {
		t.Fatalf("GET (after update): Failed to decode response: %v", err)
	}

	if settings["redact"] != false || settings["log_body"] != true {
		t.Errorf("GET (after update): Unexpected settings: %+v", settings)
	}
}
