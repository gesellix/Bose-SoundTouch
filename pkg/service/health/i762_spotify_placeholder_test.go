package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// speakerSourcesStub serves a raw /sources body.
func speakerSourcesStub(t *testing.T, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sources" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/sources"
}

// TestSourcesDiff_SpotifyPlaceholdersAreNotReported covers issue 762. Every
// SoundTouch lists two SPOTIFY items of its own, with the placeholder
// accounts SpotifyConnectUserName and SpotifyAlexaUserName, whether or not
// Spotify is used. Without a linked account they made the check report
// SPOTIFY as a type the service doesn't know about. A real Spotify account on
// the speaker is still reported.
func TestSourcesDiff_SpotifyPlaceholdersAreNotReported(t *testing.T) {
	const (
		account = "1000001"
		device  = "DEVICEID01"
	)

	placeholders := `<sourceItem source="SPOTIFY" sourceAccount="SpotifyConnectUserName" status="UNAVAILABLE">SpotifyConnectUserName</sourceItem>` +
		`<sourceItem source="SPOTIFY" sourceAccount="SpotifyAlexaUserName" status="UNAVAILABLE">SpotifyAlexaUserName</sourceItem>`

	cases := []struct {
		name       string
		items      string
		wantReport bool
	}{
		{"placeholders only", placeholders, false},
		{"real account next to the placeholders", `<sourceItem source="SPOTIFY" sourceAccount="spotify-user" status="READY">spotify-user</sourceItem>` + placeholders, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := newSourcesDiffDS(t, account, device)
			setServiceSources(t, ds, account, device, "TUNEIN")

			body := `<?xml version="1.0" encoding="UTF-8"?><sources deviceID="` + device + `">` +
				`<sourceItem source="TUNEIN" status="READY"/>` + tc.items + `</sources>`

			got := diffSourcesForDeviceWithURL(ds, account, device, "192.0.2.10", speakerSourcesStub(t, body))

			var reported bool

			for _, f := range got {
				if strings.Contains(f.Message, "doesn't know about") && strings.Contains(f.Message, "SPOTIFY") {
					reported = true
				}
			}

			if reported != tc.wantReport {
				t.Errorf("SPOTIFY reported = %v, want %v; findings: %+v", reported, tc.wantReport, got)
			}
		})
	}
}
