package bmx

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestPlayCustomStream(t *testing.T) {
	// Simple test for custom stream XML generation
	dataObj := struct {
		StreamURL string `json:"streamUrl"`
		ImageURL  string `json:"imageUrl"`
		Name      string `json:"name"`
	}{
		StreamURL: "http://example.com/stream.mp3",
		ImageURL:  "image.png",
		Name:      "Stream Name",
	}
	jsonBytes, _ := json.Marshal(dataObj)

	// Test Standard Base64
	dataStd := base64.StdEncoding.EncodeToString(jsonBytes)
	resp, err := PlayCustomStream(dataStd)
	if err != nil {
		t.Fatalf("PlayCustomStream with standard base64 failed: %v", err)
	}
	if resp.Name != "Stream Name" {
		t.Errorf("Expected name Stream Name, got %s", resp.Name)
	}

	// Test URL-safe Base64
	dataURL := base64.URLEncoding.EncodeToString(jsonBytes)
	resp, err = PlayCustomStream(dataURL)
	if err != nil {
		t.Fatalf("PlayCustomStream with URL-safe base64 failed: %v", err)
	}
	if resp.Name != "Stream Name" {
		t.Errorf("Expected name Stream Name, got %s", resp.Name)
	}
}

func TestTuneInPodcastInfo_Base64(t *testing.T) {
	name := "Podcast Name / with special chars?"

	// Test Standard Base64
	encodedStd := base64.StdEncoding.EncodeToString([]byte(name))
	resp, err := TuneInPodcastInfo("123", encodedStd)
	if err != nil {
		t.Fatalf("TuneInPodcastInfo with standard base64 failed: %v", err)
	}
	if resp.Name != name {
		t.Errorf("Expected name %s, got %s", name, resp.Name)
	}

	// Test URL-safe Base64
	encodedURL := base64.URLEncoding.EncodeToString([]byte(name))
	resp, err = TuneInPodcastInfo("123", encodedURL)
	if err != nil {
		t.Fatalf("TuneInPodcastInfo with URL-safe base64 failed: %v", err)
	}
	if resp.Name != name {
		t.Errorf("Expected name %s, got %s", name, resp.Name)
	}
}
