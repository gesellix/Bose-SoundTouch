package bmx

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestBuildOrionLocation_IsRelative(t *testing.T) {
	// Relative, so the speaker resolves it against the Orion baseUrl from the
	// BMX registry and a stored preset follows AfterTouch to a new address
	// (issue 769).
	loc := BuildOrionLocation("Doc Radio", "http://192.0.2.10/art.png", "http://192.0.2.10:8000/stream?a=1&b=2")

	data, ok := strings.CutPrefix(loc, "/station?data=")
	if !ok {
		t.Fatalf("location = %q, want relative /station?data=...", loc)
	}

	if strings.ContainsAny(data, "+/=&") {
		t.Fatalf("data parameter %q is not query-escaped", data)
	}

	unescaped, err := url.QueryUnescape(data)
	if err != nil {
		t.Fatalf("unescape: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(unescaped)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var payload struct {
		Name      string `json:"name"`
		ImageURL  string `json:"imageUrl"`
		StreamURL string `json:"streamUrl"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.Name != "Doc Radio" || payload.ImageURL != "http://192.0.2.10/art.png" ||
		payload.StreamURL != "http://192.0.2.10:8000/stream?a=1&b=2" {
		t.Fatalf("payload = %+v", payload)
	}

	// The station endpoint must decode what the builder encodes.
	resp, err := PlayCustomStream(unescaped)
	if err != nil {
		t.Fatalf("PlayCustomStream: %v", err)
	}

	if resp.Audio.StreamUrl != payload.StreamURL || resp.Name != "Doc Radio" {
		t.Fatalf("round trip = %+v", resp)
	}
}

func TestPlayCustomStream(t *testing.T) {
	// Test Standard Base64
	dataStd := "eyJzdHJlYW1VcmwiOiJodHRwOi8vZXhhbXBsZS5jb20vc3RyZWFtLm1wMyIsImltYWdlVXJsIjoiaW1hZ2UucG5nIiwibmFtZSI6IlN0cmVhbSBOYW1lIn0="

	resp, err := PlayCustomStream(dataStd)
	if err != nil {
		t.Fatalf("PlayCustomStream with standard base64 failed: %v", err)
	}

	if resp.Name != "Stream Name" {
		t.Errorf("Expected name Stream Name, got %s", resp.Name)
	}

	// Test URL-safe Base64
	dataURL := "eyJzdHJlYW1VcmwiOiJodHRwOi8vZXhhbXBsZS5jb20vc3RyZWFtLm1wMyIsImltYWdlVXJsIjoiaW1hZ2UucG5nIiwibmFtZSI6IlN0cmVhbSBOYW1lIn0="

	resp, err = PlayCustomStream(dataURL)
	if err != nil {
		t.Fatalf("PlayCustomStream with URL-safe base64 failed: %v", err)
	}

	if resp.Name != "Stream Name" {
		t.Errorf("Expected name Stream Name, got %s", resp.Name)
	}
}

func TestBuildCustomStreamResponseFromURLs(t *testing.T) {
	// Multiple candidates must all reach the speaker, in order, so it can
	// fail over from a dead variant to a working one (see s56857 / NDR 2).
	urls := []string{
		"https://example.com/aac/low",
		"https://example.com/mp3/128/stream.mp3",
	}

	resp, err := BuildCustomStreamResponseFromURLs(urls, "image.png", "NDR 2")
	if err != nil {
		t.Fatalf("BuildCustomStreamResponseFromURLs failed: %v", err)
	}

	if got := len(resp.Audio.Streams); got != len(urls) {
		t.Fatalf("expected %d streams, got %d", len(urls), got)
	}

	for i, want := range urls {
		if got := resp.Audio.Streams[i].StreamUrl; got != want {
			t.Errorf("stream %d: expected %q, got %q", i, want, got)
		}
	}

	if resp.Audio.StreamUrl != urls[0] {
		t.Errorf("top-level StreamUrl: expected %q, got %q", urls[0], resp.Audio.StreamUrl)
	}

	// Empty input is an error, not a panic.
	if _, err := BuildCustomStreamResponseFromURLs(nil, "", ""); err == nil {
		t.Error("expected error for empty URL list, got nil")
	}

	// The single-URL wrapper still yields exactly one stream.
	single, err := BuildCustomStreamResponse("https://example.com/only", "", "Solo")
	if err != nil {
		t.Fatalf("BuildCustomStreamResponse failed: %v", err)
	}

	if got := len(single.Audio.Streams); got != 1 {
		t.Errorf("expected 1 stream from single-URL builder, got %d", got)
	}
}
