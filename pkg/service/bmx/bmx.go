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

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TuneIn endpoint templates used to resolve station and stream URLs.
const (
	TuneInDescribe = "https://opml.radiotime.com/describe.ashx?id=%s"
	TuneInStream   = "http://opml.radiotime.com/Tune.ashx?id=%s&formats=mp3,aac,ogg"
)

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

	streamList := []models.Stream{
		{
			HasPlaylist: true,
			IsRealtime:  true,
			StreamUrl:   jsonObj.StreamURL,
		},
	}

	audio := models.Audio{
		HasPlaylist: true,
		IsRealtime:  true,
		StreamUrl:   jsonObj.StreamURL,
		Streams:     streamList,
	}

	response := &models.BmxPlaybackResponse{
		Audio:      audio,
		ImageUrl:   jsonObj.ImageURL,
		Name:       jsonObj.Name,
		StreamType: "liveRadio",
	}

	return response, nil
}
