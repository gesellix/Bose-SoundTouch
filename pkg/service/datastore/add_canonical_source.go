package datastore

import (
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// AddCanonicalSource gives a device one of the sources AfterTouch defines
// itself -- TuneIn, Radio Browser, Local Internet Radio (issue 754).
//
// The definition is taken from this service's own defaults, never copied from
// the speaker that happens to have it. Copying would carry that speaker's
// secret across, and there is no reason to: these are exactly the sources
// whose token the service mints, so the canonical entry is the right one by
// construction.
//
// It is idempotent: a device that already has the source keeps what it has,
// and the caller is told nothing changed.
func (ds *DataStore) AddCanonicalSource(account, device, sourceType string) (added bool, err error) {
	wanted := strings.ToUpper(strings.TrimSpace(sourceType))

	if models.SourceAvailability(wanted) != models.SourceAvailableToAdd {
		return false, fmt.Errorf("%s is not a source AfterTouch can add on its own", wanted)
	}

	canonical, ok := ds.canonicalSourceByType(wanted)
	if !ok {
		// STORED_MUSIC is addable, but not from the defaults: a media server
		// is registered on the speaker itself, which the player does through
		// the speaker's own API.
		return false, fmt.Errorf("no canonical definition for %s", wanted)
	}

	_, err = ds.MutateConfiguredSources(account, device, func(current []models.ConfiguredSource) ([]models.ConfiguredSource, error) {
		for i := range current {
			if strings.EqualFold(models.IdentityOfSource(current[i]).Type, wanted) {
				return current, nil
			}
		}

		added = true

		return append(current, canonical), nil
	})
	if err != nil {
		return false, err
	}

	return added, nil
}

// canonicalSourceByType finds this service's own definition of a source by its
// sourceKey type, which is how a caller names it; the defaults are keyed by
// the numeric id the speaker sees.
func (ds *DataStore) canonicalSourceByType(sourceType string) (models.ConfiguredSource, bool) {
	defaults := ds.getDefaultSources()

	for i := range defaults {
		if strings.EqualFold(models.IdentityOfSource(defaults[i]).Type, sourceType) {
			return defaults[i], true
		}
	}

	return models.ConfiguredSource{}, false
}
