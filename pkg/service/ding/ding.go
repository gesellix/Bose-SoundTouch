// Package ding renders the AfterTouch "ding" signature sound — a
// two-chirp tone derived from the braille letters S and T that
// make up the AfterTouch logo. Used as the Health-tab test
// playback target: pushed to a speaker as a custom-radio
// ContentItem so operators can confirm a freshly migrated speaker
// emits audio without depending on TuneIn or any external service.
//
// Mapping:
//
//	Braille S = ⠎ = dots 2, 3, 4
//	Braille T = ⠞ = dots 2, 3, 4, 5
//
//	Dot positions in the 6-dot grid:
//	   1   4
//	   2   5
//	   3   6
//
//	Columns → stereo channels (left=1,2,3 / right=4,5,6).
//	Rows    → pitches: top=PitchHigh, mid=PitchMid, bottom=PitchLow.
//
// So S (dots 2,3,4) renders as L=PitchMid+PitchLow, R=PitchHigh,
// and T (dots 2,3,4,5) adds R=PitchMid on top of S.
//
// Render(opts) returns a self-contained 16-bit stereo PCM WAV.
// Default options produce a ~600 ms / 52 KB clip; callers can
// override any subset and let the rest fall back to defaults
// (see DefaultOptions).
package ding

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Options controls the synthesis. A zero-valued Options struct
// is *not* usable directly; the WithDefaults method fills in
// sensible numbers for unset fields so callers can supply only
// the parameters they want to override.
type Options struct {
	SampleRate int // Hz. Default 22050.

	PitchHigh float64 // Hz, top row (A5=880).
	PitchMid  float64 // Hz, middle row (E5=659.2551).
	PitchLow  float64 // Hz, bottom row (A4=440).

	ChirpDuration   float64 // seconds per chirp. Default 0.25.
	GapDuration     float64 // seconds between chirps. Default 0.10.
	AttackDuration  float64 // seconds of fade-in per chirp. Default 0.020.
	ReleaseDuration float64 // seconds of fade-out per chirp. Default 0.060.

	Peak float64 // final-mix headroom; 0 < Peak <= 1.0. Default 0.85.
}

// DefaultOptions returns the canonical option set used by the
// runtime handler when no overrides are supplied.
func DefaultOptions() Options {
	return Options{
		SampleRate:      22050,
		PitchHigh:       880.00,
		PitchMid:        659.2551,
		PitchLow:        440.00,
		ChirpDuration:   0.25,
		GapDuration:     0.10,
		AttackDuration:  0.020,
		ReleaseDuration: 0.060,
		Peak:            0.85,
	}
}

// WithDefaults returns a copy of o with any zero-valued fields
// filled in from DefaultOptions. Lets callers write
//
//	ding.Options{PitchHigh: 1000}.WithDefaults()
//
// instead of restating every field.
func (o Options) WithDefaults() Options {
	d := DefaultOptions()

	if o.SampleRate <= 0 || o.SampleRate > int(maxSampleRate) {
		o.SampleRate = d.SampleRate
	}

	if o.PitchHigh <= 0 {
		o.PitchHigh = d.PitchHigh
	}

	if o.PitchMid <= 0 {
		o.PitchMid = d.PitchMid
	}

	if o.PitchLow <= 0 {
		o.PitchLow = d.PitchLow
	}

	if o.ChirpDuration <= 0 {
		o.ChirpDuration = d.ChirpDuration
	}

	if o.GapDuration <= 0 {
		o.GapDuration = d.GapDuration
	}

	if o.AttackDuration <= 0 {
		o.AttackDuration = d.AttackDuration
	}

	if o.ReleaseDuration <= 0 {
		o.ReleaseDuration = d.ReleaseDuration
	}

	if o.Peak <= 0 || o.Peak > 1.0 {
		o.Peak = d.Peak
	}

	return o
}

// Render synthesises the ding using opts (after defaulting) and
// returns a self-contained 16-bit PCM WAV file.
func Render(opts Options) []byte {
	opts = opts.WithDefaults()

	voicesS := []voice{
		{freq: opts.PitchMid, channel: 0},
		{freq: opts.PitchLow, channel: 0},
		{freq: opts.PitchHigh, channel: 1},
	}
	voicesT := []voice{
		{freq: opts.PitchMid, channel: 0},
		{freq: opts.PitchLow, channel: 0},
		{freq: opts.PitchHigh, channel: 1},
		{freq: opts.PitchMid, channel: 1},
	}

	chirpN := int(math.Round(float64(opts.SampleRate) * opts.ChirpDuration))
	gapN := int(math.Round(float64(opts.SampleRate) * opts.GapDuration))
	attackN := int(math.Round(float64(opts.SampleRate) * opts.AttackDuration))
	releaseN := int(math.Round(float64(opts.SampleRate) * opts.ReleaseDuration))

	samplesPerChannel := chirpN*2 + gapN
	left := make([]float64, samplesPerChannel)
	right := make([]float64, samplesPerChannel)

	renderChirp(left, right, 0, chirpN, attackN, releaseN, voicesS, opts.SampleRate)
	renderChirp(left, right, chirpN+gapN, chirpN, attackN, releaseN, voicesT, opts.SampleRate)

	normalise(left, right, opts.Peak)

	var buf bytes.Buffer

	// Defensive bound check: clamp before the conversion to
	// uint32 so even a buggy caller (or one that bypassed the
	// handler-side bound check on the query param) can't trigger
	// integer truncation in the WAV header fields.
	sampleRate32 := safeSampleRate(opts.SampleRate)

	_ = writeWAV(&buf, left, right, sampleRate32)

	return buf.Bytes()
}

type voice struct {
	freq    float64
	channel int
}

// renderChirp synthesises one chirp into the L/R buffers
// starting at offset, with a trapezoidal attack/sustain/release
// envelope.
func renderChirp(left, right []float64, offset, length, attackN, releaseN int, voices []voice, sampleRate int) {
	if attackN+releaseN > length {
		attackN = length / 3
		releaseN = length / 3
	}

	for i := 0; i < length; i++ {
		t := float64(i) / float64(sampleRate)
		env := 1.0

		switch {
		case i < attackN:
			env = float64(i) / float64(attackN)
		case i >= length-releaseN:
			remaining := length - i
			env = float64(remaining) / float64(releaseN)
		}

		for _, v := range voices {
			sample := math.Sin(2*math.Pi*v.freq*t) * env
			if v.channel == 0 {
				left[offset+i] += sample
			} else {
				right[offset+i] += sample
			}
		}
	}
}

// normalise scales L/R so the peak absolute value equals `peak`
// (≤ 1.0). Keeps the chord sum below clipping without hardcoding
// voice counts.
func normalise(left, right []float64, peak float64) {
	maxVal := 0.0

	for i := range left {
		if v := math.Abs(left[i]); v > maxVal {
			maxVal = v
		}

		if v := math.Abs(right[i]); v > maxVal {
			maxVal = v
		}
	}

	if maxVal == 0 {
		return
	}

	scale := peak / maxVal
	for i := range left {
		left[i] *= scale
		right[i] *= scale
	}
}

const (
	wavChannels = 2
	wavBitsPer  = 16
)

// maxSampleRate is the largest sample rate writeWAV will accept
// before clamping. Generous enough to allow studio-quality 192
// kHz; well below the uint32 ceiling the WAV header field can
// represent, and far below anything the byte-rate multiplication
// downstream could overflow.
const maxSampleRate uint32 = 192_000

// safeSampleRate converts the operator-supplied int sample rate
// into the uint32 the WAV header needs, clamping anything
// out-of-range to the default. Defence-in-depth: the
// handler-side sampleRateParam already rejects unreasonable
// inputs, but Render is exported so other callers (tests,
// scripts) could pass anything.
func safeSampleRate(in int) uint32 {
	if in <= 0 || in > int(maxSampleRate) {
		return uint32(DefaultOptions().SampleRate)
	}

	return uint32(in)
}

func writeWAV(w io.Writer, left, right []float64, sampleRate uint32) error {
	if len(left) != len(right) {
		return fmt.Errorf("channel length mismatch: %d vs %d", len(left), len(right))
	}

	samples := len(left)
	dataBytes := samples * wavChannels * (wavBitsPer / 8)
	totalRIFFSize := 4 + (8 + 16) + (8 + dataBytes)

	if _, err := w.Write([]byte("RIFF")); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(totalRIFFSize)); err != nil {
		return err
	}

	if _, err := w.Write([]byte("WAVE")); err != nil {
		return err
	}

	if _, err := w.Write([]byte("fmt ")); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint16(1)); err != nil { // PCM
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint16(wavChannels)); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, sampleRate); err != nil {
		return err
	}

	byteRate := sampleRate * uint32(wavChannels) * uint32(wavBitsPer/8)
	if err := binary.Write(w, binary.LittleEndian, byteRate); err != nil {
		return err
	}

	blockAlign := uint16(wavChannels * (wavBitsPer / 8))
	if err := binary.Write(w, binary.LittleEndian, blockAlign); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint16(wavBitsPer)); err != nil {
		return err
	}

	if _, err := w.Write([]byte("data")); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(dataBytes)); err != nil {
		return err
	}

	for i := 0; i < samples; i++ {
		if err := binary.Write(w, binary.LittleEndian, floatToInt16(left[i])); err != nil {
			return err
		}

		if err := binary.Write(w, binary.LittleEndian, floatToInt16(right[i])); err != nil {
			return err
		}
	}

	return nil
}

func floatToInt16(v float64) int16 {
	if v > 1.0 {
		v = 1.0
	} else if v < -1.0 {
		v = -1.0
	}

	return int16(math.Round(v * 32767))
}
