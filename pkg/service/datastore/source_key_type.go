package datastore

import "github.com/gesellix/bose-soundtouch/pkg/service/constants"

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
