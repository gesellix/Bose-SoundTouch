package datastore

import (
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// A speaker missing TuneIn while its neighbour has it is the shape behind
// several "radio sources do not mount" reports, so giving it the source is a
// repair, not a convenience.
func TestAddCanonicalSource(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if err := ds.SaveConfiguredSources("ACCOUNT01", "DEVICEID01", []models.ConfiguredSource{
		sourceFor("AUX", "AUX", "AUX IN"),
	}); err != nil {
		t.Fatalf("SaveConfiguredSources: %v", err)
	}

	added, err := ds.AddCanonicalSource("ACCOUNT01", "DEVICEID01", "TUNEIN")
	if err != nil {
		t.Fatalf("AddCanonicalSource: %v", err)
	}

	if !added {
		t.Fatal("expected the source to be added")
	}

	sources, err := ds.GetConfiguredSources("ACCOUNT01", "DEVICEID01")
	if err != nil {
		t.Fatalf("GetConfiguredSources: %v", err)
	}

	var found *models.ConfiguredSource

	for i := range sources {
		if models.IdentityOfSource(sources[i]).Type == "TUNEIN" {
			found = &sources[i]
		}
	}

	if found == nil {
		t.Fatalf("TuneIn is not in the stored sources: %+v", sources)
	}

	// The definition comes from this service's own defaults, so it carries the
	// token this service mints -- never the secret of whichever speaker
	// happened to have the source first.
	if found.Secret == "" || found.Secret == "bs-" {
		t.Errorf("expected the canonical minted token, got %q", found.Secret)
	}
}

func TestAddCanonicalSourceIsIdempotent(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	if _, err := ds.AddCanonicalSource("ACCOUNT01", "DEVICEID01", "TUNEIN"); err != nil {
		t.Fatalf("first add: %v", err)
	}

	added, err := ds.AddCanonicalSource("ACCOUNT01", "DEVICEID01", "tunein")
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	if added {
		t.Error("a source the device already has must not be added twice")
	}

	sources, _ := ds.GetConfiguredSources("ACCOUNT01", "DEVICEID01")

	count := 0

	for i := range sources {
		if models.IdentityOfSource(sources[i]).Type == "TUNEIN" {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("expected exactly one TuneIn entry, got %d", count)
	}
}

// The classification is the guard: a credential we cannot mint must never be
// conjured from the defaults, and a socket is not something to add at all.
func TestAddCanonicalSourceRefusesWhatItCannotMint(t *testing.T) {
	ds := NewDataStore(t.TempDir())

	for _, sourceType := range []string{"SPOTIFY", "AMAZON", "AUX", "BLUETOOTH"} {
		added, err := ds.AddCanonicalSource("ACCOUNT01", "DEVICEID01", sourceType)
		if err == nil {
			t.Errorf("%s: expected a refusal", sourceType)
		}

		if added {
			t.Errorf("%s: reported as added", sourceType)
		}
	}

	// STORED_MUSIC is addable, but through the speaker rather than from our
	// defaults, so this path has to say so rather than pretend.
	_, err := ds.AddCanonicalSource("ACCOUNT01", "DEVICEID01", "STORED_MUSIC")
	if err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Errorf("STORED_MUSIC: expected a no-definition error, got %v", err)
	}
}
