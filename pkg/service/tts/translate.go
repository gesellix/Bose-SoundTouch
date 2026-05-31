package tts

import (
	"context"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// ProviderTranslate is the identifier for the Google Translate provider.
const ProviderTranslate = "translate"

// TranslateProvider hands the speaker an undocumented Google Translate TTS URL
// to fetch directly. No credentials, no local hosting; quality and length are
// limited and the endpoint can change without notice.
type TranslateProvider struct{}

// NewTranslateProvider returns a Translate provider.
func NewTranslateProvider() *TranslateProvider {
	return &TranslateProvider{}
}

// Name implements Provider.
func (p *TranslateProvider) Name() string { return ProviderTranslate }

// Synthesize returns a Result whose DirectURL points at the Translate endpoint.
func (p *TranslateProvider) Synthesize(_ context.Context, req Request) (Result, error) {
	language := req.Language
	if language == "" {
		language = "EN"
	}

	return Result{DirectURL: models.BuildTranslateTTSURL(req.Text, language)}, nil
}
