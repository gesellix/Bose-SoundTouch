package datastore

import (
	"fmt"
	"strconv"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// PresetButtons is how many preset buttons a SoundTouch has. Every model in
// the line has six, so a stored row outside that range can never be recalled.
const PresetButtons = 6

// ClassifyStoredPresets turns a stored preset list into rows with a verdict.
//
// The read path already collapses two rows claiming the same button, so what
// survives to here and still cannot be recalled is a row with no usable id at
// all, or one pointing at a button the speaker does not have. Both still count
// towards the stored list's length, which is what makes a sync look
// destructive (issue 697).
func ClassifyStoredPresets(presets []models.ServicePreset) []models.StoredPresetRow {
	rows := make([]models.StoredPresetRow, 0, len(presets))

	for i := range presets {
		p := &presets[i]

		row := models.StoredPresetRow{
			Index:         i,
			Button:        effectivePresetButton(*p),
			Source:        p.Source,
			SourceAccount: p.SourceAccount,
			Location:      p.Location,
			Name:          p.Name,
			ContainerArt:  p.ContainerArt,
		}

		switch button, err := strconv.Atoi(row.Button); {
		case row.Button == "" || err != nil:
			row.Verdict = models.StoredPresetNoSlot
		case button < 1 || button > PresetButtons:
			row.Slot = button
			row.Verdict = models.StoredPresetOutOfRange
		case p.Source == "" || p.Location == "":
			row.Slot = button
			row.Verdict = models.StoredPresetEmptyContent
		default:
			row.Slot = button
			row.Verdict = models.StoredPresetOK
		}

		rows = append(rows, row)
	}

	return rows
}

// StoredPresets returns what AfterTouch has stored for a device, as stored.
func (ds *DataStore) StoredPresets(account, device string) ([]models.StoredPresetRow, error) {
	presets, err := ds.GetPresetsReadOnly(account, device)
	if err != nil {
		return nil, err
	}

	return ClassifyStoredPresets(presets), nil
}

// DropStoredPresetRows deletes stored rows by position and returns what is
// left. expected is the row count the caller was looking at; a mismatch means
// the list changed underneath it, and nothing is deleted.
//
// Addressing rows by position rather than by button number is deliberate: the
// rows most in need of deleting are exactly the ones with no button number to
// address them by.
func (ds *DataStore) DropStoredPresetRows(account, device string, indexes []int, expected int) ([]models.StoredPresetRow, error) {
	drop := make(map[int]bool, len(indexes))
	for _, i := range indexes {
		drop[i] = true
	}

	if len(drop) == 0 {
		return nil, fmt.Errorf("no rows to drop")
	}

	kept, err := ds.MutatePresets(account, device, func(current []models.ServicePreset) ([]models.ServicePreset, error) {
		if len(current) != expected {
			return nil, fmt.Errorf("the stored list now holds %d rows, not %d; reload and try again", len(current), expected)
		}

		for i := range indexes {
			if indexes[i] < 0 || indexes[i] >= len(current) {
				return nil, fmt.Errorf("row %d is not in the stored list", indexes[i])
			}
		}

		next := make([]models.ServicePreset, 0, len(current))

		for i := range current {
			if !drop[i] {
				next = append(next, current[i])
			}
		}

		return next, nil
	})
	if err != nil {
		return nil, err
	}

	return ClassifyStoredPresets(kept), nil
}
