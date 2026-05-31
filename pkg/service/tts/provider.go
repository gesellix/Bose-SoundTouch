// Package tts turns text into speaker-playable audio for the local service.
//
// SoundTouch speakers play a notification by fetching a URL themselves (via the
// /speaker endpoint). Two delivery shapes exist behind a single Provider
// interface:
//
//   - Direct-URL providers (e.g. Google Translate) hand the speaker a URL it
//     fetches directly. No local hosting needed; Result.DirectURL is set.
//   - Synthesizing providers (e.g. Google Cloud TTS) return audio *bytes*. The
//     Service caches them and serves them from a local /media/tts/{id} URL that
//     the speaker can reach. Result.Audio is set.
//
// The Service (service.go) hides this distinction: callers ask it to Prepare a
// Request and get back a single playable URL.
package tts

import "context"

// Audio format identifiers for a Request.
const (
	FormatMP3 = "mp3"
	FormatWAV = "wav"
)

// Request describes one synthesis. Language and Voice are provider-specific:
// the Translate provider expects a short code like "EN"; Google Cloud expects a
// BCP-47 tag like "en-US" plus an optional voice name. The active provider is
// fixed per deployment, so the configured defaults are matched to it.
type Request struct {
	Text     string
	Language string
	Voice    string
	Format   string // FormatMP3 (default) or FormatWAV
}

// Result is what a Provider returns. Exactly one of DirectURL or Audio is set.
type Result struct {
	Audio       []byte // synthesized bytes; nil for direct-URL providers
	ContentType string // e.g. "audio/mpeg"; set alongside Audio
	DirectURL   string // speaker-fetchable URL; set instead of Audio
}

// Provider converts text to either a direct URL or audio bytes.
type Provider interface {
	// Name returns the provider identifier (e.g. "translate", "google-cloud").
	Name() string
	// Synthesize converts req into a Result.
	Synthesize(ctx context.Context, req Request) (Result, error)
}
