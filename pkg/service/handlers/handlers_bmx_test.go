package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	defer func() { _ = res.Body.Close() }()

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
	if !strings.Contains(bodyStr, "http://localhost:8001") {
		t.Errorf("Response does not contain expected baseURL http://localhost:8001, got: %s", bodyStr)
	}

	if strings.Contains(bodyStr, "{BMX_SERVER}") {
		t.Error("Response still contains {BMX_SERVER} placeholder")
	}

	if strings.Contains(bodyStr, "{MEDIA_SERVER}") {
		t.Error("Response still contains {MEDIA_SERVER} placeholder")
	}
}

func TestBMXServices_EmptyBaseURL(t *testing.T) {
	r, _ := setupRouter("", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/bmx/registry/v1/services")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)

	// Since we removed the fallback, it should use the empty baseURL
	if strings.Contains(bodyStr, "http://localhost:8000") {
		t.Error("Response contains fallback URL http://localhost:8000, which should be removed")
	}

	if strings.Contains(bodyStr, "{BMX_SERVER}") {
		t.Error("Response still contains {BMX_SERVER} placeholder")
	}
}

func TestOrionPlayback(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Base64 encoded: {"streamUrl": "http://example.com/stream", "imageUrl": "http://example.com/img.jpg", "name": "Test Orion"}
	data := "eyJzdHJlYW1VcmwiOiAiaHR0cDovL2V4YW1wbGUuY29tL3N0cmVhbSIsICJpbWFnZVVybCI6ICJodHRwOi8vZXhhbXBsZS5jb20vaW1nLmpwZyIsICJuYW1lIjogIlRlc3QgT3Jpb24ifQ=="

	req, _ := http.NewRequest("POST", ts.URL+"/bmx/orion/v1/playback/station/"+data, nil)
	req.Header.Set("Authorization", "Bearer mock-token")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)

	var resp map[string]interface{}

	_ = json.Unmarshal(body, &resp)

	if resp["name"] != "Test Orion" {
		t.Errorf("Expected name Test Orion, got %v", resp["name"])
	}
}

func TestCustomPlayback(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Base64 encoded: http://example.com/stream
	encodedURL := "aHR0cDovL2V4YW1wbGUuY29tL3N0cmVhbQ=="
	imageUrl := "http://example.com/img.jpg"
	name := "Test Custom"

	res, err := http.Get(ts.URL + "/custom/v1/playback/" + encodedURL + "?imageUrl=" + url.QueryEscape(imageUrl) + "&name=" + url.QueryEscape(name))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)

	var resp map[string]interface{}
	_ = json.Unmarshal(body, &resp)

	if resp["name"] != "Test Custom" {
		t.Errorf("Expected name Test Custom, got %v", resp["name"])
	}

	audio := resp["audio"].(map[string]interface{})
	if audio["streamUrl"] != "http://example.com/stream" {
		t.Errorf("Expected streamUrl http://example.com/stream, got %v", audio["streamUrl"])
	}

	if resp["imageUrl"] != imageUrl {
		t.Errorf("Expected imageUrl %s, got %v", imageUrl, resp["imageUrl"])
	}
}

func TestBMXUnauthorized(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	paths := []struct {
		method string
		path   string
	}{
		{"GET", "/bmx/tunein/v1/playback/station/s123"},
		{"GET", "/bmx/tunein/v1/playback/episodes/p123"},
		{"GET", "/bmx/tunein/v1/playback/episode/p123"},
		{"POST", "/bmx/orion/v1/playback/station/data"},
	}

	for _, tc := range paths {
		req, _ := http.NewRequest(tc.method, ts.URL+tc.path, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("%s %s: %v", tc.method, tc.path, err)
			continue
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: Expected status 401, got %v", tc.method, tc.path, res.Status)
		}

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "401 Unauthorized") || !strings.Contains(bodyStr, "No access token found.") {
			t.Errorf("%s %s: Unexpected response body: %s", tc.method, tc.path, bodyStr)
		}
	}
}

func TestHandleTuneInToken(t *testing.T) {
	r, s := setupRouter("http://localhost:8001", nil)
	s.SetMirrorSettings(false, nil, nil, "")

	ts := httptest.NewServer(r)
	defer ts.Close()

	payload := `{"grant_type":"refresh_token","refresh_token":"test-refresh-token"}`
	res, err := http.Post(ts.URL+"/bmx/tunein/v1/token", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %v", res.Status)
	}

	var resp map[string]string
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp["access_token"] != "test-refresh-token" {
		t.Errorf("Expected access_token 'test-refresh-token', got %v", resp["access_token"])
	}
	if resp["refresh_token"] != "test-refresh-token" {
		t.Errorf("Expected refresh_token 'test-refresh-token', got %v", resp["refresh_token"])
	}
}

func TestHandleTuneInPlayback_Authorized(t *testing.T) {
	r, s := setupRouter("http://localhost:8001", nil)
	s.SetMirrorSettings(false, nil, nil, "")

	ts := httptest.NewServer(r)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/bmx/tunein/v1/playback/station/s166521", nil)
	req.Header.Set("Authorization", "Bearer mock-token")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %v", res.Status)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp["name"] == "" {
		t.Errorf("Expected station name, got empty")
	}
	if audio, ok := resp["audio"].(map[string]interface{}); !ok || audio["streamUrl"] == "" {
		t.Errorf("Expected audio streamUrl, got %v", resp["audio"])
	}
}
