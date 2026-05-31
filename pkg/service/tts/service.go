package tts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Config configures a Service.
type Config struct {
	// BaseURL is the service's public URL the speaker can reach (e.g.
	// "https://soundtouch.local"). Used to build /media/tts/{id} URLs for
	// synthesized clips. Not needed for direct-URL providers.
	BaseURL string
	// AppKey is the Bose /speaker app_key. The Service does not use it itself;
	// it is surfaced to the handler that POSTs play_info to the speaker.
	AppKey string
	// DefaultLanguage / DefaultVoice / DefaultVolume fill in unset Request
	// fields. Defaults must match the active provider's expectations.
	DefaultLanguage string
	DefaultVoice    string
	DefaultVolume   int
	// CacheTTL and CacheMaxEntries bound the synthesized-clip cache.
	CacheTTL        time.Duration
	CacheMaxEntries int
}

// Service orchestrates a Provider plus a clip cache to turn text into a single
// speaker-playable URL. It is provider-agnostic: callers use Prepare and never
// see the direct-URL vs. byte-synthesis distinction.
type Service struct {
	provider Provider
	cache    *clipCache
	cfg      Config
}

// NewService builds a Service around the given provider and config.
func NewService(provider Provider, cfg Config) *Service {
	return &Service{
		provider: provider,
		cache:    newClipCache(cfg.CacheTTL, cfg.CacheMaxEntries),
		cfg:      cfg,
	}
}

// ProviderName returns the active provider's identifier.
func (s *Service) ProviderName() string { return s.provider.Name() }

// AppKey returns the configured Bose /speaker app_key.
func (s *Service) AppKey() string { return s.cfg.AppKey }

// DefaultVolume returns the configured default playback volume (0 = current).
func (s *Service) DefaultVolume() int { return s.cfg.DefaultVolume }

// DefaultLanguage returns the configured default language.
func (s *Service) DefaultLanguage() string { return s.cfg.DefaultLanguage }

// DefaultVoice returns the configured default voice.
func (s *Service) DefaultVoice() string { return s.cfg.DefaultVoice }

// Prepare produces a speaker-playable URL for req. For direct-URL providers it
// returns the provider's URL; for synthesizing providers it caches the audio
// and returns a local /media/tts/{id} URL. Repeated identical requests reuse
// the cached clip.
func (s *Service) Prepare(ctx context.Context, req Request) (string, error) {
	req = s.applyDefaults(req)

	if strings.TrimSpace(req.Text) == "" {
		return "", fmt.Errorf("tts: text is empty")
	}

	id := clipID(req)
	if _, _, ok := s.cache.get(id); ok {
		return s.mediaURL(id), nil
	}

	res, err := s.provider.Synthesize(ctx, req)
	if err != nil {
		return "", err
	}

	if res.DirectURL != "" {
		return res.DirectURL, nil
	}

	if len(res.Audio) == 0 {
		return "", fmt.Errorf("tts: provider returned no audio")
	}

	if strings.TrimSpace(s.cfg.BaseURL) == "" {
		return "", fmt.Errorf("tts: provider returned audio but no base URL is configured to host it")
	}

	s.cache.put(id, res.Audio, res.ContentType)

	return s.mediaURL(id), nil
}

// Clip returns the cached audio and content type for a media id, if present.
// Used by the /media/tts/{id} handler.
func (s *Service) Clip(id string) (audio []byte, contentType string, ok bool) {
	return s.cache.get(id)
}

// applyDefaults fills unset Request fields from config.
func (s *Service) applyDefaults(req Request) Request {
	if req.Language == "" {
		req.Language = s.cfg.DefaultLanguage
	}

	if req.Voice == "" {
		req.Voice = s.cfg.DefaultVoice
	}

	if req.Format == "" {
		req.Format = FormatMP3
	}

	return req
}

// mediaURL builds the local URL the speaker fetches a cached clip from.
func (s *Service) mediaURL(id string) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/media/tts/" + id
}

// clipID is a deterministic media id (hash + extension) so identical requests
// map to the same cached clip and URL.
func clipID(req Request) string {
	sum := sha256.Sum256([]byte(req.Text + "|" + req.Voice + "|" + req.Language + "|" + req.Format))

	ext := "mp3"
	if req.Format == FormatWAV {
		ext = "wav"
	}

	return hex.EncodeToString(sum[:16]) + "." + ext
}
