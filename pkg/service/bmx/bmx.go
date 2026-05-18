// Package bmx implements minimal helper calls to public music service endpoints
// like TuneIn and RadioBrowser and wraps them into Bose-compatible
// response models.
package bmx

import (
	"encoding/json"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

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
