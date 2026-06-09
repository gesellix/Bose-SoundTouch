package dlna_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/dlna"
	"github.com/gesellix/bose-soundtouch/pkg/dlna/dlnatest"
)

// ----------------------------------------------------------------------------
// Integration tests using the in-process DLNA test fixture
// ----------------------------------------------------------------------------

// TestBrowse_Root checks that Browse("0") returns the Music container from the
// default test tree.
func TestBrowse_Root(t *testing.T) {
	ts, _ := dlnatest.NewHTTPTest()
	defer ts.Close()

	srv := discovery.MediaServer{
		FriendlyName:  "Test Server",
		CDSControlURL: ts.URL + "/ctl/ContentDir",
	}

	ctx := context.Background()

	result, err := dlna.Browse(ctx, srv, "0", 0, 50)
	if err != nil {
		t.Fatalf("Browse root: %v", err)
	}

	if len(result.Containers) == 0 {
		t.Fatal("Browse root: got 0 containers, want at least 1")
	}

	var musicContainer *dlna.Container

	for i := range result.Containers {
		if result.Containers[i].Title == "Music" {
			musicContainer = &result.Containers[i]
			break
		}
	}

	if musicContainer == nil {
		t.Fatalf("Browse root: Music container not found; got %v", result.Containers)
	}

	if musicContainer.ID == "" {
		t.Error("Music container has empty ID")
	}

	t.Logf("Music container: id=%q parentID=%q childCount=%d",
		musicContainer.ID, musicContainer.ParentID, musicContainer.ChildCount)
}

// TestBrowse_MusicFolder checks that browsing into the Music container returns
// exactly 2 audio items with non-empty StreamURLs that are fetchable.
func TestBrowse_MusicFolder(t *testing.T) {
	ts, _ := dlnatest.NewHTTPTest()
	defer ts.Close()

	srv := discovery.MediaServer{
		FriendlyName:  "Test Server",
		CDSControlURL: ts.URL + "/ctl/ContentDir",
	}

	ctx := context.Background()

	// First, browse root to find the Music folder ID.
	root, err := dlna.Browse(ctx, srv, "0", 0, 50)
	if err != nil {
		t.Fatalf("Browse root: %v", err)
	}

	var musicID string

	for _, c := range root.Containers {
		if c.Title == "Music" {
			musicID = c.ID
			break
		}
	}

	if musicID == "" {
		t.Fatal("Music container not found in root browse")
	}

	// Now browse the Music folder.
	result, err := dlna.Browse(ctx, srv, musicID, 0, 50)
	if err != nil {
		t.Fatalf("Browse music folder: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 audio items, got %d", len(result.Items))
	}

	for _, item := range result.Items {
		t.Run(item.Title, func(t *testing.T) {
			if item.Title == "" {
				t.Error("item has empty Title")
			}

			if !item.IsAudioItem() {
				t.Errorf("IsAudioItem() = false for item %q (MimeType=%q Class=%q)",
					item.Title, item.MimeType, item.Class)
			}

			if item.Artist == "" {
				t.Errorf("item %q has empty Artist", item.Title)
			} else if item.Artist != "Test Artist" {
				t.Errorf("item %q: Artist = %q, want %q", item.Title, item.Artist, "Test Artist")
			}

			if item.Album == "" {
				t.Errorf("item %q has empty Album", item.Title)
			} else if item.Album != "Test Album" {
				t.Errorf("item %q: Album = %q, want %q", item.Title, item.Album, "Test Album")
			}

			if item.StreamURL == "" {
				t.Fatalf("item %q has empty StreamURL", item.Title)
			}

			// Fetch the stream URL and verify it returns audio bytes.
			resp, err := http.Get(item.StreamURL) //nolint:noctx
			if err != nil {
				t.Fatalf("GET %s: %v", item.StreamURL, err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d", item.StreamURL, resp.StatusCode)
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read media body: %v", err)
			}

			if len(data) < 44 {
				t.Errorf("audio payload too small (%d bytes), expected at least a WAV header", len(data))
			}

			// Verify RIFF/WAVE header (silentWAV always produces PCM WAV).
			if string(data[0:4]) != "RIFF" {
				t.Errorf("expected RIFF header, got %q", data[0:4])
			}

			if string(data[8:12]) != "WAVE" {
				t.Errorf("expected WAVE marker, got %q", data[8:12])
			}
		})
	}
}

// TestBrowse_NoCDSControlURL verifies that Browse returns an error when the
// server has no CDSControlURL set.
func TestBrowse_NoCDSControlURL(t *testing.T) {
	srv := discovery.MediaServer{FriendlyName: "Empty"}
	_, err := dlna.Browse(context.Background(), srv, "0", 0, 50)

	if err == nil {
		t.Error("Browse with empty CDSControlURL: expected error, got nil")
	}
}

// ----------------------------------------------------------------------------
// Pure function unit tests
// ----------------------------------------------------------------------------

// TestMimeFromProtocolInfo checks the DLNA protocolInfo MIME extraction.
func TestMimeFromProtocolInfo(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"http-get:*:audio/x-wav:*", "audio/x-wav"},
		{"http-get:*:audio/mpeg:*", "audio/mpeg"},
		{"http-get:*:audio/ogg:DLNA.ORG_PN=OGG", "audio/ogg"},
		{"http-get:*:image/jpeg:*", "image/jpeg"},
		// Fewer than 3 colons: return empty string.
		{"http-get", ""},
		{"http-get:*", ""},
		{"", ""},
	}

	for _, tc := range cases {
		got := dlna.MimeFromProtocolInfo(tc.input)
		if got != tc.want {
			t.Errorf("MimeFromProtocolInfo(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestParseHMS checks duration string parsing to seconds.
func TestParseHMS(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"0:00:01.000", 1},
		{"0:00:01", 1},
		{"0:03:42", 222},
		{"0:03:42.000", 222},
		{"1:00:00", 3600},
		{"1:30:00", 5400},
		{"0:00:00", 0},
		{"", 0},
		// Malformed: return 0.
		{"99:99", 0},
	}

	for _, tc := range cases {
		got := dlna.ParseHMS(tc.input)
		if got != tc.want {
			t.Errorf("ParseHMS(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// TestIsAudioItem checks the audio-item classifier.
func TestIsAudioItem(t *testing.T) {
	cases := []struct {
		item dlna.Item
		want bool
	}{
		// MimeType prefix "audio/" is sufficient.
		{dlna.Item{MimeType: "audio/x-wav"}, true},
		{dlna.Item{MimeType: "audio/mpeg"}, true},
		// Class "audioitem" (any case).
		{dlna.Item{Class: "object.item.audioItem.musicTrack"}, true},
		{dlna.Item{Class: "object.item.musicTrack"}, true},
		// Video and image MIME types: not audio.
		{dlna.Item{MimeType: "video/mp4"}, false},
		{dlna.Item{MimeType: "image/jpeg"}, false},
		// Empty item.
		{dlna.Item{}, false},
	}

	for _, tc := range cases {
		got := tc.item.IsAudioItem()
		if got != tc.want {
			t.Errorf("Item{MimeType:%q Class:%q}.IsAudioItem() = %v, want %v",
				tc.item.MimeType, tc.item.Class, got, tc.want)
		}
	}
}
