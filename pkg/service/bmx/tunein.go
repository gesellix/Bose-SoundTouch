package bmx

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TuneIn endpoint constants. The base URLs themselves are configurable vars
// (see tuneInOpmlTuneBase and friends below, set via SetTuneInEndpoints); only
// the format-list default is a fixed constant.
const (
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

	return fmt.Sprintf("%s/Tune.ashx?id=%s&formats=%s", tuneInOpmlTuneBase, stationID, formats)
}

// allowedTuneInHosts restricts outbound fetches to known TuneIn domains.
var allowedTuneInHosts = map[string]bool{
	"opml.radiotime.com": true,
	"api.radiotime.com":  true,
}

// TuneIn endpoint base URLs. They default to the real TuneIn hosts (matching the
// constants above) but can be redirected with SetTuneInEndpoints, e.g. to point
// the playback / describe / search calls at a local mock so an integration suite
// does not depend on the live TuneIn service. opmlBase covers the
// opml.radiotime.com endpoints (Tune.ashx, describe.ashx, navigate); apiBase
// covers the api.radiotime.com endpoints (search, profile contents).
var (
	tuneInOpmlTuneBase     = "http://opml.radiotime.com"
	tuneInOpmlDescribeBase = "https://opml.radiotime.com"
	tuneInOpmlNavigateBase = "http://opml.radiotime.com"
	tuneInAPIBase          = "https://api.radiotime.com"
)

// SetTuneInEndpoints overrides the TuneIn upstream base URLs and registers their
// hosts in the outbound allowlist. Empty arguments leave the corresponding
// default in place. Intended for tests and local mocks; production leaves the
// real TuneIn hosts.
func SetTuneInEndpoints(opmlBase, apiBase string) {
	if opmlBase != "" {
		b := strings.TrimRight(opmlBase, "/")
		tuneInOpmlTuneBase = b
		tuneInOpmlDescribeBase = b
		tuneInOpmlNavigateBase = b

		if u, err := url.Parse(b); err == nil && u.Hostname() != "" {
			allowedTuneInHosts[u.Hostname()] = true
		}
	}

	if apiBase != "" {
		b := strings.TrimRight(apiBase, "/")
		tuneInAPIBase = b

		if u, err := url.Parse(b); err == nil && u.Hostname() != "" {
			allowedTuneInHosts[u.Hostname()] = true
		}
	}
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
	return tuneInAPIBase + "/profiles?fulltextsearch=true&version=1.3&query=" + url.QueryEscape(query)
}

func fetchJSON(fetchURL string) (map[string]interface{}, error) {
	return fetchJSONMap(defaultClient, fetchURL, allowedTuneInHosts)
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
		tuneInURI = tuneInOpmlNavigateBase + "/?render=json"
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
	for idx, item := range body {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if subsection != nil && idx != *subsection {
			continue
		}

		itemType, _ := m["type"].(string)
		switch itemType {
		case "link":
			if children, ok := m["children"].([]interface{}); ok && len(children) > 0 {
				name, _ := m["text"].(string)

				section := models.BmxNavSection{
					Name:  name,
					Items: make([]models.BmxNavItem, 0, len(children)),
				}
				for _, child := range children {
					cm, ok := child.(map[string]interface{})
					if !ok {
						continue
					}

					childType, _ := cm["type"].(string)
					if childType == "audio" {
						section.Items = append(section.Items, tuneInNavigatePlayItem(cm))
					} else {
						section.Items = append(section.Items, tuneInNavigateLink(cm))
					}
				}

				sections = append(sections, section)
			} else {
				topItems = append(topItems, tuneInNavigateLink(m))
			}
		case "audio":
			topItems = append(topItems, tuneInNavigatePlayItem(m))
		case "text":
			// Ignore info text
		}
	}

	if len(topItems) > 0 {
		sections = append([]models.BmxNavSection{{Items: topItems}}, sections...)
	}

	for i := range sections {
		if sections[i].Layout == "" {
			sections[i].Layout = layout
		}
	}

	return sections, nil
}

func tuneInSectionsJSONAPI(tuneInURI string, subsection *int) ([]models.BmxNavSection, error) {
	data, err := fetchJSON(tuneInURI)
	if err != nil {
		return nil, err
	}

	var sections []models.BmxNavSection

	body, _ := data["body"].([]interface{})
	for idx, item := range body {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		if subsection != nil && idx != *subsection {
			continue
		}

		sections = append(sections, tuneInSearchSection(m, idx, "", "list"))
	}

	return sections, nil
}

func tuneInNavigatePlayItem(item map[string]interface{}) models.BmxNavItem {
	name, _ := item["Title"].(string)
	if name == "" {
		name, _ = item["text"].(string)
	}

	stationID, _ := item["GuideId"].(string)
	if stationID == "" {
		stationID, _ = item["guide_id"].(string)
	}

	image, _ := item["image"].(string)
	subtitle, _ := item["subtext"].(string)

	return models.BmxNavItem{
		Name:     name,
		ImageUrl: image,
		Subtitle: subtitle,
		Links: &models.Links{
			BmxPlayback: &models.Link{
				Href: fmt.Sprintf("/v1/playback/station/%s", stationID),
				Type: "stationurl",
			},
		},
	}
}

func tuneInNavigateLink(item map[string]interface{}) models.BmxNavItem {
	name, _ := item["Title"].(string)
	if name == "" {
		name, _ = item["text"].(string)
	}

	image, _ := item["image"].(string)
	subtitle, _ := item["subtext"].(string)
	href, _ := item["URL"].(string)

	return models.BmxNavItem{
		Name:     name,
		ImageUrl: image,
		Subtitle: subtitle,
		Links: &models.Links{
			BmxNavigate: &models.Link{
				Href: "/v1/navigate/" + base64.RawURLEncoding.EncodeToString([]byte(tuneInRenderJSONURI(href))),
			},
		},
	}
}

// TuneInSearch searches TuneIn for the given query.
func TuneInSearch(query string) (*models.BmxNavResponse, error) {
	data, err := fetchJSON(tuneInSearchURI(query))
	if err != nil {
		return nil, err
	}

	navResp := &models.BmxNavResponse{
		Links: &models.Links{
			Self: &models.Link{Href: "/v1/search?q=" + url.QueryEscape(query)},
		},
		Layout: "classic",
	}

	// Try "Items" (v1.3) first, then "body" (legacy)
	items, ok := data["Items"].([]interface{})
	if !ok {
		items, _ = data["body"].([]interface{})
	}

	for idx, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		navResp.BmxSections = append(navResp.BmxSections, tuneInSearchSection(m, idx, query, "grid"))
	}

	return navResp, nil
}

func tuneInSearchSection(item map[string]interface{}, idx int, query, layout string) models.BmxNavSection {
	name, _ := item["Title"].(string)
	if name == "" {
		name, _ = item["text"].(string)
	}

	children, ok := item["Children"].([]interface{})
	if !ok {
		children, _ = item["children"].([]interface{})
	}

	section := models.BmxNavSection{
		Name:   name,
		Layout: layout,
		Items:  make([]models.BmxNavItem, 0, len(children)),
	}

	if query != "" {
		section.Links = &models.Links{
			Self: &models.Link{Href: fmt.Sprintf("/v1/search/sub/%d?q=%s", idx, url.QueryEscape(query))},
		}
	}

	// Pivots.More.Url is the "load more" cursor from the TuneIn profiles API.
	// It is only present when there are more results beyond the first page.
	if pivots, ok := item["Pivots"].(map[string]interface{}); ok {
		if more, ok := pivots["More"].(map[string]interface{}); ok {
			if containerURL, _ := more["Url"].(string); strings.Contains(containerURL, "itemToken") {
				if u, err := url.Parse(containerURL); err == nil && allowedTuneInHosts[u.Hostname()] {
					encoded := base64.RawURLEncoding.EncodeToString([]byte(containerURL))

					if section.Links == nil {
						section.Links = &models.Links{}
					}

					section.Links.BmxNext = &models.Link{Href: "/v1/search/next?cursor=" + encoded}
				}
			}
		}
	}

	for _, child := range children {
		cm, ok := child.(map[string]interface{})
		if !ok {
			continue
		}

		typeStr, _ := cm["Type"].(string)
		if typeStr == "" {
			typeStr, _ = cm["className"].(string)
		}

		switch typeStr {
		case "Station", "PlayItem", "Topic":
			// Topics are single podcast episodes (t<N>) — Tune.ashx
			// accepts them just like station IDs, so the same play-link
			// shape works.
			section.Items = append(section.Items, tuneInSearchPlayItem(cm))
		case "Program", "Profile":
			section.Items = append(section.Items, tuneInSearchProfile(cm, name))
		}
	}

	return section
}

// TuneInSearchNext fetches the remaining results for a section using the opaque
// cursor produced by TuneInSearch. The cursor URL returns a flat Items[] list
// (not nested containers), so we parse items directly rather than via
// tuneInSearchSection. TuneIn typically returns all remaining results in one
// shot; Paging is empty and no further cursor is generated.
func TuneInSearchNext(encodedCursor string) (*models.BmxNavResponse, error) {
	cursorBytes, err := base64.RawURLEncoding.DecodeString(encodedCursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	cursorURL := string(cursorBytes)

	u, err := url.Parse(cursorURL)
	if err != nil || !allowedTuneInHosts[u.Hostname()] {
		return nil, fmt.Errorf("cursor URL not allowed")
	}

	data, err := fetchJSON(cursorURL)
	if err != nil {
		return nil, err
	}

	rawItems, ok := data["Items"].([]interface{})
	if !ok {
		rawItems, _ = data["body"].([]interface{})
	}

	navItems := make([]models.BmxNavItem, 0, len(rawItems))
	for _, raw := range rawItems {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		typeStr, _ := m["Type"].(string)
		switch typeStr {
		case "Station", "PlayItem", "Topic":
			navItems = append(navItems, tuneInSearchPlayItem(m))
		case "Program", "Profile":
			navItems = append(navItems, tuneInSearchProfile(m, ""))
		}
	}

	return &models.BmxNavResponse{
		Layout: "classic",
		BmxSections: []models.BmxNavSection{
			{Items: navItems, Layout: "grid"},
		},
	}, nil
}

func tuneInSearchPlayItem(item map[string]interface{}) models.BmxNavItem {
	name, _ := item["Title"].(string)
	if name == "" {
		name, _ = item["text"].(string)
	}

	stationID, _ := item["GuideId"].(string)
	if stationID == "" {
		stationID, _ = item["guide_id"].(string)
	}

	image, _ := item["Image"].(string)
	if image == "" {
		image, _ = item["image"].(string)
	}

	subtitle, _ := item["Subtitle"].(string)
	if subtitle == "" {
		subtitle, _ = item["subtext"].(string)
	}

	return models.BmxNavItem{
		Name:     name,
		ImageUrl: image,
		Subtitle: subtitle,
		Links: &models.Links{
			BmxPlayback: &models.Link{
				Href: fmt.Sprintf("/v1/playback/station/%s", stationID),
				Type: "stationurl",
			},
		},
	}
}

func tuneInSearchProfile(item map[string]interface{}, _ string) models.BmxNavItem {
	profileName, _ := item["Title"].(string)
	if profileName == "" {
		profileName, _ = item["text"].(string)
	}

	image, _ := item["Image"].(string)
	if image == "" {
		image, _ = item["image"].(string)
	}

	subtitle, _ := item["Subtitle"].(string)
	if subtitle == "" {
		subtitle, _ = item["subtext"].(string)
	}

	href := ""

	if actions, ok := item["Actions"].(map[string]interface{}); ok {
		if profile, ok := actions["Profile"].(map[string]interface{}); ok {
			href, _ = profile["Url"].(string)
		}
	}

	if href == "" {
		href, _ = item["URL"].(string)
	}

	// Programs with a GuideId can be played directly (as tracklisturl).
	// Artists/Stations/etc are typically navigated first.
	if typeStr, _ := item["Type"].(string); typeStr == "Program" {
		if guideID, _ := item["GuideId"].(string); guideID != "" {
			encodedName := base64.URLEncoding.EncodeToString([]byte(profileName))
			playbackHref := fmt.Sprintf("/v1/playback/episodes/%s?encoded_name=%s", guideID, encodedName)

			return models.BmxNavItem{
				Name:     profileName,
				ImageUrl: image,
				Subtitle: subtitle,
				Links: &models.Links{
					BmxPlayback: &models.Link{
						Href: playbackHref,
						Type: "tracklisturl",
					},
					BmxNavigate: &models.Link{
						Href: "/v1/navigate/profiles/" + base64.URLEncoding.EncodeToString([]byte(href)),
					},
				},
			}
		}
	}

	// Profiles for Artists/etc often have a separate navigate path
	// that lists their programs/albums.
	return models.BmxNavItem{
		Name:     profileName,
		ImageUrl: image,
		Subtitle: subtitle,
		Links: &models.Links{
			BmxNavigate: &models.Link{
				Href: "/v1/navigate/profiles/" + base64.RawURLEncoding.EncodeToString([]byte(href)),
			},
		},
	}
}

// TuneInNavigateProfile returns a browse response for a TuneIn profile.
func TuneInNavigateProfile(encodedURI string) (*models.BmxNavResponse, error) {
	decoded, err := decodeBase64URI(encodedURI)
	if err != nil {
		return nil, err
	}

	data, err := fetchJSON(tuneInRenderJSONURI(decoded))
	if err != nil {
		return nil, err
	}

	navResp := &models.BmxNavResponse{
		Links: &models.Links{
			Self: &models.Link{Href: "/v1/navigate/profile/" + encodedURI},
		},
		Layout: "classic",
	}

	// Profiles contain "pivots" (sections like "Programs", "Related", etc.)
	pivots, _ := data["pivots"].([]interface{})
	for _, p := range pivots {
		pivot, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		pivotName, _ := pivot["text"].(string)
		pivotURL, _ := pivot["URL"].(string)

		// We only care about the "Contents" pivot for now (the main list)
		if !strings.EqualFold(pivotName, "contents") {
			continue
		}

		contents, err := fetchJSON(tuneInRenderJSONURI(pivotURL))
		if err != nil {
			return nil, err
		}

		body, _ := contents["body"].([]interface{})
		for idx, item := range body {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			navResp.BmxSections = append(navResp.BmxSections, tuneInSearchSection(m, idx, "", "list"))
		}
	}

	return navResp, nil
}

func parseTuneInStreamBody(body []byte, guideID string) ([]string, error) {
	// TuneIn sometimes returns plain text with URLs or comments,
	// especially for .ashx or error responses.
	// But our recent refactoring assumed everything is JSON.
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		payload, ok := data["body"].([]interface{})
		if ok && len(payload) > 0 {
			urls := make([]string, 0, len(payload))
			for _, item := range payload {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				if u, ok := m["url"].(string); ok && u != "" {
					urls = append(urls, u)
				}
			}

			if len(urls) > 0 {
				return urls, nil
			}
		}
	}

	// Fallback to plain text parsing (line by line)
	lines := strings.Split(string(body), "\n")

	urls := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		urls = append(urls, line)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no valid stream URLs found for %s", guideID)
	}

	return urls, nil
}

type tuneInProfileContentsResponse struct {
	Items []tuneInProfileContentsSection `json:"Items"`
	Body  []tuneInProfileContentsSection `json:"body"`
}

type tuneInProfileContentsItem struct {
	GuideID string `json:"GuideId"`
	Text    string `json:"text"`
}

type tuneInProfileContentsSection struct {
	Title          string                      `json:"Title"`
	ContainerType  string                      `json:"ContainerType"`
	Children       []tuneInProfileContentsItem `json:"Children"`
	LegacyChildren []tuneInProfileContentsItem `json:"children"`
}

func parseTuneInProgramContents(body []byte, programID string) (episodeID string, err error) {
	var resp tuneInProfileContentsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}

	sections := resp.Items
	if len(sections) == 0 {
		sections = resp.Body
	}

	// Prefer "Episodes" (or "Folgen" etc.) container
	for _, section := range sections {
		if strings.EqualFold(section.ContainerType, "Topics") {
			// If it's explicitly called "Episodes", use it
			title := strings.ToLower(section.Title)
			if strings.Contains(title, "episode") ||
				strings.Contains(title, "folgen") {
				children := section.Children
				if len(children) == 0 {
					children = section.LegacyChildren
				}

				for _, child := range children {
					if strings.HasPrefix(child.GuideID, "t") {
						return child.GuideID, nil
					}
				}
			}
		}
	}

	// Fallback to first Topics container with a 't' child
	for _, section := range sections {
		if strings.EqualFold(section.ContainerType, "Topics") {
			children := section.Children
			if len(children) == 0 {
				children = section.LegacyChildren
			}

			for _, child := range children {
				if strings.HasPrefix(child.GuideID, "t") {
					return child.GuideID, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no episodes found for program %s", programID)
}

func resolveTuneInProgramLatestEpisode(programID string) (episodeID string, err error) {
	// The modern JSON API lists a program's (`p<N>`) episodes; the legacy OPML
	// endpoints can't (`Tune.ashx?id=p<N>` returns `#STATUS: 400`). We use the
	// api.radiotime.com host (tuneInAPIBase) because TuneInNavigateProfile already
	// navigates there. See `_/i226/tunein-api-findings.md` for the endpoint map.
	fetchURL := fmt.Sprintf("%s/profiles/%s/contents?version=1.3", tuneInAPIBase, programID)

	resp, err := defaultClient.Get(fetchURL)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch program contents: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return parseTuneInProgramContents(body, programID)
}

// TuneInDescribeMeta fetches the name and logo for a TuneIn guide ID.
func TuneInDescribeMeta(id string) (name, logo string, err error) {
	fetchURL := fmt.Sprintf("%s/describe.ashx?id=%s", tuneInOpmlDescribeBase, id)

	resp, err := defaultClient.Get(fetchURL)
	if err != nil {
		return "", "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("tunein describe failed with status %d", resp.StatusCode)
	}

	var opml struct {
		Body struct {
			Outline []struct {
				Text  string `xml:"text,attr"`
				Image string `xml:"image,attr"`
			} `xml:"outline"`
		} `xml:"body"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&opml); err != nil {
		return "", "", err
	}

	if len(opml.Body.Outline) > 0 {
		return opml.Body.Outline[0].Text, opml.Body.Outline[0].Image, nil
	}

	return "", "", fmt.Errorf("no metadata found for %s", id)
}

// TuneInPlayback returns a playback response for a TuneIn station.
func TuneInPlayback(stationID, formats string) (*models.BmxPlaybackResponse, error) {
	fetchURL := TuneInStream(stationID, formats)

	resp, err := defaultClient.Get(fetchURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tunein tune failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	urls, err := parseTuneInStreamBody(body, stationID)
	if err != nil {
		return nil, err
	}

	name, logo, _ := TuneInDescribeMeta(stationID)

	return BuildCustomStreamResponseFromURLs(urls, logo, name)
}

// TuneInPodcastInfo returns info for a TuneIn podcast.
func TuneInPodcastInfo(_, encodedName string) (*models.BmxPodcastInfoResponse, error) {
	name, _ := decodeBase64URI(encodedName)

	return &models.BmxPodcastInfoResponse{
		Name:   name,
		Tracks: []models.Track{},
	}, nil
}

// TuneInPlaybackPodcast returns a playback response for a TuneIn podcast.
func TuneInPlaybackPodcast(podcastID, formats string) (*models.BmxPlaybackResponse, error) {
	// Podcasts (p<N>) are just containers for episodes (s<N>).
	// We resolve the latest episode ID first.
	episodeID, err := resolveTuneInProgramLatestEpisode(podcastID)
	if err != nil {
		return nil, err
	}

	return TuneInPlayback(episodeID, formats)
}
