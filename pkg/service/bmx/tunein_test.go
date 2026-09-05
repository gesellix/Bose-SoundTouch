package bmx

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestTuneInSectionsAshx_UntypedContainerSurfacesStations is a regression
// test for a real-world bug: TuneIn's Browse.ashx?render=json responses
// often wrap the actual stations for a category in a container object that
// has "children" but no "type" field at all (unlike navigable sub-categories,
// which are always type:"link"). The original parser's switch only ever
// extracted "children" when itemType == "link", so these untyped containers
// -- and every station nested inside them -- were silently dropped: browse
// showed only category links, never any actual stations. Reproduces the
// shape of a real captured Jazz-genre browse response.
func TestTuneInSectionsAshx_UntypedContainerSurfacesStations(t *testing.T) {
	const wantStationName = "SmoothJazz.com.pl (Poland)"

	payload := `{
		"head": {"status": "200", "title": "Jazz"},
		"body": [
			{
				"text": "Stations",
				"key": "stations",
				"children": [
					{
						"type": "audio",
						"text": "` + wantStationName + `",
						"URL": "http://opml.radiotime.com/Tune.ashx?id=s106565",
						"guide_id": "s106565",
						"subtext": "Smooth Jazz"
					}
				]
			}
		]
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	parsed, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("could not parse test server URL: %v", err)
	}

	allowedTuneInHosts[parsed.Hostname()] = true
	defer delete(allowedTuneInHosts, parsed.Hostname())

	sections, err := tuneInSectionsAshx(ts.URL, nil)
	if err != nil {
		t.Fatalf("tuneInSectionsAshx returned error: %v", err)
	}

	for _, section := range sections {
		for _, item := range section.Items {
			if item.Name == wantStationName {
				if item.Links == nil || item.Links.BmxPlayback == nil {
					t.Errorf("station %q was surfaced but has no BmxPlayback link: %+v", wantStationName, item)
				}

				return
			}
		}
	}

	t.Fatalf("expected station %q to be surfaced from the untyped container, got sections: %+v", wantStationName, sections)
}

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
			check: func(u string) bool {
				return u == "https://api.radiotime.com/profiles?fulltextsearch=true&version=1.3&query=jazz"
			},
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

// TestTuneInNavigateProfileHandlesContainerShapes is a regression test for
// four bugs found reviewing PR #677's profile-navigate fix:
//   - an empty Container (no children, identified by "ContainerType") was
//     misread as a leaf item and turned into a bogus playback link keyed by
//     the container's own non-playable GuideId (it checked "Type", which
//     only leaf items carry, instead of "ContainerType");
//   - the legacy lowercase "children" key (as opposed to "Children") was no
//     longer read at all, silently hiding any container using it;
//   - the Pivots.More.Url "load more" pagination cursor was dropped
//     entirely, so a container's BmxNext link was never built; and
//   - the response's own self link used "/v1/navigate/profile/" (singular),
//     which none of the route dispatchers that recognize "profiles"
//     (plural) actually match, breaking re-navigation via that link.
func TestTuneInNavigateProfileHandlesContainerShapes(t *testing.T) {
	var contentsURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/profile":
			_, _ = w.Write([]byte(`{
				"Item": {
					"Pivots": {
						"Contents": {"DisplayName": "Broadcasts", "Url": "` + contentsURL + `"}
					}
				}
			}`))
		case "/contents":
			_, _ = w.Write([]byte(`{
				"Items": [
					{
						"Title": "Episodes",
						"GuideId": "v5",
						"ContainerType": "Topics",
						"Children": [
							{"Type": "Topic", "Title": "Ep 1", "GuideId": "t100"}
						],
						"Pivots": {"More": {"Url": "` + contentsURL + `?itemToken=abc"}}
					},
					{
						"Title": "Empty Container",
						"GuideId": "v6",
						"ContainerType": "Topics",
						"Children": []
					},
					{
						"Title": "Legacy Children",
						"GuideId": "v7",
						"ContainerType": "Topics",
						"children": [
							{"Type": "Topic", "Title": "Ep 2", "GuideId": "t200"}
						]
					},
					{
						"Type": "Station",
						"Title": "Flat Leaf",
						"GuideId": "s999"
					}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	contentsURL = ts.URL + "/contents"

	parsed, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("could not parse test server URL: %v", err)
	}

	allowedTuneInHosts[parsed.Hostname()] = true
	defer delete(allowedTuneInHosts, parsed.Hostname())

	encodedURI := base64.RawURLEncoding.EncodeToString([]byte(ts.URL + "/profile"))

	navResp, err := TuneInNavigateProfile(encodedURI)
	if err != nil {
		t.Fatalf("TuneInNavigateProfile returned error: %v", err)
	}

	if navResp.Links == nil || navResp.Links.Self == nil {
		t.Fatalf("response has no self link: %+v", navResp)
	}

	if want := "/v1/navigate/profiles/" + encodedURI; navResp.Links.Self.Href != want {
		t.Errorf("self link = %q, want %q (must match the \"profiles\" prefix the dispatchers recognize)", navResp.Links.Self.Href, want)
	}

	byName := make(map[string]models.BmxNavSection, len(navResp.BmxSections))
	for _, section := range navResp.BmxSections {
		byName[section.Name] = section
	}

	if _, found := byName["Empty Container"]; found {
		t.Errorf("empty container was surfaced as a section, want it skipped: %+v", navResp.BmxSections)
	}

	episodes, ok := byName["Episodes"]
	if !ok {
		t.Fatalf("no \"Episodes\" section found: %+v", navResp.BmxSections)
	}

	if len(episodes.Items) != 1 || episodes.Items[0].Name != "Ep 1" {
		t.Errorf("Episodes items = %+v, want exactly [Ep 1]", episodes.Items)
	}

	if episodes.Links == nil || episodes.Links.BmxNext == nil || !strings.Contains(episodes.Links.BmxNext.Href, "/v1/search/next?cursor=") {
		t.Errorf("Episodes section missing BmxNext pagination link: %+v", episodes.Links)
	}

	legacy, ok := byName["Legacy Children"]
	if !ok {
		t.Fatalf("no \"Legacy Children\" section found (lowercase \"children\" fallback not applied): %+v", navResp.BmxSections)
	}

	if len(legacy.Items) != 1 || legacy.Items[0].Name != "Ep 2" {
		t.Errorf("Legacy Children items = %+v, want exactly [Ep 2]", legacy.Items)
	}

	broadcasts, ok := byName["Broadcasts"]
	if !ok {
		t.Fatalf("no \"Broadcasts\" (flat leaf, pivot display name) section found: %+v", navResp.BmxSections)
	}

	if len(broadcasts.Items) != 1 || broadcasts.Items[0].Name != "Flat Leaf" {
		t.Errorf("Broadcasts items = %+v, want exactly [Flat Leaf]", broadcasts.Items)
	}
}

// TestTuneInClassifyItemMarksUnrecognizedTypesInsteadOfDroppingThem covers
// the shared classifier used by tuneInSearchSection, TuneInSearchNext, and
// TuneInNavigateProfile's container children. Previously, tuneInSearchSection
// and TuneInSearchNext each had their own switch with no default case, so an
// item whose "Type" wasn't one of the 5 known values was silently omitted --
// TuneIn's type list isn't guaranteed stable, and this hid real content with
// no trace it existed. An unrecognized type now still gets a playback link,
// with its Subtitle marked so a playback attempt that doesn't pan out reads
// as "this content type isn't fully supported yet" rather than a mystery
// broken link.
func TestTuneInClassifyItemMarksUnrecognizedTypesInsteadOfDroppingThem(t *testing.T) {
	t.Run("known playable type is unmarked", func(t *testing.T) {
		item := tuneInClassifyItem(map[string]interface{}{
			"Type": "Station", "Title": "Jazz FM", "GuideId": "s123", "Subtitle": "Smooth Jazz",
		})

		if item.Subtitle != "Smooth Jazz" {
			t.Errorf("Subtitle = %q, want unmodified %q", item.Subtitle, "Smooth Jazz")
		}
	})

	t.Run("Program/Profile type navigates instead of playing", func(t *testing.T) {
		item := tuneInClassifyItem(map[string]interface{}{
			"Type": "Program", "Title": "Some Show", "GuideId": "p123",
		})

		if item.Links == nil || item.Links.BmxNavigate == nil {
			t.Errorf("Program item has no BmxNavigate link: %+v", item)
		}
	})

	t.Run("unrecognized type without existing subtitle", func(t *testing.T) {
		item := tuneInClassifyItem(map[string]interface{}{
			"Type": "SomeFutureType", "Title": "Mystery Item", "GuideId": "x123",
		})

		if item.Links == nil || item.Links.BmxPlayback == nil {
			t.Fatalf("unrecognized-type item has no playback link, want it still playable: %+v", item)
		}

		if item.Subtitle != "Unrecognized type, may not play" {
			t.Errorf("Subtitle = %q, want the unrecognized-type marker", item.Subtitle)
		}
	})

	t.Run("unrecognized type with existing subtitle appends the marker", func(t *testing.T) {
		item := tuneInClassifyItem(map[string]interface{}{
			"Type": "SomeFutureType", "Title": "Mystery Item", "GuideId": "x123", "Subtitle": "From Mystery Network",
		})

		if !strings.Contains(item.Subtitle, "From Mystery Network") || !strings.Contains(item.Subtitle, "Unrecognized type") {
			t.Errorf("Subtitle = %q, want both the original subtitle and the unrecognized-type marker", item.Subtitle)
		}
	})

	t.Run("empty type falls back to className, still unrecognized if className is also unknown", func(t *testing.T) {
		item := tuneInClassifyItem(map[string]interface{}{
			"className": "weirdLegacyThing", "Title": "Legacy Item", "GuideId": "y123",
		})

		if !strings.Contains(item.Subtitle, "Unrecognized type") {
			t.Errorf("Subtitle = %q, want the unrecognized-type marker", item.Subtitle)
		}
	})
}
