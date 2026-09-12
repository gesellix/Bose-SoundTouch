package main

import (
	"encoding/binary"
)

// A DLNA server is expected to report how long a track is. Ours reported
// duration="0:00:00.000" for every file, because nothing ever measured one,
// and a speaker given that reports <time total="0"> in turn: no progress bar
// in any client, including our own player.
//
// Measuring properly means reading the audio itself. These probes read only
// the header (plus, for VBR MP3, the Xing/Info frame), which is enough for the
// duration, bit rate, sample rate and channel count that DIDL-Lite wants, and
// costs nothing compared with decoding. Formats we cannot parse report zero
// and fall back to the placeholders, exactly as before.

// audioParams is what a probe can tell about a track.
type audioParams struct {
	DurSec     float64
	Bitrate    int // bits per second
	SampleRate int // Hz
	Channels   int
}

// probeAudio measures what it can from a file's bytes, by MIME type.
func probeAudio(mime string, payload []byte) audioParams {
	switch mime {
	case "audio/mpeg":
		return probeMP3(payload)
	case "audio/x-wav", "audio/wav":
		return probeWAV(payload)
	default:
		return audioParams{}
	}
}

// --- WAV ---------------------------------------------------------------------

// probeWAV reads the RIFF header: the fmt chunk carries the sample rate,
// channel count and byte rate, and the data chunk's length divided by the byte
// rate is the duration.
func probeWAV(b []byte) audioParams {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return audioParams{}
	}

	var params audioParams

	var byteRate uint32

	for pos := 12; pos+8 <= len(b); {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		body := pos + 8

		if size < 0 || body+size > len(b) {
			size = len(b) - body // tolerate a truncated final chunk
		}

		switch {
		case id == "fmt " && size >= 16:
			params.Channels = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
			params.SampleRate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
			byteRate = binary.LittleEndian.Uint32(b[body+8 : body+12])
			params.Bitrate = int(byteRate) * 8
		case id == "data" && byteRate > 0:
			params.DurSec = float64(size) / float64(byteRate)

			return params
		}

		pos = body + size
		if size%2 == 1 {
			pos++ // chunks are word-aligned
		}
	}

	return params
}

// --- MP3 ---------------------------------------------------------------------

// MPEG audio bit rates in kbit/s, indexed by the header's bitrate_index, for
// the version/layer combinations we care about.
var mp3BitrateKbps = map[int][16]int{
	// MPEG 1 Layer III
	1: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	// MPEG 2 / 2.5 Layer III
	2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
}

var mp3SampleRates = map[int][4]int{
	3: {44100, 48000, 32000, 0}, // MPEG 1
	2: {22050, 24000, 16000, 0}, // MPEG 2
	0: {11025, 12000, 8000, 0},  // MPEG 2.5
}

// probeMP3 finds the first audio frame and reads its header. A Xing or Info
// frame, written by VBR encoders, carries the total frame count, which gives
// an exact duration; without one the file is assumed constant-bitrate and the
// duration comes from the remaining byte count.
func probeMP3(b []byte) audioParams {
	start := skipID3v2(b)

	offset, header, ok := findMP3Frame(b, start)
	if !ok {
		return audioParams{}
	}

	versionBits := int(header[1]>>3) & 0x03
	bitrateIndex := int(header[2]>>4) & 0x0f
	sampleIndex := int(header[2]>>2) & 0x03
	channelMode := int(header[3]>>6) & 0x03

	sampleRates, known := mp3SampleRates[versionBits]
	if !known {
		return audioParams{}
	}

	sampleRate := sampleRates[sampleIndex]

	table := 2
	if versionBits == 3 {
		table = 1
	}

	bitrate := mp3BitrateKbps[table][bitrateIndex] * 1000

	channels := 2
	if channelMode == 3 {
		channels = 1
	}

	if sampleRate == 0 || bitrate == 0 {
		return audioParams{}
	}

	params := audioParams{Bitrate: bitrate, SampleRate: sampleRate, Channels: channels}

	// Samples per frame: 1152 for MPEG 1 Layer III, 576 for MPEG 2/2.5.
	samplesPerFrame := 576
	if versionBits == 3 {
		samplesPerFrame = 1152
	}

	if frames, found := xingFrameCount(b, offset, versionBits, channelMode); found {
		params.DurSec = float64(frames) * float64(samplesPerFrame) / float64(sampleRate)

		return params
	}

	params.DurSec = float64(len(b)-offset) * 8 / float64(bitrate)

	return params
}

// skipID3v2 returns the offset just past an ID3v2 tag, if the file opens with
// one. Its size is stored as four 7-bit bytes.
func skipID3v2(b []byte) int {
	if len(b) < 10 || string(b[0:3]) != "ID3" {
		return 0
	}

	size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
	end := 10 + size

	if end > len(b) {
		return 0
	}

	return end
}

// findMP3Frame scans for the first frame sync, the 11 set bits that open every
// MPEG audio frame, skipping anything the tag parser left behind.
func findMP3Frame(b []byte, start int) (int, []byte, bool) {
	const maxScan = 1 << 16 // a sync this far in means it is not an MP3 we can read

	limit := start + maxScan
	if limit > len(b)-4 {
		limit = len(b) - 4
	}

	for i := start; i <= limit; i++ {
		if b[i] != 0xff || b[i+1]&0xe0 != 0xe0 {
			continue
		}

		// Reject the reserved version and layer encodings, so a random 0xff
		// byte inside metadata is not mistaken for a frame.
		if int(b[i+1]>>3)&0x03 == 1 || int(b[i+1]>>1)&0x03 == 0 {
			continue
		}

		return i, b[i : i+4], true
	}

	return 0, nil, false
}

// xingFrameCount reads the frame count from a Xing or Info header, which VBR
// encoders place in the first frame's side-information area.
func xingFrameCount(b []byte, frameOffset, versionBits, channelMode int) (int, bool) {
	// The tag sits after the side information, whose size depends on the MPEG
	// version and whether the stream is mono.
	var sideInfo int

	switch {
	case versionBits == 3 && channelMode == 3:
		sideInfo = 17
	case versionBits == 3:
		sideInfo = 32
	case channelMode == 3:
		sideInfo = 9
	default:
		sideInfo = 17
	}

	pos := frameOffset + 4 + sideInfo
	if pos+12 > len(b) {
		return 0, false
	}

	tag := string(b[pos : pos+4])
	if tag != "Xing" && tag != "Info" {
		return 0, false
	}

	flags := binary.BigEndian.Uint32(b[pos+4 : pos+8])
	if flags&0x0001 == 0 {
		return 0, false // no frame count present
	}

	return int(binary.BigEndian.Uint32(b[pos+8 : pos+12])), true
}
