package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// wavBytes builds a minimal RIFF/WAVE file with the given parameters and
// dataBytes of (silent) payload.
func wavBytes(sampleRate, channels, bitsPerSample, dataBytes int) []byte {
	byteRate := sampleRate * channels * bitsPerSample / 8

	var b bytes.Buffer

	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+dataBytes))
	b.WriteString("WAVE")

	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels*bitsPerSample/8))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bitsPerSample))

	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(dataBytes))
	b.Write(make([]byte, dataBytes))

	return b.Bytes()
}

// mp3Bytes builds bytes that open with one MPEG 1 Layer III frame header
// (128 kbit/s, 44100 Hz, stereo) followed by total-4 bytes of filler.
func mp3Bytes(total int) []byte {
	b := make([]byte, total)
	copy(b, []byte{0xff, 0xfb, 0x90, 0x00})

	return b
}

func TestProbeWAVMeasuresDuration(t *testing.T) {
	// 44100 Hz, stereo, 16-bit: one second is 176400 bytes.
	params := probeAudio("audio/x-wav", wavBytes(44100, 2, 16, 176400*3))

	if params.SampleRate != 44100 || params.Channels != 2 {
		t.Errorf("sampleRate=%d channels=%d, want 44100 and 2", params.SampleRate, params.Channels)
	}

	if params.Bitrate != 44100*2*16 {
		t.Errorf("bitrate = %d, want %d", params.Bitrate, 44100*2*16)
	}

	if math.Abs(params.DurSec-3) > 0.001 {
		t.Errorf("duration = %v, want 3", params.DurSec)
	}
}

func TestProbeMP3ReadsTheFrameHeader(t *testing.T) {
	// At 128 kbit/s, 32000 bytes is exactly two seconds.
	params := probeAudio("audio/mpeg", mp3Bytes(32000))

	if params.Bitrate != 128000 || params.SampleRate != 44100 || params.Channels != 2 {
		t.Errorf("bitrate=%d sampleRate=%d channels=%d, want 128000, 44100 and 2",
			params.Bitrate, params.SampleRate, params.Channels)
	}

	if math.Abs(params.DurSec-2) > 0.01 {
		t.Errorf("duration = %v, want 2", params.DurSec)
	}
}

// A VBR file's byte count says nothing about its length, so the Xing frame
// count is what has to be believed when one is present.
func TestProbeMP3PrefersTheXingFrameCount(t *testing.T) {
	const frames = 100

	b := mp3Bytes(32000)

	// Side information for MPEG 1 stereo is 32 bytes, so the tag starts there.
	pos := 4 + 32
	copy(b[pos:], []byte("Xing"))
	binary.BigEndian.PutUint32(b[pos+4:], 0x0001) // frame count present
	binary.BigEndian.PutUint32(b[pos+8:], frames)

	params := probeAudio("audio/mpeg", b)

	want := float64(frames) * 1152 / 44100
	if math.Abs(params.DurSec-want) > 0.001 {
		t.Errorf("duration = %v, want %v from the Xing frame count", params.DurSec, want)
	}
}

func TestProbeMP3SkipsAnID3Tag(t *testing.T) {
	const tagBody = 40

	tag := make([]byte, 10+tagBody)
	copy(tag, []byte("ID3"))
	tag[3] = 3               // version 2.3
	tag[9] = byte(tagBody)   // syncsafe size, small enough to fit one byte
	audio := mp3Bytes(32000) //nolint:mnd // exactly two seconds at 128 kbit/s
	b := append(tag, audio...)

	params := probeAudio("audio/mpeg", b)

	if params.Bitrate != 128000 {
		t.Errorf("bitrate = %d, want the frame past the ID3 tag to be read", params.Bitrate)
	}
}

// Anything we cannot parse must report nothing rather than guess, so the DIDL
// falls back to its placeholders.
func TestProbeUnknownFormatsReportNothing(t *testing.T) {
	for _, mime := range []string{"audio/flac", "audio/mp4", "audio/ogg", ""} {
		if got := probeAudio(mime, []byte("not audio")); got != (audioParams{}) {
			t.Errorf("probeAudio(%q) = %+v, want the zero value", mime, got)
		}
	}
}
