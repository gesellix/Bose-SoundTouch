package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchTuneInMetadata(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `
<!doctype html>
<html>
<head>
    <meta property="og:title" content="WDR 2 Rheinland, 100.4 FM, Köln | Free Internet Radio | TuneIn"/>
    <meta property="og:image" content="https://cdn-radiotime-logos.tunein.com/s213886g.png"/>
</head>
<body></body>
</html>
`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer ts.Close()

	// Temporarily override httpClient to use test server
	oldClient := httpClient
	httpClient = ts.Client()

	defer func() { httpClient = oldClient }()

	metadata, err := fetchTuneInMetadata(ts.URL + "/radio/WDR-2-Rheinland-1004-s213886/")
	if err != nil {
		t.Fatalf("fetchTuneInMetadata() error = %v", err)
	}

	if metadata == nil {
		t.Fatal("fetchTuneInMetadata() returned nil metadata")
	} else {
		expectedName := "WDR 2 Rheinland"
		if metadata.Name != expectedName {
			t.Errorf("metadata.Name = %v, want %v", metadata.Name, expectedName)
		}

		expectedArtwork := "https://cdn-radiotime-logos.tunein.com/s213886g.png"
		if metadata.Artwork != expectedArtwork {
			t.Errorf("metadata.Artwork = %v, want %v", metadata.Artwork, expectedArtwork)
		}
	}
}

func TestResolveLocation(t *testing.T) {
	tests := []struct {
		name             string
		source           string
		location         string
		expectedSource   string
		expectedLocation string
	}{
		{
			name:             "Plain location",
			source:           "TUNEIN",
			location:         "/v1/playback/station/s213886",
			expectedSource:   "TUNEIN",
			expectedLocation: "/v1/playback/station/s213886",
		},
		{
			name:             "TuneIn URL",
			source:           "",
			location:         "https://tunein.com/radio/WDR-2-Rheinland-1004-s213886/",
			expectedSource:   "TUNEIN",
			expectedLocation: "/v1/playback/station/s213886",
		},
		{
			name:             "TuneIn URL with source",
			source:           "SOMETHING",
			location:         "https://tunein.com/radio/WDR-2-Rheinland-1004-s213886/",
			expectedSource:   "TUNEIN",
			expectedLocation: "/v1/playback/station/s213886",
		},
		{
			name:             "TuneIn URL without trailing slash",
			source:           "",
			location:         "https://tunein.com/radio/WDR-2-Rheinland-1004-s213886",
			expectedSource:   "TUNEIN",
			expectedLocation: "/v1/playback/station/s213886",
		},
		{
			name:             "Non-TuneIn URL",
			source:           "OTHER",
			location:         "https://example.com/radio/s123",
			expectedSource:   "OTHER",
			expectedLocation: "https://example.com/radio/s123",
		},
		{
			name:             "TuneIn URL short form",
			source:           "",
			location:         "https://tunein.com/radio/s213886/",
			expectedSource:   "TUNEIN",
			expectedLocation: "/v1/playback/station/s213886",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSource, gotLocation := resolveLocation(tt.source, tt.location)
			if gotSource != tt.expectedSource {
				t.Errorf("resolveLocation() gotSource = %v, want %v", gotSource, tt.expectedSource)
			}

			if gotLocation != tt.expectedLocation {
				t.Errorf("resolveLocation() gotLocation = %v, want %v", gotLocation, tt.expectedLocation)
			}
		})
	}
}

func TestResolveLocationSpotify(t *testing.T) {
	tests := []struct {
		name             string
		source           string
		location         string
		expectedSource   string
		expectedLocation string
	}{
		{
			name:             "Spotify album URL",
			source:           "",
			location:         "https://open.spotify.com/album/6rT8yer84xoh0t17poLsmn?si=XqxdZazpTLC1ceoC8EeCuA",
			expectedSource:   "SPOTIFY",
			expectedLocation: "/playback/container/c3BvdGlmeTphbGJ1bTo2clQ4eWVyODR4b2gwdDE3cG9Mc21u",
		},
		{
			name:             "Spotify playlist URL",
			source:           "",
			location:         "https://open.spotify.com/playlist/37i9dQZF1DX0XUsuxWHRQd",
			expectedSource:   "SPOTIFY",
			expectedLocation: "/playback/container/c3BvdGlmeTpwbGF5bGlzdDozN2k5ZFFaRjFEWDBYVXN1eFdIUlFk",
		},
		{
			name:             "Spotify track URL",
			source:           "",
			location:         "https://open.spotify.com/track/17GmwQ9Q3MTAz05OokmNNB?si=123",
			expectedSource:   "SPOTIFY",
			expectedLocation: "/playback/container/c3BvdGlmeTp0cmFjazoxN0dtd1E5UTNNVEF6MDVPb2ttTk5C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSource, gotLocation := resolveLocation(tt.source, tt.location)
			if gotSource != tt.expectedSource {
				t.Errorf("resolveLocation() gotSource = %v, want %v", gotSource, tt.expectedSource)
			}

			if gotLocation != tt.expectedLocation {
				t.Errorf("resolveLocation() gotLocation = %v, want %v", gotLocation, tt.expectedLocation)
			}
		})
	}
}

func TestFetchSpotifyMetadata(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `
<!doctype html>
<html>
<head>
    <meta property="og:title" content="Terminal Caribe - Album by Santi &amp; Tuğçe | Spotify"/>
    <meta property="og:image" content="https://i.scdn.co/image/ab67616d0000b273f0e55478f4a15182405bcb47"/>
</head>
<body></body>
</html>
`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer ts.Close()

	// Temporarily override httpClient to use test server
	oldClient := httpClient
	httpClient = ts.Client()

	defer func() { httpClient = oldClient }()

	metadata, err := fetchSpotifyMetadata(ts.URL + "/album/7F50uh7oGitmAEScRKV6pD")
	if err != nil {
		t.Fatalf("fetchSpotifyMetadata() error = %v", err)
	}

	if metadata == nil {
		t.Fatal("fetchSpotifyMetadata() returned nil metadata")
	} else {
		expectedName := "Terminal Caribe - Album by Santi & Tuğçe"
		if metadata.Name != expectedName {
			t.Errorf("metadata.Name = %v, want %v", metadata.Name, expectedName)
		}

		expectedArtwork := "https://i.scdn.co/image/ab67616d0000b273f0e55478f4a15182405bcb47"
		if metadata.Artwork != expectedArtwork {
			t.Errorf("metadata.Artwork = %v, want %v", metadata.Artwork, expectedArtwork)
		}
	}
}
