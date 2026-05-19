package ding

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRender_ProducesWAVHeader(t *testing.T) {
	data := Render(DefaultOptions())
	if len(data) < 44 {
		t.Fatalf("expected at least 44 bytes (WAV header), got %d", len(data))
	}

	if !bytes.HasPrefix(data, []byte("RIFF")) {
		t.Errorf("expected RIFF prefix")
	}

	if !bytes.Equal(data[8:12], []byte("WAVE")) {
		t.Errorf("expected WAVE format marker")
	}

	if !bytes.Equal(data[12:16], []byte("fmt ")) {
		t.Errorf("expected fmt chunk")
	}

	// PCM format
	if pcm := binary.LittleEndian.Uint16(data[20:22]); pcm != 1 {
		t.Errorf("expected PCM (1), got %d", pcm)
	}

	// Channels
	if ch := binary.LittleEndian.Uint16(data[22:24]); ch != 2 {
		t.Errorf("expected stereo (2), got %d", ch)
	}

	// Sample rate
	if sr := binary.LittleEndian.Uint32(data[24:28]); sr != 22050 {
		t.Errorf("expected default sample rate 22050, got %d", sr)
	}

	// Bits per sample
	if bps := binary.LittleEndian.Uint16(data[34:36]); bps != 16 {
		t.Errorf("expected 16 bits/sample, got %d", bps)
	}
}

func TestRender_DefaultSizeApproximately52KB(t *testing.T) {
	data := Render(DefaultOptions())

	// Default: 22050 Hz * 2 channels * 2 bytes * 0.6 s = 52920 data
	// + ~44 byte header.
	const wantData = 22050 * 2 * 2 * 60 / 100 // 0.6 seconds, integer math
	if got := len(data); got < wantData || got > wantData+200 {
		t.Errorf("expected ~%d bytes, got %d", wantData, got)
	}
}

func TestRender_OverridePitchAffectsContent(t *testing.T) {
	a := Render(DefaultOptions())
	b := Render(Options{PitchHigh: 1200}.WithDefaults())

	if bytes.Equal(a, b) {
		t.Errorf("expected different content for different PitchHigh values")
	}

	// Same length (envelope doesn't change).
	if len(a) != len(b) {
		t.Errorf("expected same byte length: %d vs %d", len(a), len(b))
	}
}

func TestRender_OverrideSampleRateChangesByteRate(t *testing.T) {
	data := Render(Options{SampleRate: 44100}.WithDefaults())
	if sr := binary.LittleEndian.Uint32(data[24:28]); sr != 44100 {
		t.Errorf("expected 44100, got %d", sr)
	}
}

func TestSafeSampleRate_ClampsOutOfRange(t *testing.T) {
	cases := []struct {
		in   int
		want uint32
	}{
		{22050, 22050},
		{44100, 44100},
		{192000, 192000},
		{0, 22050},       // zero → default
		{-1, 22050},      // negative → default
		{200000, 22050},  // above max → default
		{1 << 31, 22050}, // way beyond uint32 → default (the original CodeQL concern)
		{1 << 33, 22050}, // wraps to a different value on int→uint32; default protects us
		{int(maxSampleRate) + 1, 22050},
	}

	for _, c := range cases {
		if got := safeSampleRate(c.in); got != c.want {
			t.Errorf("safeSampleRate(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestRender_HugeSampleRateDoesNotTruncateOrPanic(t *testing.T) {
	// Regression for the int→uint32 truncation CodeQL flagged.
	// A caller (test, future SDK user) bypassing the handler's
	// sampleRateParam guard with an int well above uint32 used
	// to silently wrap. The defensive clamp now substitutes the
	// default sample rate instead.
	data := Render(Options{SampleRate: 1 << 33}.WithDefaults())
	if sr := binary.LittleEndian.Uint32(data[24:28]); sr != uint32(DefaultOptions().SampleRate) {
		t.Errorf("expected clamp to default sample rate, got %d", sr)
	}
}

func TestWithDefaults_FillsZeroFields(t *testing.T) {
	got := Options{PitchHigh: 1000}.WithDefaults()
	if got.PitchHigh != 1000 {
		t.Errorf("override should be preserved, got %f", got.PitchHigh)
	}

	if got.SampleRate != 22050 {
		t.Errorf("expected default SampleRate, got %d", got.SampleRate)
	}

	if got.PitchMid <= 0 {
		t.Errorf("expected PitchMid filled from default, got %f", got.PitchMid)
	}
}

func TestWithDefaults_ClampsInvalidPeak(t *testing.T) {
	got := Options{Peak: 2.0}.WithDefaults()
	if got.Peak != DefaultOptions().Peak {
		t.Errorf("expected default Peak for invalid input, got %f", got.Peak)
	}

	got = Options{Peak: -0.5}.WithDefaults()
	if got.Peak != DefaultOptions().Peak {
		t.Errorf("expected default Peak for negative input, got %f", got.Peak)
	}
}

func TestRender_SamplesDontClip(t *testing.T) {
	data := Render(DefaultOptions())

	// Walk every sample, ensure no value is at the 16-bit extreme
	// (which would indicate clipping). Header is 44 bytes.
	for i := 44; i+1 < len(data); i += 2 {
		s := int16(binary.LittleEndian.Uint16(data[i : i+2]))
		if s == 32767 || s == -32768 {
			t.Errorf("sample at offset %d hit clipping (%d)", i, s)
			return
		}
	}
}
