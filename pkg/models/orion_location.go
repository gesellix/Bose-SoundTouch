package models

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

// OrionBasePath is the path of the Orion adapter (LOCAL_INTERNET_RADIO) on a
// BMX host. The BMX registry advertises it as the service's baseUrl, as
// "{BMX_SERVER}" + OrionBasePath (see bmx_services.json).
const OrionBasePath = "/core02/svc-bmx-adapter-orion/prod/orion"

// OrionStationPath is the station endpoint below OrionBasePath. A relative
// LOCAL_INTERNET_RADIO location starts with it.
const OrionStationPath = "/station"

// OrionLocalInternetRadioSource is the source a ContentItem with an Orion
// station location is played from.
const OrionLocalInternetRadioSource = "LOCAL_INTERNET_RADIO"

// RelativeOrionLocation returns the relative form ("/station?data=...") of an
// Orion station location, and whether location is one at all.
//
// Both forms are accepted: the relative one is returned unchanged, and an
// absolute one ("<any host>/core02/svc-bmx-adapter-orion/prod/orion/station?...")
// has everything up to and including OrionBasePath stripped, whatever the
// host. The speaker resolves a relative location against the Orion baseUrl
// from the BMX registry, the same way it resolves TuneIn's and Radio
// Browser's (verified on a SoundTouch 10, firmware 27.0.6, 2026-09-27; see
// issue 769). So both forms name the same station; only the relative one
// follows AfterTouch to a new address.
func RelativeOrionLocation(location string) (string, bool) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", false
	}

	if isRelativeOrionStation(location) {
		return location, true
	}

	u, err := url.Parse(location)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}

	rest, ok := strings.CutPrefix(u.EscapedPath(), OrionBasePath)
	if !ok || rest != OrionStationPath {
		return "", false
	}

	relative := OrionStationPath
	if u.RawQuery != "" {
		relative += "?" + u.RawQuery
	}

	return relative, true
}

// IsOrionStationLocation reports whether location is an Orion station
// location, in its relative or absolute form.
func IsOrionStationLocation(location string) bool {
	_, ok := RelativeOrionLocation(location)

	return ok
}

// OrionLocationHost returns the host an absolute Orion station location
// points at ("" for the relative form or anything that is not an Orion
// station location).
func OrionLocationHost(location string) string {
	if _, ok := RelativeOrionLocation(location); !ok {
		return ""
	}

	u, err := url.Parse(strings.TrimSpace(location))
	if err != nil {
		return ""
	}

	return u.Host
}

// CanonicalContentLocation returns the key to compare when deciding whether
// two ContentItems play the same content. It is a comparison key, never a
// location to store or send to a speaker.
//
// A LOCAL_INTERNET_RADIO station has several spellings that all play one
// stream: the relative Orion location ("/station?data=..."), the absolute one
// saved before issue 769, and the older playback adapter
// ("<host>/custom/v1/playback/<base64 stream>?name=..."). They are keyed by
// the stream URL they wrap (issue 784), so a list mixing them holds one
// station, not three. An Orion location whose data cannot be decoded falls
// back to its relative form. Every other location is returned as it is.
func CanonicalContentLocation(source, location string) string {
	if !strings.EqualFold(strings.TrimSpace(source), OrionLocalInternetRadioSource) {
		return location
	}

	if stream, ok := StationStreamURL(location); ok {
		return stationStreamKeyPrefix + stream
	}

	if relative, ok := RelativeOrionLocation(location); ok {
		return relative
	}

	return location
}

// stationStreamKeyPrefix marks a canonical key built from a stream URL, so it
// can never equal a real location.
const stationStreamKeyPrefix = "stream:"

// PlaybackAdapterPath is the path of the older LOCAL_INTERNET_RADIO playback
// adapter: "<host>/custom/v1/playback/<base64 stream URL>?name=...". Its
// locations carry the service's address, like the absolute Orion form.
const PlaybackAdapterPath = "/custom/v1/playback/"

// StationStreamURL returns the stream URL a LOCAL_INTERNET_RADIO station
// location wraps: the "streamUrl" in an Orion location's data (relative or
// absolute), or the path segment of a playback adapter location. It reports
// false for anything else, including a raw stream URL used as a location.
func StationStreamURL(location string) (string, bool) {
	location = strings.TrimSpace(location)

	if relative, ok := RelativeOrionLocation(location); ok {
		return orionDataStreamURL(relative)
	}

	return playbackAdapterStreamURL(location)
}

func orionDataStreamURL(relative string) (string, bool) {
	_, query, _ := strings.Cut(relative, "?")

	values, err := url.ParseQuery(query)
	if err != nil {
		return "", false
	}

	decoded, ok := decodeLenientBase64(values.Get("data"))
	if !ok {
		return "", false
	}

	var payload struct {
		StreamURL string `json:"streamUrl"`
	}
	if json.Unmarshal(decoded, &payload) != nil {
		return "", false
	}

	stream := strings.TrimSpace(payload.StreamURL)

	return stream, stream != ""
}

func playbackAdapterStreamURL(location string) (string, bool) {
	path := location
	if u, err := url.Parse(location); err == nil {
		path = u.EscapedPath()
	}

	_, encoded, found := strings.Cut(path, PlaybackAdapterPath)
	if !found || encoded == "" || strings.Contains(encoded, "/") {
		return "", false
	}

	if unescaped, err := url.PathUnescape(encoded); err == nil {
		encoded = unescaped
	}

	// The adapter accepts a base64-encoded URL and, failing that, a plain
	// (URL-escaped) one; see HandleCustomPlayback.
	stream := encoded
	if decoded, ok := decodeLenientBase64(encoded); ok && looksLikeStreamURL(string(decoded)) {
		stream = string(decoded)
	}

	if !looksLikeStreamURL(stream) {
		return "", false
	}

	return stream, true
}

func looksLikeStreamURL(s string) bool {
	u, err := url.Parse(s)

	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}

// decodeLenientBase64 accepts standard and URL-safe base64, padded or not.
func decodeLenientBase64(s string) ([]byte, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}

	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return decoded, true
		}
	}

	return nil, false
}

func isRelativeOrionStation(location string) bool {
	rest, ok := strings.CutPrefix(location, OrionStationPath)
	if !ok {
		return false
	}

	return rest == "" || strings.HasPrefix(rest, "?")
}
