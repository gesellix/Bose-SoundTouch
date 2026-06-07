package soundtouchweb

import (
	"log"
	"strings"
)

// sanitizeLog strips newline characters from s to prevent log-injection
// (CodeQL go/log-injection). Values from speakers, HTTP requests, and
// external APIs may contain attacker-controlled newlines.
func sanitizeLog(s string) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)

	return s
}

// logPlaybackRequest records what soundtouch-player is about to ask a speaker to
// play or switch to. A SoundTouch /select returns HTTP 200 even when the
// source is ultimately rejected: the failure only surfaces afterwards as a
// now_playing transition to an error source (see logNowPlayingError). So this
// line is frequently the only record of what was actually requested, and the
// pair (request here, error transition there) is what closes the loop when
// diagnosing source/playback failures.
//
// sourceAccount here is an account identifier (e.g. "AUX1" for a specific jack,
// or a placeholder username), not a bearer credential: the real OAuth tokens
// live in the service datastore, not in the ContentItem sent on /select. It is
// logged as-is so multi-account sources can be debugged.
func logPlaybackRequest(action, deviceID, source, sourceAccount, location, itemName string) {
	log.Printf("[play] %s device=%q source=%q sourceAccount=%q location=%q itemName=%q",
		sanitizeLog(action),
		sanitizeLog(deviceID),
		sanitizeLog(source),
		sanitizeLog(sourceAccount),
		sanitizeLog(location),
		sanitizeLog(itemName),
	)
}

// isErrorSource reports whether a now_playing source value indicates the
// speaker rejected or failed a selection rather than entering a normal state.
// It covers INVALID_SOURCE and the family of *_ERROR sources the firmware
// emits (e.g. UNKNOWN_SOURCE_ERROR).
func isErrorSource(source string) bool {
	return source == "INVALID_SOURCE" || strings.HasSuffix(source, "_ERROR")
}

// logNowPlayingError logs when a speaker's now_playing enters an error source.
// Because /select returns 200 regardless, this asynchronous transition is the
// real signal that a selection failed on the device.
func logNowPlayingError(deviceID, source, sourceAccount string) {
	log.Printf("[play] device=%q now_playing entered error source=%q sourceAccount=%q",
		sanitizeLog(deviceID),
		sanitizeLog(source),
		sanitizeLog(sourceAccount),
	)
}
