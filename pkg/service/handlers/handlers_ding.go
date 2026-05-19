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

// HandleDing serves the AfterTouch "ding" signature audio.
// Defaults are used when no query parameters are supplied;
// callers can override any of the rendering knobs:
//
//	pitch-high, pitch-mid, pitch-low (Hz, float)
//	chirp-ms, gap-ms, attack-ms, release-ms (milliseconds, int)
//	sample-rate (Hz, int)
//	peak (0..1, float)
//
// Unrecognised parameters and out-of-range values fall back to
// defaults silently — this is a "play around with it" endpoint,
// not a strict API.
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

	if v, ok := intParam(q.Get("sample-rate")); ok {
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

func intParam(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false
	}

	return v, true
}
