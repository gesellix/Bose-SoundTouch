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
	TuneInStream       = "http://opml.radiotime.com/Tune.ashx?id=%s&formats=mp3,aac,ogg"
	TuneInNavigateAshx = "http://opml.radiotime.com/?render=json"
	TuneInSearchAPI    = "https://api.radiotime.com/profiles?fulltextsearch=true&version=1.3&query="
)

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

	return models.BmxNavItem{
		Links: &models.Links{
			BmxNavigate: &models.Link{Href: fmt.Sprintf("/v1/navigate/profiles/%s/%s/%s", name, guideID, apiURLEncoded)},
			BmxPreset:   &models.Link{ContainerArt: image, Href: fmt.Sprintf("/v1/preset/program/%s", guideID), Name: title, Type: "tracklisturl"},
		},
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

	sections := []models.BmxNavSection{
		{
			Items:  []models.BmxNavItem{{Name: profileTitle, ImageUrl: profileImage, Subtitle: profileSubtitle}},
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

// TuneInPlayback resolves a live radio station and returns a Bose-compatible
// playback response with primary stream and variants.
func TuneInPlayback(stationID string) (*models.BmxPlaybackResponse, error) {
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

	streamReq := fmt.Sprintf(TuneInStream, stationID)

	streamResp, err := http.Get(streamReq)
	if err != nil {
		return nil, err
	}

	defer func() { _ = streamResp.Body.Close() }()

	streamBody, err := io.ReadAll(streamResp.Body)
	if err != nil {
		return nil, err
	}

	streamURLList := strings.Split(strings.TrimSpace(string(streamBody)), "\n")
	if len(streamURLList) == 0 || streamURLList[0] == "" {
		return nil, fmt.Errorf("no streams found")
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
// a playback response suitable for SoundTouch devices.
func TuneInPlaybackPodcast(podcastID string) (*models.BmxPlaybackResponse, error) {
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

	streamReq := fmt.Sprintf(TuneInStream, podcastID)

	streamResp, err := http.Get(streamReq)
	if err != nil {
		return nil, err
	}

	defer func() { _ = streamResp.Body.Close() }()

	streamBody, err := io.ReadAll(streamResp.Body)
	if err != nil {
		return nil, err
	}

	streamURLList := strings.Split(strings.TrimSpace(string(streamBody)), "\n")
	if len(streamURLList) == 0 || streamURLList[0] == "" {
		return nil, fmt.Errorf("no streams found")
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
