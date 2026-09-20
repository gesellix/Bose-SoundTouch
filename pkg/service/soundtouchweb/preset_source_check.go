package soundtouchweb

import (
	"sort"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// describeMissingSource says why a speaker could not play the given content,
// or returns "" when it has what the content names (issue 754).
//
// A preset can reference a source the target speaker does not have: a media
// server only one speaker has discovered, a music service linked on another
// speaker. Storing it anyway leaves a slot that looks fine and fails at play
// time, which is exactly what the editor exists to stop, so the check runs
// before the write and names what is missing.
//
// It never refuses on absent evidence. An empty source list means we do not
// know what the speaker has, not that it has nothing.
func describeMissingSource(sources *models.Sources, source, account string) string {
	if sources == nil || len(sources.SourceItem) == 0 || source == "" {
		return ""
	}

	// The speaker echoes the source name back as sourceAccount where there is
	// no real account ("TUNEIN"), and presets for the same station leave it
	// empty. Treating the placeholder as an account would demand a credential
	// that never existed.
	wanted := strings.TrimSpace(account)
	if strings.EqualFold(wanted, source) {
		wanted = ""
	}

	var matching []models.SourceItem

	for i := range sources.SourceItem {
		if strings.EqualFold(strings.TrimSpace(sources.SourceItem[i].Source), strings.TrimSpace(source)) {
			matching = append(matching, sources.SourceItem[i])
		}
	}

	if len(matching) == 0 {
		return "This speaker has no " + sourceDisplayName(source) + " source."
	}

	if wanted == "" {
		return ""
	}

	for i := range matching {
		if strings.EqualFold(strings.TrimSpace(matching[i].SourceAccount), wanted) {
			return ""
		}
	}

	// Naming what the speaker does have turns "it will not work" into
	// something actionable: the usual cause is a media server or a linked
	// service that exists on one speaker and not on this one.
	if available := accountNames(matching); available != "" {
		return "This speaker has no " + sourceDisplayName(source) + " account \"" + wanted +
			"\". It has: " + available + "."
	}

	return "This speaker has no " + sourceDisplayName(source) + " account \"" + wanted + "\"."
}

// accountNames lists what a speaker offers for one source, preferring the
// display names a person would recognise over the raw account ids.
func accountNames(items []models.SourceItem) string {
	seen := map[string]bool{}
	names := make([]string, 0, len(items))

	for i := range items {
		name := strings.TrimSpace(items[i].DisplayName)
		if name == "" {
			name = strings.TrimSpace(items[i].SourceAccount)
		}

		if name == "" || seen[name] {
			continue
		}

		seen[name] = true

		names = append(names, name)
	}

	sort.Strings(names)

	return strings.Join(names, ", ")
}

// sourceDisplayName mirrors the labels the player puts on preset tiles, so a
// refusal names the source the way the rest of the UI does.
func sourceDisplayName(source string) string {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case "STORED_MUSIC":
		return "Library"
	case "RADIO_BROWSER":
		return "Radio Browser"
	case "LOCAL_INTERNET_RADIO":
		return "Internet Radio"
	case "TUNEIN":
		return "TuneIn"
	case "SPOTIFY":
		return "Spotify"
	case "AMAZON":
		return "Amazon"
	case "DEEZER":
		return "Deezer"
	case "PANDORA":
		return "Pandora"
	case "IHEARTRADIO":
		return "iHeart"
	default:
		return source
	}
}

// checkPresetSource validates a store against what the target speaker has,
// and returns the sentence to refuse with, or "" to go ahead.
//
// A cached list that already accounts for the content answers the happy path
// without a speaker call. Anything else -- no cached list, or one that does
// not have the source -- is settled by asking the speaker, because a cached
// list can be minutes old and a service linked in the meantime would otherwise
// be reported as missing. Nothing is ever refused on the strength of the cache
// alone.
//
// If the speaker cannot be asked, the write proceeds: refusing on evidence we
// could not obtain would be worse than a preset that might not play.
func (app *WebApp) checkPresetSource(device *webtypes.DeviceConnection, source, account string) string {
	if status := device.Status(); status != nil && status.Sources != nil && len(status.Sources.SourceItem) > 0 {
		if describeMissingSource(status.Sources, source, account) == "" {
			return ""
		}
	}

	if device.Client == nil {
		return ""
	}

	live, err := device.Client.GetSources()
	if err != nil {
		return ""
	}

	return describeMissingSource(live, source, account)
}
