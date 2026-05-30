// Package bmx implements minimal helper calls to public music service endpoints
// like TuneIn and RadioBrowser and wraps them into Bose-compatible
// response models.
package bmx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// BuildOrionLocation wraps a raw stream URL in the AfterTouch Orion station
// endpoint that the speaker's BMX module expects when playing LOCAL_INTERNET_RADIO
// content. The speaker calls GET on the stored location expecting a
// BmxPlaybackResponse JSON — not raw audio bytes.
func BuildOrionLocation(serviceURL, name, imageURL, streamURL string) string {
	payload := struct {
		Name      string `json:"name"`
		ImageURL  string `json:"imageUrl"`
		StreamURL string `json:"streamUrl"`
	}{
		Name:      name,
		ImageURL:  imageURL,
		StreamURL: streamURL,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	encoded := url.QueryEscape(base64.StdEncoding.EncodeToString(data))

	return serviceURL + "/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + encoded
}

// BuildCustomStreamResponseFromURLs wraps one or more candidate stream URLs
// in a playback response. The speaker fails over between entries in the
// Streams array, so order matters — pass them as the provider listed them.
// The top-level StreamUrl mirrors urls[0] for compatibility.
func BuildCustomStreamResponseFromURLs(urls []string, imageURL, name string) (*models.BmxPlaybackResponse, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no stream URLs provided")
	}

	streamList := make([]models.Stream, 0, len(urls))
	for _, u := range urls {
		streamList = append(streamList, models.Stream{
			HasPlaylist: true,
			IsRealtime:  true,
			StreamUrl:   u,
		})
	}

	audio := models.Audio{
		HasPlaylist: true,
		IsRealtime:  true,
		StreamUrl:   urls[0],
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

// BuildCustomStreamResponse builds a playback response from streamUrl, imageUrl, and name.
func BuildCustomStreamResponse(streamURL, imageURL, name string) (*models.BmxPlaybackResponse, error) {
	return BuildCustomStreamResponseFromURLs([]string{streamURL}, imageURL, name)
}

// PlayCustomStream builds a playback response from a base64-encoded JSON blob
// with fields streamUrl, imageUrl, and name.
func PlayCustomStream(data string) (*models.BmxPlaybackResponse, error) {
	jsonStr, err := decodeBase64URI(data)
	if err != nil {
		return nil, err
	}

	var jsonObj struct {
		StreamURL string `json:"streamUrl"`
		ImageURL  string `json:"imageUrl"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &jsonObj); err != nil {
		return nil, err
	}

	return BuildCustomStreamResponse(jsonObj.StreamURL, jsonObj.ImageURL, jsonObj.Name)
}
