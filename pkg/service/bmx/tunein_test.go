package bmx

import (
	"encoding/base64"
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
	decoded, err := decodeBase64URI(encoded)
	if err != nil {
		t.Fatalf("failed to decode navigate href: %v", err)
	}

	got := decoded
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
	decoded, err := decodeBase64URI(encoded)
	if err != nil {
		t.Fatalf("failed to decode navigate href: %v", err)
	}

	got := decoded
	if strings.Count(got, "render=json") != 1 {
		t.Errorf("navigate href %q should contain render=json exactly once", got)
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

func TestParseTuneInStreamBody(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantURLs  []string
		wantError bool
	}{
		{
			name:     "single URL",
			body:     "https://stream.example.com/foo.mp3\n",
			wantURLs: []string{"https://stream.example.com/foo.mp3"},
		},
		{
			name:     "multiple URLs",
			body:     "https://a/1.mp3\nhttps://b/2.mp3\n",
			wantURLs: []string{"https://a/1.mp3", "https://b/2.mp3"},
		},
		{
			name:      "comment-only body — TuneIn 400 error",
			body:      "#STATUS: 400\n#description=Bad request\n",
			wantError: true,
		},
		{
			name:     "comments mixed with real URL",
			body:     "#EXTM3U\nhttps://stream.example.com/foo.mp3\n#END\n",
			wantURLs: []string{"https://stream.example.com/foo.mp3"},
		},
		{
			name:      "empty body",
			body:      "",
			wantError: true,
		},
		{
			name:      "only blank lines",
			body:      "\n\n  \n",
			wantError: true,
		},
		{
			name:     "trims surrounding whitespace per line",
			body:     "  https://a/1.mp3  \n\thttps://b/2.mp3\t\n",
			wantURLs: []string{"https://a/1.mp3", "https://b/2.mp3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTuneInStreamBody([]byte(tc.body), "test-guide-id")

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}

				if !strings.Contains(err.Error(), "test-guide-id") {
					t.Errorf("error should mention the guide-id for diagnosis: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tc.wantURLs) {
				t.Fatalf("len mismatch: got %d (%v), want %d (%v)", len(got), got, len(tc.wantURLs), tc.wantURLs)
			}

			for i := range got {
				if got[i] != tc.wantURLs[i] {
					t.Errorf("URL[%d] mismatch: got %q, want %q", i, got[i], tc.wantURLs[i])
				}
			}
		})
	}
}

func TestTuneInSearchProfileEmitsBmxPlayback(t *testing.T) {
	cases := []struct {
		name         string
		profileName  string
		guideID      string
		wantPlayback bool
		wantType     string
	}{
		{name: "Program with guide-id gets play link", profileName: "Program", guideID: "p290778", wantPlayback: true, wantType: "tracklisturl"},
		{name: "Artist with guide-id is navigate-only", profileName: "Artist", guideID: "a12345", wantPlayback: false},
		{name: "Program without guide-id is navigate-only", profileName: "Program", guideID: "", wantPlayback: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]interface{}{
				"GuideId":  tc.guideID,
				"Title":    "Die Nachrichten",
				"Image":    "http://example.com/logo.png",
				"Subtitle": "Deutschlandfunk",
				"Type":     tc.profileName,
				"Actions": map[string]interface{}{
					"Profile": map[string]interface{}{
						"Url": "https://api.radiotime.com/profiles/" + tc.guideID,
					},
				},
			}

			navItem := tuneInSearchProfile(item, tc.profileName)

			if navItem.Links == nil {
				t.Fatal("expected Links to be set")
			}

			if tc.wantPlayback {
				if navItem.Links.BmxPlayback == nil {
					t.Fatal("expected BmxPlayback link for Program")
				}

				if navItem.Links.BmxPlayback.Type != tc.wantType {
					t.Errorf("BmxPlayback.Type = %q, want %q", navItem.Links.BmxPlayback.Type, tc.wantType)
				}

				if !strings.Contains(navItem.Links.BmxPlayback.Href, tc.guideID) {
					t.Errorf("BmxPlayback.Href must carry the guide-id %q; got %q", tc.guideID, navItem.Links.BmxPlayback.Href)
				}

				if !strings.Contains(navItem.Links.BmxPlayback.Href, "encoded_name=") {
					t.Errorf("BmxPlayback.Href should carry encoded_name; got %q", navItem.Links.BmxPlayback.Href)
				}
			} else if navItem.Links.BmxPlayback != nil {
				t.Errorf("did not expect BmxPlayback link; got %+v", navItem.Links.BmxPlayback)
			}

			if navItem.Links.BmxNavigate == nil {
				t.Error("expected BmxNavigate link to remain available")
			}
		})
	}
}

func TestParseTuneInProgramContents(t *testing.T) {
	const happyPath = `{
		"Items": [
			{
				"ContainerType": "Topics",
				"Title": "Episodes",
				"Children": [
					{ "GuideId": "t554138374", "Type": "Topic", "Title": "newest" },
					{ "GuideId": "t554134863", "Type": "Topic", "Title": "previous" }
				]
			}
		]
	}`

	const localisedTitle = `{
		"Items": [
			{
				"ContainerType": "Topics",
				"Title": "Folgen",
				"Children": [
					{ "GuideId": "t111", "Type": "Topic", "Title": "newest" }
				]
			}
		]
	}`

	const episodesAfterRelated = `{
		"Items": [
			{
				"ContainerType": "Topics",
				"Title": "Related Shows",
				"Children": [
					{ "GuideId": "t999", "Type": "Topic", "Title": "wrong" }
				]
			},
			{
				"ContainerType": "Topics",
				"Title": "Episodes",
				"Children": [
					{ "GuideId": "t222", "Type": "Topic", "Title": "right" }
				]
			}
		]
	}`

	const skipsNonTopic = `{
		"Items": [
			{
				"ContainerType": "Topics",
				"Title": "Episodes",
				"Children": [
					{ "GuideId": "p333", "Type": "Container", "Title": "nested program" },
					{ "GuideId": "t444", "Type": "Topic", "Title": "real episode" }
				]
			}
		]
	}`

	cases := []struct {
		name      string
		body      string
		wantID    string
		wantError bool
	}{
		{name: "happy path — first child wins", body: happyPath, wantID: "t554138374"},
		{name: "localised title — falls back to first Topics container", body: localisedTitle, wantID: "t111"},
		{name: "Episodes container preferred over Related", body: episodesAfterRelated, wantID: "t222"},
		{name: "skips non-Topic children", body: skipsNonTopic, wantID: "t444"},
		{name: "empty body — error", body: `{}`, wantError: true},
		{name: "no Topics containers — error", body: `{"Items":[{"ContainerType":"Banner","Children":[]}]}`, wantError: true},
		{name: "Topics with no t-prefixed children — error",
			body:      `{"Items":[{"ContainerType":"Topics","Title":"Episodes","Children":[{"GuideId":"p1"}]}]}`,
			wantError: true},
		{name: "malformed JSON — error", body: `{not json`, wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTuneInProgramContents([]byte(tc.body), "p290778")

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got id=%q", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.wantID {
				t.Errorf("got episode id %q, want %q", got, tc.wantID)
			}
		})
	}
}
