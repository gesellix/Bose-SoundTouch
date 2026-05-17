// Package bmx implements minimal helper calls to public TuneIn endpoints
// and wraps them into Bose-compatible response models.
package bmx

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TuneIn endpoint templates used to resolve station and stream URLs.
const (
	TuneInDescribe     = "https://opml.radiotime.com/describe.ashx?id=%s"
	TuneInNavigateAshx = "http://opml.radiotime.com/?render=json"
	TuneInSearchAPI    = "https://api.radiotime.com/profiles?fulltextsearch=true&version=1.3&query="

	// TuneInProfileContents is the modern JSON API that lists a
	// program's (`p<N>`) episodes. The legacy OPML endpoints can't —
	// `Tune.ashx?id=p<N>` returns `#STATUS: 400`, `Browse.ashx?id=p<N>`
	// only surfaces related genres + networks. Same payload is served
	// from api.tunein.com and api.radiotime.com; we use radiotime
	// because TuneInNavigateProfile already navigates there via
	// Pivots.Contents.Url, so all program-related traffic stays on the
	// same host that's already in allowedTuneInHosts. See
	// `_/i226/tunein-api-findings.md` for the full endpoint map.
	TuneInProfileContents = "https://api.radiotime.com/profiles/%s/contents?version=1.3"

	// DefaultTuneInStreamFormats is the comma-separated format list
	// AfterTouch sends to TuneIn's Tune.ashx by default. Matches the
	// pre-2026-05-10 behaviour from before PR #249 added "hls"
	// unconditionally — HLS playback is broken on SoundTouch 10/
	// firmware 27 (and probably the rest of the line; see #292).
	// Speakers receive an .m3u8 playlist URL they can't parse, blink
	// amber, fall silent. Operators with HLS-compatible speakers can
	// override via Settings.TuneInStreamFormats.
	DefaultTuneInStreamFormats = "mp3,aac,ogg"
)

// TuneInStream returns the formatted Tune.ashx URL for a station or
// podcast. The formats argument controls the formats= query parameter;
// empty falls back to DefaultTuneInStreamFormats. Operators can set
// arbitrary lists (e.g. "mp3,aac,ogg,hls" to re-enable HLS, or
// "aac" to force a single format) via Settings.TuneInStreamFormats.
// The value is passed through verbatim — no token-level validation.
func TuneInStream(stationID, formats string) string {
	formats = strings.TrimSpace(formats)
	if formats == "" {
		formats = DefaultTuneInStreamFormats
	}

	return fmt.Sprintf("http://opml.radiotime.com/Tune.ashx?id=%s&formats=%s", stationID, formats)
}

var tuneInClient = &http.Client{Timeout: 10 * time.Second}

// allowedTuneInHosts restricts outbound fetches to known TuneIn domains.
var allowedTuneInHosts = map[string]bool{
	"opml.radiotime.com": true,
	"api.radiotime.com":  true,
}

func isTuneInURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return allowedTuneInHosts[u.Hostname()]
}

// isTuneInOpmlURI returns true when the URL's host is opml.radiotime.com,
// used to select the OPML/ashx parser over the JSON API parser.
func isTuneInOpmlURI(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(u.Hostname(), "opml.radiotime.com")
}

// tuneInRenderJSONURI returns the URL with render=json set as a query parameter,
// replacing any existing render value instead of appending a duplicate.
func tuneInRenderJSONURI(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := u.Query()
	q.Set("render", "json")
	u.RawQuery = q.Encode()

	return u.String()
}

// tuneInSearchURI returns the TuneIn search API URL with the query properly URL-encoded.
func tuneInSearchURI(query string) string {
	return TuneInSearchAPI + url.QueryEscape(query)
}

func fetchJSON(fetchURL string) (map[string]interface{}, error) {
	if !isTuneInURL(fetchURL) {
		return nil, fmt.Errorf("URL not in allowed list: %s", fetchURL)
	}

	resp, err := tuneInClient.Get(fetchURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func decodeBase64URI(encoded string) (string, error) {
	b, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(encoded)
	}

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// TuneInNavigate returns a live browse response for the given encoded TuneIn URI.
// Pass subsection as nil for a full page, or a pointer to an int for a single subsection.
func TuneInNavigate(encodedURI string, subsection *int) (*models.BmxNavResponse, error) {
	var (
		tuneInURI     string
		bmxSearchLink *models.Link
	)

	if encodedURI != "" {
		decoded, err := decodeBase64URI(encodedURI)
		if err != nil {
			return nil, err
		}

		tuneInURI = decoded
	} else {
		tuneInURI = TuneInNavigateAshx
		templated := true
		bmxSearchLink = &models.Link{
			Filters:   []interface{}{},
			Href:      "/v1/search?q={query}",
			Templated: &templated,
		}
	}

	var (
		sections []models.BmxNavSection
		err      error
	)

	if isTuneInOpmlURI(tuneInURI) {
		sections, err = tuneInSectionsAshx(tuneInURI, subsection)
	} else {
		sections, err = tuneInSectionsJSONAPI(tuneInURI, subsection)
	}

	if err != nil {
		return nil, err
	}

	var subsectionPart, uriPart string
	if subsection != nil {
		subsectionPart = fmt.Sprintf("/sub/%d", *subsection)
	}

	if encodedURI != "" {
		uriPart = "/" + encodedURI
	}

	return &models.BmxNavResponse{
		Links: &models.Links{
			Self:      &models.Link{Href: fmt.Sprintf("/v1/navigate%s%s", subsectionPart, uriPart)},
			BmxSearch: bmxSearchLink,
		},
		BmxSections: sections,
		Layout:      "classic",
	}, nil
}

func tuneInSectionsAshx(tuneInURI string, subsection *int) ([]models.BmxNavSection, error) {
	data, err := fetchJSON(tuneInURI)
	if err != nil {
		return nil, err
	}

	layout := "list"

	var (
		sections []models.BmxNavSection
		topItems []models.BmxNavItem
	)

	body, _ := data["body"].([]interface{})

	for idx, rawItem := range body {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}

		itemType, _ := item["type"].(string)
		if itemType == "link" {
			topItems = append(topItems, tuneInNavigateLink(item))
			continue
		}

		if subsection != nil && *subsection != idx {
			continue
		}

		if len(body) == 1 || subsection != nil {
			layout = "responsiveGrid"
		} else {
			layout = "ribbon"
		}

		maxCount := 5
		if layout == "responsiveGrid" {
			maxCount = 500
		}

		sectionTitle, _ := item["text"].(string)

		var sectionItems []models.BmxNavItem

		count := 0

		children, _ := item["children"].([]interface{})
		for _, rawChild := range children {
			child, ok := rawChild.(map[string]interface{})
			if !ok {
				continue
			}

			childType, _ := child["type"].(string)
			switch childType {
			case "audio":
				sectionItems = append(sectionItems, tuneInNavigatePlayItem(child))
			case "link":
				sectionItems = append(sectionItems, tuneInNavigateLink(child))
			}

			count++
			if count >= maxCount {
				break
			}
		}

		encURI := base64.URLEncoding.EncodeToString([]byte(tuneInURI))
		sections = append(sections, models.BmxNavSection{
			Links:  &models.Links{Self: &models.Link{Href: fmt.Sprintf("/v1/navigate/sub/%d/%s", idx, encURI)}},
			Items:  sectionItems,
			Layout: layout,
			Name:   sectionTitle,
		})
	}

	head, _ := data["head"].(map[string]interface{})
	title, _ := head["title"].(string)

	var subsectionPart string
	if subsection != nil {
		subsectionPart = fmt.Sprintf("sub/%d/", *subsection)
	}

	encURI := base64.URLEncoding.EncodeToString([]byte(tuneInURI))
	sections = append(sections, models.BmxNavSection{
		Links:  &models.Links{Self: &models.Link{Href: fmt.Sprintf("/v1/navigate/%s%s", subsectionPart, encURI)}},
		Items:  topItems,
		Layout: layout,
		Name:   title,
	})

	return sections, nil
}

func tuneInSectionsJSONAPI(tuneInURI string, subsection *int) ([]models.BmxNavSection, error) {
	data, err := fetchJSON(tuneInURI)
	if err != nil {
		return nil, err
	}

	var sections []models.BmxNavSection

	items, _ := data["Items"].([]interface{})
	for idx, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}

		if subsection != nil && *subsection != idx {
			continue
		}

		itemType, _ := item["Type"].(string)
		containerType, _ := item["ContainerType"].(string)

		if itemType == "Container" && containerType != "NotPlayableStations" {
			sections = append(sections, tuneInSearchSection(item, idx, "", "shortList"))
		}
	}

	return sections, nil
}

func tuneInNavigatePlayItem(item map[string]interface{}) models.BmxNavItem {
	guideID, _ := item["guide_id"].(string)
	imageURL, _ := item["image"].(string)
	text, _ := item["text"].(string)
	subtext, _ := item["subtext"].(string)

	playbackHref := fmt.Sprintf("/v1/playback/station/%s", guideID)

	return models.BmxNavItem{
		Links: &models.Links{
			BmxPlayback: &models.Link{Href: playbackHref, Type: "stationurl"},
			BmxPreset:   &models.Link{ContainerArt: imageURL, Href: guideID, Name: text, Type: "stationurl"},
		},
		ImageUrl: imageURL,
		Name:     text,
		Subtitle: subtext,
	}
}

func tuneInNavigateLink(item map[string]interface{}) models.BmxNavItem {
	rawURL, _ := item["URL"].(string)
	imageURL, _ := item["image"].(string)
	text, _ := item["text"].(string)
	subtext, _ := item["subtext"].(string)

	encURL := base64.URLEncoding.EncodeToString([]byte(tuneInRenderJSONURI(rawURL)))

	return models.BmxNavItem{
		Links:    &models.Links{BmxNavigate: &models.Link{Href: fmt.Sprintf("/v1/navigate/%s", encURL)}},
		ImageUrl: imageURL,
		Name:     text,
		Subtitle: subtext,
	}
}

// TuneInSearch returns live search results from TuneIn for the given query.
func TuneInSearch(query string) (*models.BmxNavResponse, error) {
	tuneInURI := tuneInSearchURI(query)

	templated := true
	bmxSearchLink := &models.Link{
		Filters:   []interface{}{},
		Href:      "/v1/search?q={query}",
		Templated: &templated,
	}

	data, err := fetchJSON(tuneInURI)
	if err != nil {
		return nil, err
	}

	var sections []models.BmxNavSection

	items, _ := data["Items"].([]interface{})
	for idx, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}

		itemType, _ := item["Type"].(string)
		containerType, _ := item["ContainerType"].(string)

		if itemType == "Container" && containerType != "NotPlayableStations" {
			sections = append(sections, tuneInSearchSection(item, idx, query, "shortList"))
		}
	}

	return &models.BmxNavResponse{
		Links: &models.Links{
			Self:      &models.Link{Href: fmt.Sprintf("/v1/search?q=%s", url.QueryEscape(query))},
			BmxSearch: bmxSearchLink,
		},
		BmxSections: sections,
		Layout:      "classic",
	}, nil
}

func tuneInSearchSection(item map[string]interface{}, idx int, query, layout string) models.BmxNavSection {
	pivots, _ := item["Pivots"].(map[string]interface{})
	more, _ := pivots["More"].(map[string]interface{})
	pivotURL, _ := more["Url"].(string)

	var href string
	if pivotURL != "" {
		href = fmt.Sprintf("/v1/navigate/%s", base64.URLEncoding.EncodeToString([]byte(pivotURL)))
	} else {
		encodedQuery := base64.URLEncoding.EncodeToString([]byte(tuneInSearchURI(query)))
		href = fmt.Sprintf("/v1/navigate/sub/%d/%s", idx, encodedQuery)
	}

	var sectionItems []models.BmxNavItem

	children, _ := item["Children"].([]interface{})
	for _, rawChild := range children {
		child, ok := rawChild.(map[string]interface{})
		if !ok {
			continue
		}

		childType, _ := child["Type"].(string)
		switch childType {
		case "Station":
			sectionItems = append(sectionItems, tuneInSearchPlayItem(child))
		case "Topic":
			sectionItems = append(sectionItems, tuneInSearchTopic(child))
		case "Program":
			sectionItems = append(sectionItems, tuneInSearchProfile(child, "Program"))
		case "Artist":
			sectionItems = append(sectionItems, tuneInSearchProfile(child, "Artist"))
		case "Category":
			actions, _ := child["Actions"].(map[string]interface{})
			browse, _ := actions["Browse"].(map[string]interface{})
			categoryHref, _ := browse["Url"].(string)
			encHref := base64.URLEncoding.EncodeToString([]byte(categoryHref))
			image, _ := child["Image"].(string)
			title, _ := child["Title"].(string)
			subtitle, _ := child["Subtitle"].(string)
			sectionItems = append(sectionItems, models.BmxNavItem{
				Links:    &models.Links{BmxNavigate: &models.Link{Href: fmt.Sprintf("/v1/navigate/%s", encHref)}},
				ImageUrl: image,
				Name:     title,
				Subtitle: subtitle,
			})
		}
	}

	title, _ := item["Title"].(string)

	return models.BmxNavSection{
		Links:  &models.Links{Self: &models.Link{Href: href}},
		Items:  sectionItems,
		Layout: layout,
		Name:   title,
	}
}

func tuneInSearchPlayItem(item map[string]interface{}) models.BmxNavItem {
	guideID, _ := item["GuideId"].(string)
	image, _ := item["Image"].(string)
	title, _ := item["Title"].(string)
	subtitle, _ := item["Subtitle"].(string)

	href := fmt.Sprintf("/v1/playback/station/%s", guideID)

	return models.BmxNavItem{
		Links: &models.Links{
			BmxPlayback: &models.Link{Href: href, Type: "stationurl"},
			BmxPreset:   &models.Link{ContainerArt: image, Href: href, Name: title, Type: "stationurl"},
		},
		ImageUrl: image,
		Name:     title,
		Subtitle: subtitle,
	}
}

func tuneInSearchTopic(item map[string]interface{}) models.BmxNavItem {
	guideID, _ := item["GuideId"].(string)
	image, _ := item["Image"].(string)
	title, _ := item["Title"].(string)
	subtitle, _ := item["Subtitle"].(string)

	encodedName := base64.URLEncoding.EncodeToString([]byte(title))
	href := fmt.Sprintf("/v1/playback/episodes/%s?encoded_name=%s", guideID, encodedName)

	return models.BmxNavItem{
		Links: &models.Links{
			BmxPlayback: &models.Link{Href: href, Type: "tracklisturl"},
			BmxPreset:   &models.Link{ContainerArt: image, Href: href, Name: title, Type: "tracklisturl"},
		},
		ImageUrl: image,
		Name:     title,
		Subtitle: subtitle,
	}
}

func tuneInSearchProfile(item map[string]interface{}, name string) models.BmxNavItem {
	guideID, _ := item["GuideId"].(string)
	image, _ := item["Image"].(string)
	title, _ := item["Title"].(string)
	subtitle, _ := item["Subtitle"].(string)

	actions, _ := item["Actions"].(map[string]interface{})
	profile, _ := actions["Profile"].(map[string]interface{})
	apiURL, _ := profile["Url"].(string)
	apiURLEncoded := base64.URLEncoding.EncodeToString([]byte(apiURL))

	links := &models.Links{
		BmxNavigate: &models.Link{Href: fmt.Sprintf("/v1/navigate/profiles/%s/%s/%s", name, guideID, apiURLEncoded)},
		BmxPreset:   &models.Link{ContainerArt: image, Href: fmt.Sprintf("/v1/preset/program/%s", guideID), Name: title, Type: "tracklisturl"},
	}

	// Programs are containers, but with the `p` → `t` expansion in
	// TuneInPlaybackPodcast a single "play this program" click can now
	// route to the newest episode. Surface that as a BmxPlayback link so
	// the web UI renders a play button on the program card itself, not
	// just on individual episode cards reached by drilling in. Artists
	// stay navigate-only — there's no single sensible "play this artist"
	// stream.
	if name == "Program" && guideID != "" {
		encodedName := base64.URLEncoding.EncodeToString([]byte(title))
		playbackHref := fmt.Sprintf("/v1/playback/episodes/%s?encoded_name=%s", guideID, encodedName)
		links.BmxPlayback = &models.Link{Href: playbackHref, Type: "tracklisturl"}
	}

	return models.BmxNavItem{
		Links:    links,
		ImageUrl: image,
		Name:     title,
		Subtitle: subtitle,
	}
}

// TuneInNavigateProfile returns a profile (artist/program) navigation response.
func TuneInNavigateProfile(encodedURI string) (*models.BmxNavResponse, error) {
	tuneInURI, err := decodeBase64URI(encodedURI)
	if err != nil {
		return nil, err
	}

	profileData, err := fetchJSON(tuneInURI)
	if err != nil {
		return nil, err
	}

	profileItem, _ := profileData["Item"].(map[string]interface{})
	profileTitle, _ := profileItem["Title"].(string)
	profileImage, _ := profileItem["Image"].(string)
	profileSubtitle, _ := profileItem["Subtitle"].(string)
	profileType, _ := profileItem["Type"].(string)
	profileGuideID, _ := profileItem["GuideId"].(string)

	heroItem := models.BmxNavItem{Name: profileTitle, ImageUrl: profileImage, Subtitle: profileSubtitle}

	// Surface "play latest episode" on the profile hero so users don't
	// have to scroll to the episode list. Matches the BmxPlayback link
	// emitted for Program cards in search results; the backend
	// p<N> → t<N> expansion resolves the actual stream.
	if profileType == "Program" && profileGuideID != "" {
		encodedName := base64.URLEncoding.EncodeToString([]byte(profileTitle))
		playbackHref := fmt.Sprintf("/v1/playback/episodes/%s?encoded_name=%s", profileGuideID, encodedName)
		heroItem.Links = &models.Links{
			BmxPlayback: &models.Link{Href: playbackHref, Type: "tracklisturl"},
		}
	}

	sections := []models.BmxNavSection{
		{
			Items:  []models.BmxNavItem{heroItem},
			Layout: "hero",
			Name:   "",
		},
	}

	pivots, _ := profileItem["Pivots"].(map[string]interface{})
	contents, _ := pivots["Contents"].(map[string]interface{})
	contentsURL, _ := contents["Url"].(string)

	if contentsURL != "" {
		if contentsData, fetchErr := fetchJSON(contentsURL); fetchErr == nil {
			contentsItems, _ := contentsData["Items"].([]interface{})
			for idx, rawItem := range contentsItems {
				item, ok := rawItem.(map[string]interface{})
				if !ok {
					continue
				}

				itemType, _ := item["Type"].(string)
				containerType, _ := item["ContainerType"].(string)

				if itemType == "Container" && containerType != "NotPlayableStations" {
					sections = append(sections, tuneInSearchSection(item, idx, "", "list"))
				}
			}
		}
	}

	return &models.BmxNavResponse{
		Links:       &models.Links{Self: &models.Link{Href: fmt.Sprintf("/v1/navigate/profiles/%s", encodedURI)}},
		BmxSections: sections,
		Layout:      "classic",
	}, nil
}

// parseTuneInStreamBody filters a Tune.ashx response body down to the
// playable stream URLs. TuneIn responds with HTTP 200 even on errors,
// embedding a `#STATUS: <code>` comment line in the body (the body is
// pls/m3u-like, so `#`-prefixed lines are comments — including error
// markers like `#STATUS: 400`). Without this filter the caller would
// happily pass `#STATUS: 400` to the speaker as if it were a stream URL.
//
// Returns the cleaned list of URL strings (TrimSpaced, comment lines
// dropped, empty lines dropped). Returns an error if no playable URL
// remains so callers surface a real 500 instead of silently corrupting
// the playback response.
func parseTuneInStreamBody(body []byte, guideID string) ([]string, error) {
	raw := strings.Split(strings.TrimSpace(string(body)), "\n")
	out := make([]string, 0, len(raw))

	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		out = append(out, line)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("TuneIn returned no playable stream URL for guide-id %q (body: %q)",
			guideID, strings.TrimSpace(string(body)))
	}

	return out, nil
}

// tuneInProfileContentsResponse models the subset of the
// api.tunein.com/profiles/{id}/contents JSON we need to pick a
// program's newest episode. The endpoint returns substantially more
// fields per item; everything outside this struct is ignored.
type tuneInProfileContentsResponse struct {
	Items []tuneInProfileContentsItem `json:"Items"`
}

type tuneInProfileContentsItem struct {
	ContainerType      string                       `json:"ContainerType"`
	Title              string                       `json:"Title"`
	AccessibilityTitle string                       `json:"AccessibilityTitle"`
	Children           []tuneInProfileContentsTopic `json:"Children"`
}

type tuneInProfileContentsTopic struct {
	GuideId string `json:"GuideId"`
	Type    string `json:"Type"`
	Title   string `json:"Title"`
	Image   string `json:"Image"`
}

// parseTuneInProgramContents walks a profile/contents JSON body and
// returns the guide-id of the newest playable episode. The contract:
//
//   - Items[] entry with ContainerType=="Topics" and Title (or
//     AccessibilityTitle) equal to "Episodes" is treated as the
//     authoritative episode list.
//   - If no item matches by name, the first ContainerType=="Topics"
//     entry is used as fallback — TuneIn occasionally varies the
//     localised title.
//   - Inside the chosen container the first child with a `t`-prefixed
//     GuideId wins. TuneIn orders children newest-first.
//
// Returns a wrapped error if the body is malformed or contains no
// playable topic; callers surface this as a 500 rather than handing
// the speaker a broken stream URL.
func parseTuneInProgramContents(body []byte, programID string) (episodeID string, err error) {
	var parsed tuneInProfileContentsResponse
	if decErr := json.Unmarshal(body, &parsed); decErr != nil {
		return "", fmt.Errorf("decode TuneIn profile/contents for %q: %w", programID, decErr)
	}

	var fallback *tuneInProfileContentsItem

	for i := range parsed.Items {
		item := &parsed.Items[i]
		if item.ContainerType != "Topics" {
			continue
		}

		if fallback == nil {
			fallback = item
		}

		if strings.EqualFold(item.Title, "Episodes") ||
			strings.EqualFold(item.AccessibilityTitle, "Episodes") {
			if id := firstTuneInTopicGuideID(item.Children); id != "" {
				return id, nil
			}
		}
	}

	if fallback != nil {
		if id := firstTuneInTopicGuideID(fallback.Children); id != "" {
			return id, nil
		}
	}

	return "", fmt.Errorf("no playable episode found in TuneIn profile/contents for program %q", programID)
}

func firstTuneInTopicGuideID(children []tuneInProfileContentsTopic) string {
	for _, child := range children {
		if strings.HasPrefix(child.GuideId, "t") {
			return child.GuideId
		}
	}

	return ""
}

// resolveTuneInProgramLatestEpisode fetches the program's profile from
// api.tunein.com and returns the newest playable episode's topic
// guide-id (the `t<N>` form Tune.ashx accepts). The legacy OPML
// endpoints can't enumerate program episodes; see
// `_/i226/tunein-api-findings.md` for the full endpoint contract.
func resolveTuneInProgramLatestEpisode(programID string) (episodeID string, err error) {
	contentsURL := fmt.Sprintf(TuneInProfileContents, programID)

	resp, err := http.Get(contentsURL)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TuneIn profile/contents returned status %d for program %q",
			resp.StatusCode, programID)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return parseTuneInProgramContents(body, programID)
}

// TuneInDescribeMeta fetches just the display name and logo URL for a TuneIn
// guide ID via the same describe endpoint TuneInPlayback uses. Useful for
// CLI / UI enrichment that wants to populate ContentItem.ItemName +
// ContainerArt before sending a SelectContentItem to the speaker — without
// resolving the full stream URL.
//
// Returns empty strings (and a nil error) if the describe payload doesn't
// contain a recognisable station / show element. Network errors and XML
// decode errors surface verbatim.
func TuneInDescribeMeta(id string) (name, logo string, err error) {
	describeURL := fmt.Sprintf(TuneInDescribe, id)

	resp, err := http.Get(describeURL)
	if err != nil {
		return "", "", err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	// Same shape TuneInPlayback parses for stations. For programs and
	// episodes the describe endpoint returns analogous structures; if
	// the station element is absent the response yields empty strings
	// and the caller can fall back to user-supplied values.
	var opml struct {
		Body struct {
			Outline struct {
				Station struct {
					Name string `xml:"name"`
					Logo string `xml:"logo"`
				} `xml:"station"`
			} `xml:"outline"`
		} `xml:"body"`
	}

	if uErr := xml.Unmarshal(body, &opml); uErr != nil {
		return "", "", uErr
	}

	return opml.Body.Outline.Station.Name, opml.Body.Outline.Station.Logo, nil
}

// TuneInPlayback resolves a live radio station and returns a Bose-compatible
// playback response with primary stream and variants. formats is the
// comma-separated list passed to Tune.ashx?formats=… ; empty falls back to
// DefaultTuneInStreamFormats (the SoundTouch-line-compatible shape).
func TuneInPlayback(stationID, formats string) (*models.BmxPlaybackResponse, error) {
	describeURL := fmt.Sprintf(TuneInDescribe, stationID)

	resp, err := http.Get(describeURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var opml struct {
		Body struct {
			Outline struct {
				Station struct {
					Name string `xml:"name"`
					Logo string `xml:"logo"`
				} `xml:"station"`
			} `xml:"outline"`
		} `xml:"body"`
	}

	if unmarshalErr := xml.Unmarshal(body, &opml); unmarshalErr != nil {
		return nil, unmarshalErr
	}

	station := opml.Body.Outline.Station

	streamReq := TuneInStream(stationID, formats)

	streamResp, err := http.Get(streamReq)
	if err != nil {
		return nil, err
	}

	defer func() { _ = streamResp.Body.Close() }()

	streamBody, err := io.ReadAll(streamResp.Body)
	if err != nil {
		return nil, err
	}

	streamURLList, err := parseTuneInStreamBody(streamBody, stationID)
	if err != nil {
		return nil, err
	}

	streamID := "e3342"
	listenID := "3432432423"
	bmxReportingQS := url.Values{}
	bmxReportingQS.Set("stream_id", streamID)
	bmxReportingQS.Set("guide_id", stationID)
	bmxReportingQS.Set("listen_id", listenID)
	bmxReportingQS.Set("stream_type", "liveRadio")
	bmxReporting := "/v1/report?" + bmxReportingQS.Encode()

	var streams []models.Stream

	for _, sURL := range streamURLList {
		sURL = strings.TrimSpace(sURL)
		if sURL == "" {
			continue
		}

		streams = append(streams, models.Stream{
			Links: &models.Links{
				BmxReporting: &models.Link{Href: bmxReporting},
			},
			HasPlaylist:       true,
			IsRealtime:        true,
			BufferingTimeout:  20,
			ConnectingTimeout: 10,
			StreamUrl:         sURL,
		})
	}

	audio := models.Audio{
		HasPlaylist: true,
		IsRealtime:  true,
		MaxTimeout:  60,
		StreamUrl:   streamURLList[0],
		Streams:     streams,
	}

	response := &models.BmxPlaybackResponse{
		Links: &models.Links{
			BmxFavorite:   &models.Link{Href: "/v1/favorite/" + stationID},
			BmxNowPlaying: &models.Link{Href: "/v1/now-playing/station/" + stationID, UseInternalClient: "ALWAYS"},
			BmxReporting:  &models.Link{Href: bmxReporting},
		},
		Audio:      audio,
		ImageUrl:   station.Logo,
		IsFavorite: new(bool), // defaults to false
		Name:       station.Name,
		StreamType: "liveRadio",
	}

	return response, nil
}

// TuneInPodcastInfo returns minimal podcast/episode metadata for UI selection.
func TuneInPodcastInfo(podcastID, encodedName string) (*models.BmxPodcastInfoResponse, error) {
	// Bose app sometimes sends non-standard base64, so try both standard and URL-safe
	nameBytes, err := base64.URLEncoding.DecodeString(encodedName)
	if err != nil {
		nameBytes, err = base64.StdEncoding.DecodeString(encodedName)
	}

	if err != nil {
		return nil, err
	}

	name := string(nameBytes)

	track := models.Track{
		Links: &models.Links{
			BmxTrack: &models.Link{Href: fmt.Sprintf("/v1/playback/episode/%s", podcastID)},
		},
		IsSelected: false,
		Name:       name,
	}

	response := &models.BmxPodcastInfoResponse{
		Links: &models.Links{
			Self: &models.Link{Href: fmt.Sprintf("/v1/playback/episodes/%s?encoded_name=%s", podcastID, encodedName)},
		},
		Name:            name,
		ShuffleDisabled: true,
		RepeatDisabled:  true,
		StreamType:      "onDemand",
		Tracks:          []models.Track{track},
	}

	return response, nil
}

// TuneInPlaybackPodcast resolves an on-demand podcast episode and returns
// a playback response suitable for SoundTouch devices. formats has the
// same semantics as in TuneInPlayback.
//
// Accepts three TuneIn guide-id shapes:
//   - `t<N>` — topic/episode; played directly.
//   - `e<N>` — live episode; played directly.
//   - `p<N>` — podcast program (a container, not a stream). Expanded
//     to its newest episode via the JSON profile/contents API before
//     resolving the stream URL. The legacy OPML `Tune.ashx?id=p<N>`
//     would return `#STATUS: 400` for this case.
func TuneInPlaybackPodcast(podcastID, formats string) (*models.BmxPlaybackResponse, error) {
	if strings.HasPrefix(podcastID, "p") {
		episodeID, resolveErr := resolveTuneInProgramLatestEpisode(podcastID)
		if resolveErr != nil {
			return nil, resolveErr
		}

		podcastID = episodeID
	}

	describeURL := fmt.Sprintf(TuneInDescribe, podcastID)

	resp, err := http.Get(describeURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var opml struct {
		Body struct {
			Outline struct {
				Topic struct {
					Title     string `xml:"title"`
					ShowTitle string `xml:"show_title"`
					Duration  string `xml:"duration"`
					ShowID    string `xml:"show_id"`
					Logo      string `xml:"logo"`
				} `xml:"topic"`
			} `xml:"outline"`
		} `xml:"body"`
	}

	if unmarshalErr := xml.Unmarshal(body, &opml); unmarshalErr != nil {
		return nil, unmarshalErr
	}

	topic := opml.Body.Outline.Topic

	streamReq := TuneInStream(podcastID, formats)

	streamResp, err := http.Get(streamReq)
	if err != nil {
		return nil, err
	}

	defer func() { _ = streamResp.Body.Close() }()

	streamBody, err := io.ReadAll(streamResp.Body)
	if err != nil {
		return nil, err
	}

	streamURLList, err := parseTuneInStreamBody(streamBody, podcastID)
	if err != nil {
		return nil, err
	}

	streamID := "e3342"
	listenID := "3432432423"
	bmxReportingQS := url.Values{}
	bmxReportingQS.Set("stream_id", streamID)
	bmxReportingQS.Set("guide_id", podcastID)
	bmxReportingQS.Set("listen_id", listenID)
	bmxReportingQS.Set("stream_type", "onDemand")
	bmxReporting := "/v1/report?" + bmxReportingQS.Encode()

	var streams []models.Stream

	for _, sURL := range streamURLList {
		sURL = strings.TrimSpace(sURL)
		if sURL == "" {
			continue
		}

		streams = append(streams, models.Stream{
			Links: &models.Links{
				BmxReporting: &models.Link{Href: bmxReporting},
			},
			HasPlaylist:       true,
			IsRealtime:        false,
			BufferingTimeout:  20,
			ConnectingTimeout: 10,
			StreamUrl:         sURL,
		})
	}

	audio := models.Audio{
		HasPlaylist: true,
		IsRealtime:  false,
		MaxTimeout:  60,
		StreamUrl:   streamURLList[0],
		Streams:     streams,
	}

	duration, _ := strconv.Atoi(topic.Duration)

	response := &models.BmxPlaybackResponse{
		Links: &models.Links{
			BmxFavorite:  &models.Link{Href: fmt.Sprintf("/v1/favorite/%s", topic.ShowID)},
			BmxReporting: &models.Link{Href: bmxReporting},
		},
		Artist: struct {
			Name string `json:"name,omitempty" xml:"name,omitempty"`
		}{Name: topic.ShowTitle},
		Audio:           audio,
		Duration:        duration,
		ImageUrl:        topic.Logo,
		IsFavorite:      new(bool),
		Name:            topic.Title,
		ShuffleDisabled: true,
		RepeatDisabled:  true,
		StreamType:      "onDemand",
	}

	return response, nil
}

// BuildCustomStreamResponse builds a playback response from streamUrl, imageUrl, and name.
func BuildCustomStreamResponse(streamURL, imageURL, name string) (*models.BmxPlaybackResponse, error) {
	streamList := []models.Stream{
		{
			HasPlaylist: true,
			IsRealtime:  true,
			StreamUrl:   streamURL,
		},
	}

	audio := models.Audio{
		HasPlaylist: true,
		IsRealtime:  true,
		StreamUrl:   streamURL,
		Streams:     streamList,
	}

	response := &models.BmxPlaybackResponse{
		Audio:      audio,
		ImageUrl:   imageURL,
		Name:       name,
		StreamType: "liveRadio",
	}

	return response, nil
}

// PlayCustomStream builds a playback response from a base64-encoded JSON blob
// with fields streamUrl, imageUrl, and name.
func PlayCustomStream(data string) (*models.BmxPlaybackResponse, error) {
	// Bose app sometimes sends non-standard base64, so try both standard and URL-safe
	jsonStr, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		jsonStr, err = base64.StdEncoding.DecodeString(data)
	}

	if err != nil {
		return nil, err
	}

	var jsonObj struct {
		StreamURL string `json:"streamUrl"`
		ImageURL  string `json:"imageUrl"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(jsonStr, &jsonObj); err != nil {
		return nil, err
	}

	return BuildCustomStreamResponse(jsonObj.StreamURL, jsonObj.ImageURL, jsonObj.Name)
}
