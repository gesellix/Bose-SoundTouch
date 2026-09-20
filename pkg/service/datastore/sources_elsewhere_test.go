package datastore

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func sourceFor(sourceType, account, displayName string) models.ConfiguredSource {
	s := models.ConfiguredSource{DisplayName: displayName, Secret: "bs-" + account, SecretType: "token"}
	s.SourceKey.Type = sourceType
	s.SourceKey.Account = account

	return s
}

func TestSourcesElsewhere(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	// The speaker we are asking about: one media server, TuneIn.
	if err := ds.SaveConfiguredSources("ACCOUNT01", "DEVICEID01", []models.ConfiguredSource{
		sourceFor("STORED_MUSIC", "uuid:mine/0", "MiniDLNA"),
		sourceFor("TUNEIN", "", "TuneIn"),
		sourceFor("AUX", "AUX", "AUX IN"),
	}); err != nil {
		t.Fatalf("SaveConfiguredSources: %v", err)
	}

	// A second speaker with a media server and a linked service this one does
	// not have, plus one they share and one that is only ever local.
	if err := ds.SaveConfiguredSources("ACCOUNT01", "DEVICEID02", []models.ConfiguredSource{
		sourceFor("STORED_MUSIC", "uuid:theirs/0", "fritz"),
		sourceFor("SPOTIFY", "listener", "Spotify"),
		sourceFor("TUNEIN", "", "TuneIn"),
		sourceFor("BLUETOOTH", "", "Bluetooth"),
	}); err != nil {
		t.Fatalf("SaveConfiguredSources: %v", err)
	}

	// A speaker on another account contributes too: the question is what this
	// speaker could be given, not who it is paired with.
	if err := ds.SaveConfiguredSources("ACCOUNT02", "DEVICEID03", []models.ConfiguredSource{
		sourceFor("STORED_MUSIC", "uuid:theirs/0", ""),
	}); err != nil {
		t.Fatalf("SaveConfiguredSources: %v", err)
	}

	found, err := ds.SourcesElsewhere("ACCOUNT01", "DEVICEID01")
	if err != nil {
		t.Fatalf("SourcesElsewhere: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("expected the media server and Spotify, got %+v", found)
	}

	// Addable first, so the list leads with what can be acted on.
	if found[0].Type != "STORED_MUSIC" || found[0].Availability != models.SourceAvailableToAdd {
		t.Errorf("first entry = %+v, want the addable media server", found[0])
	}

	if found[0].DisplayName != "fritz" {
		t.Errorf("display name = %q, want the one a speaker recorded", found[0].DisplayName)
	}

	if len(found[0].Devices) != 2 {
		t.Errorf("expected both speakers that have it, got %v", found[0].Devices)
	}

	if found[1].Type != "SPOTIFY" || found[1].Availability != models.SourceAvailableByLinking {
		t.Errorf("second entry = %+v, want Spotify as link-required", found[1])
	}

	for _, identity := range found {
		if identity.Type == "TUNEIN" {
			t.Error("a source this speaker already has must not be offered")
		}

		if identity.Availability == models.SourceAvailableLocalOnly {
			t.Errorf("a device-local source was offered: %+v", identity)
		}
	}
}

func TestSourcesElsewhereWithNoOtherSpeakers(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SaveConfiguredSources("ACCOUNT01", "DEVICEID01", []models.ConfiguredSource{
		sourceFor("TUNEIN", "", "TuneIn"),
	}); err != nil {
		t.Fatalf("SaveConfiguredSources: %v", err)
	}

	found, err := ds.SourcesElsewhere("ACCOUNT01", "DEVICEID01")
	if err != nil {
		t.Fatalf("SourcesElsewhere: %v", err)
	}

	if len(found) != 0 {
		t.Fatalf("expected nothing to offer, got %+v", found)
	}
}
