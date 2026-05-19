package handlers

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gesellix/bose-soundtouch/pkg/service/ding"
)

// dingDefaultCache holds the rendered bytes for the default
// option set. Computed once on first request; subsequent default
// requests are served from cache without re-synthesising.
var dingDefaultCache struct {
	once sync.Once
	data []byte
}

// HandleDing serves the AfterTouch "ding" signature audio at
// GET /media/aftertouch-ding.wav. All knobs are optional; the
// default option set is used when no query parameters are
// supplied. Unrecognised parameters and out-of-range values
// (NaN, negatives where positive is required, etc.) silently
// fall back to defaults — this is a "play around with it"
// endpoint, not a strict API.
//
// Supported knobs (all optional; defaults apply per missing param):
//
//	pitch-high                  Hz, float; default 880     (A5, top row)
//	pitch-mid                   Hz, float; default 659.25  (E5, mid row)
//	pitch-low                   Hz, float; default 440     (A4, bottom row)
//	chirp-ms                    int milliseconds; default 250
//	gap-ms                      int milliseconds; default 100
//	attack-ms                   int milliseconds; default 20
//	release-ms                  int milliseconds; default 60
//	sample-rate                 Hz, int; default 22050
//	peak                        0..1 float; default 0.85
//
// The default option set is rendered once via sync.Once and the
// resulting bytes are reused across subsequent default requests —
// first GET pays the synthesis cost (~ms), the rest serve from
// memory. Requests with any override always re-synthesise.
//
// Example:
//
//	curl 'http://localhost:8000/media/aftertouch-ding.wav?pitch-high=1320&chirp-ms=150' > short.wav
//
// See pkg/service/ding for the renderer, scripts/gen-aftertouch-ding
// for the offline-CLI equivalent that takes the same knobs as flags.
func (s *Server) HandleDing(w http.ResponseWriter, r *http.Request) {
	opts, isDefault := parseDingOptions(r)

	var data []byte

	if isDefault {
		dingDefaultCache.once.Do(func() {
			dingDefaultCache.data = ding.Render(ding.DefaultOptions())
		})
		data = dingDefaultCache.data
	} else {
		data = ding.Render(opts)
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(data)
}

// parseDingOptions reads the supported query knobs and returns
// the resulting Options. isDefault is true when no overrides
// were supplied — lets the caller serve from cache.
func parseDingOptions(r *http.Request) (ding.Options, bool) {
	q := r.URL.Query()
	if len(q) == 0 {
		return ding.DefaultOptions(), true
	}

	opts := ding.Options{}
	touched := false

	if v, ok := floatParam(q.Get("pitch-high")); ok {
		opts.PitchHigh = v
		touched = true
	}

	if v, ok := floatParam(q.Get("pitch-mid")); ok {
		opts.PitchMid = v
		touched = true
	}

	if v, ok := floatParam(q.Get("pitch-low")); ok {
		opts.PitchLow = v
		touched = true
	}

	if v, ok := millisecondsParam(q.Get("chirp-ms")); ok {
		opts.ChirpDuration = v
		touched = true
	}

	if v, ok := millisecondsParam(q.Get("gap-ms")); ok {
		opts.GapDuration = v
		touched = true
	}

	if v, ok := millisecondsParam(q.Get("attack-ms")); ok {
		opts.AttackDuration = v
		touched = true
	}

	if v, ok := millisecondsParam(q.Get("release-ms")); ok {
		opts.ReleaseDuration = v
		touched = true
	}

	if v, ok := sampleRateParam(q.Get("sample-rate")); ok {
		opts.SampleRate = v
		touched = true
	}

	if v, ok := floatParam(q.Get("peak")); ok {
		opts.Peak = v
		touched = true
	}

	if !touched {
		return ding.DefaultOptions(), true
	}

	return opts.WithDefaults(), false
}

func floatParam(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}

	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		return 0, false
	}

	return v, true
}

func millisecondsParam(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false
	}

	return float64(v) / 1000.0, true
}

// dingMinSampleRate / dingMaxSampleRate gate the operator-supplied
// sample-rate against int→uint32 truncation and against values
// outside any realistic audio range. The upper bound is generous
// (192 kHz is studio-quality) but well below the uint32 ceiling
// the WAV header field can hold.
const (
	dingMinSampleRate = 8000
	dingMaxSampleRate = 192000
)

func sampleRateParam(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v < dingMinSampleRate || v > dingMaxSampleRate {
		return 0, false
	}

	return v, true
}
