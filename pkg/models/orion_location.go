package models

import (
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

// CanonicalContentLocation returns the location to compare when deciding
// whether two ContentItems play the same content. For LOCAL_INTERNET_RADIO
// Orion stations that is the relative form, so an absolute location saved
// before issue 769 and the relative one saved since count as one station.
// Every other location is returned as it is.
func CanonicalContentLocation(source, location string) string {
	if !strings.EqualFold(strings.TrimSpace(source), OrionLocalInternetRadioSource) {
		return location
	}

	if relative, ok := RelativeOrionLocation(location); ok {
		return relative
	}

	return location
}

func isRelativeOrionStation(location string) bool {
	rest, ok := strings.CutPrefix(location, OrionStationPath)
	if !ok {
		return false
	}

	return rest == "" || strings.HasPrefix(rest, "?")
}
