package datastore

import (
	"strconv"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// canonicalSourceKeyType corrects a source key type that holds the provider's
// numeric ID instead of its name.
//
// Before issue 697, AddSource stored a source the speaker registered (a DLNA
// media server, for example) with its provider ID as the type: "7" instead of
// "STORED_MUSIC". Presets and recents name their source by type, so they
// matched none of these sources. Sources.xml files written back then still
// hold the ID, and reading them through here repairs them; the next write
// persists the name.
//
// Only that exact shape is changed: a type equal to the source's own provider
// ID that names a known provider. Anything else is returned as it is.
func canonicalSourceKeyType(sourceKeyType, sourceProviderID string) string {
	if sourceKeyType == "" || sourceKeyType != sourceProviderID {
		return sourceKeyType
	}

	return constants.GetProviderName(sourceProviderID)
}

// canonicalContentSource corrects a preset's or recent's source that holds a
// provider's numeric ID instead of its name.
//
// Saving a preset copies the matched source's key type into the preset. While
// Sources.xml held a media server under type "7" (see canonicalSourceKeyType),
// presets stored for it got source "7" as well. The speaker itself names the
// source ("STORED_MUSIC"), so the stored row no longer equals the speaker's,
// and the player reports the slot as different although it holds the same
// content.
//
// No provider name is numeric, so a source that parses as a known provider ID
// can only be this leftover. Unknown numbers are returned as they are.
func canonicalContentSource(source string) string {
	if source == "" {
		return source
	}

	if _, err := strconv.Atoi(source); err != nil {
		return source
	}

	return constants.GetProviderName(source)
}
