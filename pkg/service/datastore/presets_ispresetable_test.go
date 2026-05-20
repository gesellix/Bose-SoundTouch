package datastore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestSavePresets_PreservesIsPresetable is a regression test for GH-235:
// SavePresets used to hard-code isPresetable="true", masking the speaker's
// own verdict that Spotify-Connect content isn't recallable. Storing the
// preset looked like it succeeded but pressing it on the speaker did
// nothing. The fix preserves whatever IsPresetable the speaker provided,
// defaulting to "true" only when the caller supplied an empty string.
func TestSavePresets_PreservesIsPresetable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-ispresetable-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "AABBCCDDEEFF"

	presets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:         "Connect Playlist",
				Source:       "SPOTIFY",
				IsPresetable: "false",
			},
			ButtonNumber: "1",
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:         "Default Truth",
				Source:       "TUNEIN",
				IsPresetable: "true",
			},
			ButtonNumber: "2",
		},
		{
			ServiceContentItem: models.ServiceContentItem{
				Name:   "Caller Left Empty",
				Source: "INTERNET_RADIO",
				// IsPresetable intentionally unset
			},
			ButtonNumber: "3",
		},
	}

	if err := ds.SavePresets(account, device, presets); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml"))
	if err != nil {
		t.Fatalf("read Presets.xml: %v", err)
	}

	got := string(body)

	if !strings.Contains(got, `id="1"`) || !strings.Contains(got, `isPresetable="false"`) {
		t.Errorf("preset 1: expected isPresetable=\"false\" preserved; Presets.xml:\n%s", got)
	}

	if !strings.Contains(got, `id="2"`) || strings.Count(got, `isPresetable="true"`) < 2 {
		t.Errorf("preset 2/3: expected isPresetable=\"true\" (preset 2 from caller, preset 3 from empty-string default); Presets.xml:\n%s", got)
	}
}
