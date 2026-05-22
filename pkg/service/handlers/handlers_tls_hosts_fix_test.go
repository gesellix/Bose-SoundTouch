package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func stubInfoForFix(t *testing.T, margeURL string) string {
	t.Helper()

	body := `<?xml version="1.0" encoding="UTF-8" ?><info deviceID="DEVICEID01"><name>Test</name><margeAccountUUID>1000001</margeAccountUUID><margeURL>` + margeURL + `</margeURL></info>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/info"
}

func TestFetchMargeHostFromSpeaker_ReturnsHostOnly(t *testing.T) {
	cases := []struct {
		name     string
		margeURL string
		want     string
	}{
		{
			name:     "HTTPS with port",
			margeURL: "https://aftertouch.example:8443/",
			want:     "aftertouch.example",
		},
		{
			name:     "HTTP IP with port",
			margeURL: "http://192.0.2.10:8000/",
			want:     "192.0.2.10",
		},
		{
			name:     "Bare host fallback",
			margeURL: "aftertouch.example",
			want:     "aftertouch.example",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probeURL := stubInfoForFix(t, tc.margeURL)

			got, err := fetchMargeHostFromSpeaker(probeURL, 2*time.Second)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchMargeHostFromSpeaker_EmptyMargeURLReturnsEmpty(t *testing.T) {
	probeURL := stubInfoForFix(t, "")

	got, err := fetchMargeHostFromSpeaker(probeURL, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "" {
		t.Errorf("expected empty host for empty margeURL, got %q", got)
	}
}

func TestFetchMargeHostFromSpeaker_UnreachableReturnsError(t *testing.T) {
	// Point at a closed port; the probe should fail without panicking.
	_, err := fetchMargeHostFromSpeaker("http://127.0.0.1:1/info", 200*time.Millisecond)
	if err == nil {
		t.Errorf("expected error for unreachable speaker, got nil")
	}

	if !strings.Contains(err.Error(), "probe failed") {
		t.Errorf("expected 'probe failed' in error, got %q", err.Error())
	}
}
