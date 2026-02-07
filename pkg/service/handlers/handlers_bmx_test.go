package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBMXServices(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/bmx/registry/v1/services")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if _, ok := response["bmx_services"]; !ok {
		t.Error("Response missing bmx_services field")
	}

	// Verify placeholder replacement
	bodyStr := string(body)
	if strings.Contains(bodyStr, "{BMX_SERVER}") {
		t.Error("Response still contains {BMX_SERVER} placeholder")
	}
	if strings.Contains(bodyStr, "{MEDIA_SERVER}") {
		t.Error("Response still contains {MEDIA_SERVER} placeholder")
	}
}

func TestOrionPlayback(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Base64 encoded: {"streamUrl": "http://example.com/stream", "imageUrl": "http://example.com/img.jpg", "name": "Test Orion"}
	data := "eyJzdHJlYW1VcmwiOiAiaHR0cDovL2V4YW1wbGUuY29tL3N0cmVhbSIsICJpbWFnZVVybCI6ICJodHRwOi8vZXhhbXBsZS5jb20vaW1nLmpwZyIsICJuYW1lIjogIlRlc3QgT3Jpb24ifQ=="
	res, err := http.Post(ts.URL+"/bmx/orion/v1/playback/station/"+data, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	var resp map[string]interface{}
	json.Unmarshal(body, &resp)

	if resp["name"] != "Test Orion" {
		t.Errorf("Expected name Test Orion, got %v", resp["name"])
	}
}
