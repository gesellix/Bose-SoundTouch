// Generator for the AfterTouch "ding" sound — a two-chirp signature
// derived from the braille letters S and T (which the AfterTouch
// logo overlays).
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
//	Columns map to stereo channels:
//	   left column (1,2,3)  → left channel
//	   right column (4,5,6) → right channel
//
//	Rows map to pitch:
//	   top row (1,4)    → A5  (880 Hz)
//	   mid row (2,5)    → E5  (659.25 Hz)
//	   bottom row (3,6) → A4  (440 Hz)
//
// So:
//
//	S (dots 2,3,4): L = E5+A4, R = A5
//	T (dots 2,3,4,5): L = E5+A4, R = A5+E5  (S with an extra voice on the right)
//
// Total clip ≈ 600 ms: chirp(S) ~250 ms, gap ~100 ms, chirp(T) ~250 ms.
// Each chirp has a short attack and decay envelope to avoid clicks.
//
// Run:
//
//	go run ./scripts/gen-aftertouch-ding > pkg/service/handlers/static/media/aftertouch-ding.wav
//
// Or pass -o to write directly:
//
//	go run ./scripts/gen-aftertouch-ding -o pkg/service/handlers/static/media/aftertouch-ding.wav
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	sampleRate = 22050
	channels   = 2
	bitsPer    = 16
)

// Pitches (Hz).
const (
	pitchHigh = 880.00    // A5  (top row)
	pitchMid  = 659.2551  // E5  (mid row)
	pitchLow  = 440.00    // A4  (bottom row)
)

// A "voice" is a single sine tone routed to one stereo channel.
type voice struct {
	freq    float64
	channel int // 0 = left, 1 = right
}

// Active voices per braille letter, derived from the dot mapping above.
var (
	voicesS = []voice{
		{freq: pitchMid, channel: 0}, // dot 2: left-mid
		{freq: pitchLow, channel: 0}, // dot 3: left-bottom
		{freq: pitchHigh, channel: 1}, // dot 4: right-top
	}
	voicesT = []voice{
		{freq: pitchMid, channel: 0}, // dot 2: left-mid
		{freq: pitchLow, channel: 0}, // dot 3: left-bottom
		{freq: pitchHigh, channel: 1}, // dot 4: right-top
		{freq: pitchMid, channel: 1}, // dot 5: right-mid
	}
)

func main() {
	var outPath string
	flag.StringVar(&outPath, "o", "", "output WAV path; default stdout")
	flag.Parse()

	var (
		chirpDur = 0.25 // seconds
		gapDur   = 0.10
		attack   = 0.020 // fade-in, avoids click
		release  = 0.060 // fade-out, avoids tail click
	)

	chirpN := int(math.Round(float64(sampleRate) * chirpDur))
	gapN := int(math.Round(float64(sampleRate) * gapDur))

	// Allocate exactly: two chirps + one gap. Doing this from the
	// rendered sample counts (instead of re-computing from seconds)
	// avoids a rounding off-by-one between the two paths.
	samplesPerChannel := chirpN*2 + gapN
	left := make([]float64, samplesPerChannel)
	right := make([]float64, samplesPerChannel)

	renderChirp(left, right, 0, chirpN, voicesS, attack, release)
	renderChirp(left, right, chirpN+gapN, chirpN, voicesT, attack, release)

	normalise(left, right, 0.85) // headroom below 1.0 to avoid clipping

	var buf bytes.Buffer
	if err := writeWAV(&buf, left, right); err != nil {
		fail("encode: %v", err)
	}

	var w io.Writer = os.Stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			fail("create %s: %v", outPath, err)
		}
		defer f.Close()
		w = f
	}

	if _, err := w.Write(buf.Bytes()); err != nil {
		fail("write: %v", err)
	}
}

// renderChirp writes one chirp into the L/R buffers starting at offset.
// The envelope is a trapezoid: linear attack, flat sustain, linear release.
func renderChirp(left, right []float64, offset, length int, voices []voice, attackSec, releaseSec float64) {
	attackN := int(math.Round(float64(sampleRate) * attackSec))
	releaseN := int(math.Round(float64(sampleRate) * releaseSec))

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
			sample := math.Sin(2 * math.Pi * v.freq * t) * env
			if v.channel == 0 {
				left[offset+i] += sample
			} else {
				right[offset+i] += sample
			}
		}
	}
}

// normalise scales L/R so the peak absolute value equals `peak` (≤ 1.0).
// This keeps the chord-sum from clipping without hardcoding voice counts.
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

func writeWAV(w io.Writer, left, right []float64) error {
	if len(left) != len(right) {
		return fmt.Errorf("channel length mismatch: %d vs %d", len(left), len(right))
	}

	samples := len(left)
	dataBytes := samples * channels * (bitsPer / 8)
	totalRIFFSize := 4 + (8 + 16) + (8 + dataBytes) // "WAVE" + fmt chunk + data chunk

	// RIFF header
	if _, err := w.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(totalRIFFSize)); err != nil {
		return err
	}
	if _, err := w.Write([]byte("WAVE")); err != nil {
		return err
	}

	// fmt chunk
	if _, err := w.Write([]byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(16)); err != nil { // PCM fmt chunk size
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(1)); err != nil { // PCM
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(channels)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	byteRate := uint32(sampleRate * channels * (bitsPer / 8))
	if err := binary.Write(w, binary.LittleEndian, byteRate); err != nil {
		return err
	}
	blockAlign := uint16(channels * (bitsPer / 8))
	if err := binary.Write(w, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(bitsPer)); err != nil {
		return err
	}

	// data chunk
	if _, err := w.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(dataBytes)); err != nil {
		return err
	}

	for i := 0; i < samples; i++ {
		l := floatToInt16(left[i])
		r := floatToInt16(right[i])
		if err := binary.Write(w, binary.LittleEndian, l); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, r); err != nil {
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

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen-aftertouch-ding: "+format+"\n", args...)
	os.Exit(1)
}
