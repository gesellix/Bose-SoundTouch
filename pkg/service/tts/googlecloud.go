package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ProviderGoogleCloud is the identifier for the Google Cloud TTS provider.
const ProviderGoogleCloud = "google-cloud"

// googleCloudSynthesizeURL is the REST synthesize endpoint. Authentication is a
// plain API key passed as the ?key= query parameter (no OAuth, no SDK).
const googleCloudSynthesizeURL = "https://texttospeech.googleapis.com/v1/text:synthesize"

// CloudProvider synthesizes speech via the Google Cloud Text-to-Speech REST API
// using an API key. It returns audio bytes for the Service to host locally.
type CloudProvider struct {
	apiKey     string
	endpoint   string // overridable for tests
	httpClient *http.Client
}

// NewCloudProvider returns a Google Cloud TTS provider using the given API key.
func NewCloudProvider(apiKey string) *CloudProvider {
	return &CloudProvider{
		apiKey:     apiKey,
		endpoint:   googleCloudSynthesizeURL,
		httpClient: http.DefaultClient,
	}
}

// SetEndpoint overrides the synthesize endpoint (for testing against a mock).
func (p *CloudProvider) SetEndpoint(url string) { p.endpoint = url }

// Name implements Provider.
func (p *CloudProvider) Name() string { return ProviderGoogleCloud }

// cloudSynthesizeRequest mirrors the Cloud TTS v1 synthesize request body.
type cloudSynthesizeRequest struct {
	Input struct {
		Text string `json:"text"`
	} `json:"input"`
	Voice struct {
		LanguageCode string `json:"languageCode"`
		Name         string `json:"name,omitempty"`
	} `json:"voice"`
	AudioConfig struct {
		AudioEncoding string `json:"audioEncoding"`
	} `json:"audioConfig"`
}

// cloudSynthesizeResponse mirrors the Cloud TTS v1 synthesize response body.
// audioContent is base64-encoded audio in the requested encoding.
type cloudSynthesizeResponse struct {
	AudioContent string `json:"audioContent"`
}

// Synthesize calls the Cloud TTS REST API and returns the decoded audio bytes.
func (p *CloudProvider) Synthesize(ctx context.Context, req Request) (Result, error) {
	if p.apiKey == "" {
		return Result{}, fmt.Errorf("google cloud tts: no API key configured")
	}

	encoding, contentType := "MP3", "audio/mpeg"
	if req.Format == FormatWAV {
		// LINEAR16 is returned wrapped in a WAV container.
		encoding, contentType = "LINEAR16", "audio/wav"
	}

	language := req.Language
	if language == "" {
		language = "en-US"
	}

	var body cloudSynthesizeRequest

	body.Input.Text = req.Text
	body.Voice.LanguageCode = language
	body.Voice.Name = req.Voice
	body.AudioConfig.AudioEncoding = encoding

	payload, err := json.Marshal(&body)
	if err != nil {
		return Result{}, fmt.Errorf("google cloud tts: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"?key="+p.apiKey, bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("google cloud tts: build request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("google cloud tts: request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("google cloud tts: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("google cloud tts: synthesize failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var parsed cloudSynthesizeResponse
	if err = json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("google cloud tts: parse response: %w", err)
	}

	audio, err := base64.StdEncoding.DecodeString(parsed.AudioContent)
	if err != nil {
		return Result{}, fmt.Errorf("google cloud tts: decode audio: %w", err)
	}

	if len(audio) == 0 {
		return Result{}, fmt.Errorf("google cloud tts: empty audio content")
	}

	return Result{Audio: audio, ContentType: contentType}, nil
}
