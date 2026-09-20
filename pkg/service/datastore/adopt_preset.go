package datastore

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/catalog"
)

// SetStoredPreset writes one slot of the stored preset list, replacing
// whatever occupied that button, and returns the list as it now stands.
//
// It exists for the per-slot choice in the stored-list repair (issue 697):
// where AfterTouch and the speaker disagree about a slot, taking the
// speaker's side has to write exactly that slot and leave the other five
// alone. The whole-list import ("Sync Data") is the thing this is an
// alternative to.
//
// What it displaces is filed in the catalog first, so choosing one side of a
// disagreement cannot lose the other.
func (ds *DataStore) SetStoredPreset(account, device string, preset models.ServicePreset) ([]models.StoredPresetRow, error) {
	button := effectivePresetButton(preset)

	slot, err := strconv.Atoi(button)
	if err != nil || slot < 1 || slot > PresetButtons {
		return nil, fmt.Errorf("preset button must be between 1 and %d", PresetButtons)
	}

	var displaced []models.ServicePreset

	next, err := ds.MutatePresets(account, device, func(current []models.ServicePreset) ([]models.ServicePreset, error) {
		kept := make([]models.ServicePreset, 0, len(current)+1)

		for i := range current {
			if effectivePresetButton(current[i]) == button {
				displaced = append(displaced, current[i])

				continue
			}

			kept = append(kept, current[i])
		}

		return append(kept, preset), nil
	})
	if err != nil {
		return nil, err
	}

	if len(displaced) > 0 {
		entries := catalogEntriesFromPresets(device, displaced)
		for i := range entries {
			entries[i].LastSeen = time.Now().UTC()
			entries[i].Origin = catalog.OriginPreset
		}

		ds.RecordCatalogEntries(entries)
	}

	return ClassifyStoredPresets(next), nil
}
