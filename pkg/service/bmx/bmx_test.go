package bmx

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestTuneInRenderJSONURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty URL returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "URL with no query params gets render=json added",
			input: "http://opml.radiotime.com/Browse.ashx",
			want:  "http://opml.radiotime.com/Browse.ashx?render=json",
		},
		{
			name:  "URL with other params gets render=json appended",
			input: "http://opml.radiotime.com/Browse.ashx?c=news",
			want:  "http://opml.radiotime.com/Browse.ashx?c=news&render=json",
		},
		{
			name:  "URL already containing render=json is not duplicated",
			input: "http://opml.radiotime.com/?render=json",
			want:  "http://opml.radiotime.com/?render=json",
		},
		{
			name:  "URL with render=xml gets render replaced with json",
			input: "http://opml.radiotime.com/Browse.ashx?c=podcast&render=xml",
			want:  "http://opml.radiotime.com/Browse.ashx?c=podcast&render=json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tuneInRenderJSONURI(tt.input)
			if got != tt.want {
				t.Errorf("tuneInRenderJSONURI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTuneInOpmlURI(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"http://opml.radiotime.com/Browse.ashx", true},
		{"https://opml.radiotime.com/Browse.ashx", true},
		{"http://opml.radiotime.com/?render=json", true},
		{"http://api.radiotime.com/profiles?fulltextsearch=true", false},
		{"http://example.com", false},
		{"not-a-url", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isTuneInOpmlURI(tt.input)
			if got != tt.want {
				t.Errorf("isTuneInOpmlURI(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTuneInSearchURI(t *testing.T) {
	tests := []struct {
		name  string
		query string
		check func(string) bool
	}{
		{
			name:  "spaces are percent-encoded",
			query: "radio paradise",
			check: func(u string) bool { return !strings.Contains(u, " ") && strings.Contains(u, "radio+paradise") },
		},
		{
			name:  "ampersand is encoded",
			query: "news & talk",
			check: func(u string) bool { return !strings.Contains(u, " ") && strings.Contains(u, "%26") },
		},
		{
			name:  "plain query is appended to base URL",
			query: "jazz",
			check: func(u string) bool { return u == TuneInSearchAPI+"jazz" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tuneInSearchURI(tt.query)
			if !tt.check(got) {
				t.Errorf("tuneInSearchURI(%q) = %q: check failed", tt.query, got)
			}
		})
	}
}

func TestTuneInNavigateLinkEncodesRenderJSON(t *testing.T) {
	item := map[string]interface{}{
		"URL":     "http://opml.radiotime.com/Browse.ashx?c=news",
		"text":    "News",
		"subtext": "Latest",
		"image":   "http://example.com/news.png",
	}

	result := tuneInNavigateLink(item)

	href := result.Links.BmxNavigate.Href
	encoded := strings.TrimPrefix(href, "/v1/navigate/")
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode navigate href: %v", err)
	}

	got := string(decoded)
	if !strings.Contains(got, "render=json") {
		t.Errorf("navigate href %q missing render=json", got)
	}
	if strings.Count(got, "render=json") > 1 {
		t.Errorf("navigate href %q has duplicate render=json", got)
	}
}

func TestTuneInNavigateLinkNoDuplicateRenderJSON(t *testing.T) {
	item := map[string]interface{}{
		"URL": "http://opml.radiotime.com/Browse.ashx?c=podcast&render=json",
	}

	result := tuneInNavigateLink(item)

	href := result.Links.BmxNavigate.Href
	encoded := strings.TrimPrefix(href, "/v1/navigate/")
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode navigate href: %v", err)
	}

	got := string(decoded)
	if strings.Count(got, "render=json") != 1 {
		t.Errorf("navigate href %q should contain render=json exactly once", got)
	}
}

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

	jsonBytes, err := json.Marshal(dataObj)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

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

// TestTuneInStream_EmptyFormatsUsesDefault pins the post-#292 contract:
// AfterTouch must NOT request HLS streams from TuneIn unless the
// operator has explicitly opted in. The default request shape is
// "mp3,aac,ogg" — matches pre-2026-05-10 behaviour and works on
// every SoundTouch model verified. PR #249 had added "hls"
// unconditionally; that regressed playback on ST10/firmware 27 (the
// speaker can't parse the .m3u8 playlist TuneIn returns when HLS is
// in the format list).
func TestTuneInStream_EmptyFormatsUsesDefault(t *testing.T) {
	got := TuneInStream("s33828", "")

	if strings.Contains(got, "hls") {
		t.Errorf("default TuneInStream URL must NOT request HLS; got %s", got)
	}

	want := "formats=" + DefaultTuneInStreamFormats
	if !strings.Contains(got, want) {
		t.Errorf("default TuneInStream URL must request %q; got %s", want, got)
	}

	if !strings.Contains(got, "id=s33828") {
		t.Errorf("TuneInStream URL must carry the station ID; got %s", got)
	}
}

// TestTuneInStream_OverrideHonoured verifies the opt-in path: when an
// operator sets Settings.TuneInStreamFormats to a custom list,
// TuneInStream passes it through verbatim. Two sub-cases catch the
// common opt-in (re-add hls) and a more drastic override (single
// format) so a future regression in the trim/fallback logic surfaces
// at compile/test time.
func TestTuneInStream_OverrideHonoured(t *testing.T) {
	cases := []struct {
		formats string
		want    string
	}{
		{"mp3,aac,ogg,hls", "formats=mp3,aac,ogg,hls"}, // opt-in: re-add HLS
		{"aac", "formats=aac"},                         // single format
		{"  mp3 ", "formats=mp3"},                      // whitespace stripped
	}

	for _, tc := range cases {
		got := TuneInStream("s33828", tc.formats)
		if !strings.Contains(got, tc.want) {
			t.Errorf("TuneInStream(%q) URL must contain %q; got %s", tc.formats, tc.want, got)
		}
	}
}
