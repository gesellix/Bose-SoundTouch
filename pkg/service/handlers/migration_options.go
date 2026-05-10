package handlers

import "net/url"

// migrationOptionKeys is the allow-list of query parameters carried into
// the migration manager's options map. Two families coexist:
//
//   - marge / stats / sw_update / bmx — the XML method's per-field
//     "self | proxied | original" implementation selectors.
//   - marge_url / stats_url / sw_update_url / bmx_url — the telnet
//     method's per-field URL overrides (default: derive from target_url).
//
// Unrecognised keys are dropped so the manager never sees query
// parameters it did not opt into.
var migrationOptionKeys = map[string]struct{}{
	"marge":         {},
	"stats":         {},
	"sw_update":     {},
	"bmx":           {},
	"marge_url":     {},
	"stats_url":     {},
	"sw_update_url": {},
	"bmx_url":       {},
}

// parseMigrationOptions copies the recognised keys from query into a
// fresh map. Empty values are preserved as empty strings so the caller
// can distinguish "explicitly cleared" from "not set" if it ever needs
// to; the setup package's telnetURLsFromOptions treats empty as "use
// default", which is the desired UI behaviour today.
func parseMigrationOptions(query url.Values) map[string]string {
	out := make(map[string]string, len(migrationOptionKeys))

	for k, v := range query {
		if _, ok := migrationOptionKeys[k]; !ok {
			continue
		}

		if len(v) > 0 {
			out[k] = v[0]
		}
	}

	return out
}
