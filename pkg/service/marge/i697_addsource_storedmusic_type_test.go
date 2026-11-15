package marge

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// TestAddSourceStoresMediaServerUnderItsTypeName is a regression test for
// issue 697. AddSource stored a media server the speaker registered with its
// provider ID as the source key type ("7" instead of "STORED_MUSIC"). Presets
// name their source by type, so none matched it, and /presets served every
// library preset without a source block. A speaker that fetched that list
// lost the media-server account on its presets, which then failed with
// INVALID_SOURCE.
func TestAddSourceStoresMediaServerUnderItsTypeName(t *testing.T) {
	const (
		account = "0000000"
		device  = "DEVICEID01"
		udn     = "00000000-0000-0000-0000-0000000000aa/0"
	)

	preset := models.ServicePreset{ButtonNumber: "2", ID: "2"}
	preset.Source = constants.ProviderStoredMusic
	preset.SourceAccount = udn
	preset.Location = "214564"
	preset.Name = "Library item"

	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		device: {preset},
	})

	if _, err := AddSource(ds, account, udn, strconv.Itoa(constants.StoredMusicProviderID), "", "", "Library"); err != nil {
		t.Fatalf("add source: %v", err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("get sources: %v", err)
	}

	var found bool

	for _, s := range sources {
		if s.SourceKey.Account != udn {
			continue
		}

		found = true

		if s.SourceKey.Type != constants.ProviderStoredMusic {
			t.Errorf("media server source key type = %q, want %q", s.SourceKey.Type, constants.ProviderStoredMusic)
		}
	}

	if !found {
		t.Fatalf("media server source %q not stored: %+v", udn, sources)
	}

	xmlData, err := PresetsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("presets XML: %v", err)
	}

	if !strings.Contains(string(xmlData), "<username>"+udn+"</username>") {
		t.Fatalf("/presets serves the library preset without its media-server account in <username>:\n%s", xmlData)
	}
}
