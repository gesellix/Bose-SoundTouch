package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// Issue 715: UpdatePreset addressed presets by list position, padding the
// slice to presetNumber and writing presets[presetNumber-1]. On a sparse list
// that both wrote the wrong entry and appended a second one for a button that
// already existed, and a preset list longer than the speaker's six slots costs
// a slot.
func TestUpsertPresetByButton_GH715(t *testing.T) {
	preset := func(button, name string) models.ServicePreset {
		return models.ServicePreset{
			ServiceContentItem: models.ServiceContentItem{Name: name},
			ID:                 button,
			ButtonNumber:       button,
		}
	}

	buttons := func(presets []models.ServicePreset) []string {
		out := make([]string, 0, len(presets))
		for i := range presets {
			out = append(out, presets[i].ButtonNumber)
		}

		return out
	}

	t.Run("a sparse list does not gain a duplicate", func(t *testing.T) {
		// Exactly the observed shape: slot 5 empty, save into slot 6.
		current := []models.ServicePreset{
			preset("1", "one"), preset("2", "two"), preset("3", "three"),
			preset("4", "four"), preset("6", "six"),
		}

		got := upsertPresetByButton(current, preset("6", "six updated"))

		if want := []string{"1", "2", "3", "4", "6"}; !equalStrings(buttons(got), want) {
			t.Errorf("buttons = %v, want %v", buttons(got), want)
		}

		if got[4].Name != "six updated" {
			t.Errorf("button 6 name = %q, want the updated entry", got[4].Name)
		}
	})

	t.Run("an unused button is appended", func(t *testing.T) {
		current := []models.ServicePreset{preset("1", "one"), preset("6", "six")}

		got := upsertPresetByButton(current, preset("5", "five"))

		if want := []string{"1", "6", "5"}; !equalStrings(buttons(got), want) {
			t.Errorf("buttons = %v, want %v", buttons(got), want)
		}
	})

	t.Run("an existing button is replaced in place", func(t *testing.T) {
		current := []models.ServicePreset{preset("1", "one"), preset("2", "two")}

		got := upsertPresetByButton(current, preset("1", "one updated"))

		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: replacing must not append", len(got))
		}

		if got[0].Name != "one updated" || got[0].ButtonNumber != "1" {
			t.Errorf("entry 0 = %+v, want the updated button 1", got[0])
		}
	})

	t.Run("an empty list takes the first entry", func(t *testing.T) {
		got := upsertPresetByButton(nil, preset("3", "three"))

		if len(got) != 1 || got[0].ButtonNumber != "3" {
			t.Errorf("got %v, want a single button 3", buttons(got))
		}
	})
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
